package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Mode 表示当前部署的认证模式。
type Mode string

const (
	// ModeGateway 走 fnOS 统一网关，由网关注入可信身份头。
	ModeGateway Mode = "gateway"
	// ModeLocal 局域网直连，使用应用自身的账号密码。
	ModeLocal Mode = "local"
)

// Identity 是当前请求对应的用户身份。
type Identity struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	// Source 说明身份来源：gateway / local / token
	Source string `json:"source"`
}

const (
	sessionCookieName = "crazynote_session"
	sessionTTL        = 30 * 24 * time.Hour
	// 网关注入的可信身份头，仅在 unix socket 监听时可信
	headerUserID   = "X-Trim-Userid"
	headerUsername = "X-Trim-Username"
	headerIsAdmin  = "X-Trim-Isadmin"
)

// Manager 管理本地账号、会话与 API Token。
type Manager struct {
	mu sync.RWMutex

	appDataDir string
	mode       Mode

	// 本地模式下保存口令哈希
	passwordHash string
	passwordSalt string

	// API Token，用于网页摘录扩展免登录写入
	tokens map[string]string // token -> username

	secret []byte
}

type credentialsFile struct {
	PasswordHash string            `json:"password_hash"`
	PasswordSalt string            `json:"password_salt"`
	Tokens       map[string]string `json:"tokens"`
	Secret       string            `json:"secret"`
}

// New 创建认证管理器。mode 决定是否启用本地口令登录。
func New(appDataDir string, mode Mode) (*Manager, error) {
	if err := os.MkdirAll(appDataDir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{
		appDataDir: appDataDir,
		mode:       mode,
		tokens:     map[string]string{},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) credPath() string {
	return filepath.Join(m.appDataDir, "credentials.json")
}

func (m *Manager) load() error {
	b, err := os.ReadFile(m.credPath())
	if err != nil {
		if os.IsNotExist(err) {
			// 首次启动：生成会话密钥
			secret := make([]byte, 32)
			if _, err := rand.Read(secret); err != nil {
				return err
			}
			m.secret = secret
			return m.save()
		}
		return err
	}
	var c credentialsFile
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	m.passwordHash = c.PasswordHash
	m.passwordSalt = c.PasswordSalt
	m.tokens = c.Tokens
	if m.tokens == nil {
		m.tokens = map[string]string{}
	}
	if c.Secret != "" {
		if s, err := base64.StdEncoding.DecodeString(c.Secret); err == nil {
			m.secret = s
		}
	}
	if len(m.secret) == 0 {
		m.secret = make([]byte, 32)
		_, _ = rand.Read(m.secret)
	}
	return nil
}

func (m *Manager) save() error {
	c := credentialsFile{
		PasswordHash: m.passwordHash,
		PasswordSalt: m.passwordSalt,
		Tokens:       m.tokens,
		Secret:       base64.StdEncoding.EncodeToString(m.secret),
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.credPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.credPath())
}

// Mode 返回当前认证模式。
func (m *Manager) Mode() Mode { return m.mode }

// HasPassword 判断是否已设置本地口令。
func (m *Manager) HasPassword() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.passwordHash != ""
}

// SetPassword 设置本地模式的口令。传空字符串表示清除口令（免密局域网直连）。
func (m *Manager) SetPassword(password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if password == "" {
		m.passwordHash = ""
		m.passwordSalt = ""
		return m.save()
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	m.passwordSalt = hex.EncodeToString(salt)
	m.passwordHash = hashPassword(password, salt)
	return m.save()
}

func hashPassword(password string, salt []byte) string {
	sum := sha256.Sum256(append([]byte(password), salt...))
	// 多轮迭代提升抗暴力破解能力
	cur := sum[:]
	for i := 0; i < 10000; i++ {
		s := sha256.Sum256(append(cur, salt...))
		cur = s[:]
	}
	return hex.EncodeToString(cur)
}

// CheckPassword 校验口令。
func (m *Manager) CheckPassword(password string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.passwordHash == "" {
		return true
	}
	salt, err := hex.DecodeString(m.passwordSalt)
	if err != nil {
		return false
	}
	got := hashPassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(got), []byte(m.passwordHash)) == 1
}

// IssueToken 签发一个用于网页摘录的 API Token。
func (m *Manager) IssueToken(username string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	m.tokens[token] = username
	if err := m.save(); err != nil {
		return "", err
	}
	return token, nil
}

// RevokeTokens 吊销全部 API Token。
func (m *Manager) RevokeTokens() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens = map[string]string{}
	return m.save()
}

// ListTokens 返回已签发 Token 的用户名列表（不暴露 Token 本身）。
func (m *Manager) ListTokens() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.tokens))
	for _, u := range m.tokens {
		out = append(out, u)
	}
	return out
}

// LookupToken 校验 API Token。
func (m *Manager) LookupToken(token string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.tokens[token]
	return u, ok
}

// Authenticate 解析请求身份。
//
// 优先级：
//  1. API Token（X-CrazyNote-Token 头或 token 查询参数）——网页摘录扩展使用
//  2. fnOS 网关注入的身份头——仅在 unix socket 监听下可信
//  3. 会话 Cookie——本地模式登录后签发
func (m *Manager) Authenticate(r *http.Request, trustedGateway bool) (Identity, error) {
	// 1. API Token
	token := r.Header.Get("X-CrazyNote-Token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token != "" {
		if u, ok := m.LookupToken(token); ok {
			return Identity{Username: u, IsAdmin: false, Source: "token"}, nil
		}
		return Identity{}, errors.New("无效的访问令牌")
	}

	// 2. 网关注入的身份头
	if trustedGateway {
		if username := strings.TrimSpace(r.Header.Get(headerUsername)); username != "" {
			isAdmin := strings.EqualFold(r.Header.Get(headerIsAdmin), "1") ||
				strings.EqualFold(r.Header.Get(headerIsAdmin), "true")
			return Identity{Username: username, IsAdmin: isAdmin, Source: "gateway"}, nil
		}
	}

	// 3. 会话 Cookie
	if c, err := r.Cookie(sessionCookieName); err == nil {
		if id, ok := m.verifySession(c.Value); ok {
			return Identity{Username: id.Username, IsAdmin: id.IsAdmin, Source: "local"}, nil
		}
	}
	return Identity{}, errors.New("未认证")
}

type sessionData struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	Expires  int64  `json:"expires"`
}

// IssueSession 签发会话。
func (m *Manager) IssueSession(username string, isAdmin bool) (string, error) {
	data := sessionData{Username: username, IsAdmin: isAdmin, Expires: time.Now().Add(sessionTTL).Unix()}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	mac := m.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + mac, nil
}

func (m *Manager) verifySession(token string) (sessionData, bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return sessionData{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionData{}, false
	}
	if subtle.ConstantTimeCompare([]byte(m.sign(payload)), []byte(parts[1])) != 1 {
		return sessionData{}, false
	}
	var data sessionData
	if err := json.Unmarshal(payload, &data); err != nil {
		return sessionData{}, false
	}
	if time.Now().Unix() > data.Expires {
		return sessionData{}, false
	}
	return data, true
}

func (m *Manager) sign(payload []byte) string {
	h := sha256.New()
	h.Write(m.secret)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// SetSessionCookie 把会话写入响应。
func (m *Manager) SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ClearSessionCookie 清除会话。
func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// EnsureDefaults 在未设置口令时生成一个随机初始口令并返回（两种模式都生效）。
//
// 设计上：fnOS 桌面图标走统一网关、由网关注入身份，无需口令即可打开；
// 局域网直连（TCP）则需应用自身的口令。无论网关还是本地模式，这里都保证
// 存在一个初始口令，既避免局域网口“空口令裸奔”，又能让用户在日志里取到初始口令。
func (m *Manager) EnsureDefaults() (generatedPassword string, err error) {
	if m.HasPassword() {
		return "", nil
	}
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	password := base64.RawURLEncoding.EncodeToString(buf)
	if err := m.SetPassword(password); err != nil {
		return "", err
	}
	return password, nil
}

// Describe 返回给前端展示的认证状态描述。
func (m *Manager) Describe() map[string]any {
	return map[string]any{
		"mode":         string(m.mode),
		"has_password": m.HasPassword(),
		"token_users":  m.ListTokens(),
	}
}

// DefaultGatewayUser 是网关注入身份缺失时的兜底用户名。
const DefaultGatewayUser = "crazynote"

// EnsureGatewayIdentity 供内部调用，确保用户名非空。
func EnsureGatewayIdentity(id Identity) Identity {
	if id.Username == "" {
		id.Username = DefaultGatewayUser
	}
	return id
}

var _ = fmt.Sprintf
