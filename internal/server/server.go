package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/local/crazynote/internal/auth"
	"github.com/local/crazynote/internal/clip"
	"github.com/local/crazynote/internal/model"
	"github.com/local/crazynote/internal/render"
	"github.com/local/crazynote/internal/store"
)

// Server 是 HTTP 应用。
type Server struct {
	store *store.Store
	auth  *auth.Manager
	mux   *http.ServeMux

	// trustedGateway 标记当前监听器是否为 Unix Socket（网关模式）。
	// 只有 Unix Socket 上的网关注入头才可信，TCP 端口上的同名头一律忽略。
	trustedGateway bool

	// gatewayPrefix 网关路径前缀，需要从请求路径中剥离。
	gatewayPrefix string
}

// New 创建服务。
func New(st *store.Store, am *auth.Manager) *Server {
	s := &Server{
		store: st,
		auth:  am,
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

// SetTrustedGateway 标记该 handler 由 Unix Socket 提供服务。
func (s *Server) SetTrustedGateway(v bool) { s.trustedGateway = v }

// SetGatewayPrefix 设置网关路径前缀。
func (s *Server) SetGatewayPrefix(p string) { s.gatewayPrefix = p }

func (s *Server) routes() {
	// API
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/session", s.handleSession)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)

	s.mux.HandleFunc("GET /api/categories", s.handleListCategories)
	s.mux.HandleFunc("POST /api/categories", s.handleCreateCategory)
	s.mux.HandleFunc("PUT /api/categories/{name}", s.handleRenameCategory)
	s.mux.HandleFunc("DELETE /api/categories/{name}", s.handleDeleteCategory)

	s.mux.HandleFunc("GET /api/notes", s.handleListNotes)
	s.mux.HandleFunc("POST /api/notes", s.handleCreateNote)
	s.mux.HandleFunc("GET /api/notes/{category}/{file}", s.handleGetNote)
	s.mux.HandleFunc("PUT /api/notes/{category}/{file}", s.handleSaveNote)
	s.mux.HandleFunc("DELETE /api/notes/{category}/{file}", s.handleDeleteNote)
	s.mux.HandleFunc("POST /api/notes/{category}/{file}/move", s.handleMoveNote)

	s.mux.HandleFunc("GET /api/tags", s.handleTags)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)

	// Markdown 渲染（代码块带语法高亮）
	s.mux.HandleFunc("POST /api/render", s.handleRender)
	s.mux.HandleFunc("GET /api/highlight.css", s.handleHighlightCSS)

	// 图片上传与访问
	s.mux.HandleFunc("POST /api/upload", s.handleUpload)
	s.mux.HandleFunc("GET /assets/images/{file}", s.handleImage)

	// 备份与恢复
	s.mux.HandleFunc("GET /api/backup", s.handleBackup)
	s.mux.HandleFunc("POST /api/restore", s.handleRestore)

	// 网页摘录接口（扩展与书签脚本调用）
	s.mux.HandleFunc("POST /api/clip", s.handleClip)
	s.mux.HandleFunc("POST /api/clip/preview", s.handleClipPreview)

	// 设置与令牌
	s.mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.mux.HandleFunc("PUT /api/settings", s.handleSaveSettings)
	s.mux.HandleFunc("POST /api/tokens", s.handleIssueToken)
	s.mux.HandleFunc("DELETE /api/tokens", s.handleRevokeTokens)

	// 静态前端
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /assets/{path...}", s.handleAsset)
	s.mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	s.mux.HandleFunc("GET /favicon.png", s.handleFavicon)
}

// ServeHTTP 处理请求，含路径前缀剥离与访问日志。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 剥离网关前缀：/app/crazynote/api/xxx -> /api/xxx
	if s.gatewayPrefix != "" {
		if strings.HasPrefix(r.URL.Path, s.gatewayPrefix) {
			trimmed := strings.TrimPrefix(r.URL.Path, s.gatewayPrefix)
			if trimmed == "" {
				trimmed = "/"
			}
			r.URL.Path = trimmed
		}
	}
	// 网关模式下，静态资源与 API 需要带前缀回写，前端才能正确加载
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// 单个请求 panic 不应影响整个服务
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("请求 %s %s 处理异常: %v", r.Method, r.URL.Path, rec)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "服务内部错误"})
		}
	}()

	s.mux.ServeHTTP(w, r)
}

// ---------- 认证 ----------

// requireAuth 校验身份，通过后把身份写入上下文。
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id, err := s.auth.Authenticate(r, s.trustedGateway)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error":  "未认证",
			"reason": err.Error(),
			"mode":   string(s.auth.Mode()),
		})
		return auth.Identity{}, false
	}
	return auth.EnsureGatewayIdentity(id), true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":     id.Username,
		"is_admin": id.IsAdmin,
		"source":   id.Source,
		"mode":     string(s.auth.Mode()),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// 桌面图标走统一网关（trustedGateway=true）时，Authenticate 会直接采用
	// 网关注入的身份，不会走到这里；此处仅服务局域网直连（TCP）的口令登录。
	// 未设置口令时 CheckPassword("") 直接放行（即“免密局域网直连”）。
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if !s.auth.CheckPassword(req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "口令不正确"})
		return
	}
	token, err := s.auth.IssueSession("admin", true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.auth.SetSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": "admin", "is_admin": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 分类 ----------

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	cats, err := s.store.ListCategories()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.store.CreateCategory(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cats, _ := s.store.ListCategories()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "categories": cats})
}

func (s *Server) handleRenameCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.store.RenameCategory(r.PathValue("name"), req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cats, _ := s.store.ListCategories()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "categories": cats})
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if err := s.store.DeleteCategory(r.PathValue("name")); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cats, _ := s.store.ListCategories()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "categories": cats})
}

// ---------- 笔记 ----------

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	category := r.URL.Query().Get("category")
	notes, err := s.store.ListNotes(category)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// 列表接口不返回正文，减小传输量
	type brief struct {
		ID       string    `json:"id"`
		Title    string    `json:"title"`
		Created  time.Time `json:"created"`
		Updated  time.Time `json:"updated"`
		Category string    `json:"category"`
		Tags     []string  `json:"tags"`
		Source   string    `json:"source"`
		File     string    `json:"file"`
	}
	briefs := make([]brief, 0, len(notes))
	for _, n := range notes {
		briefs = append(briefs, brief{
			ID: n.ID, Title: n.Title, Created: n.Created, Updated: n.Updated,
			Category: n.Category, Tags: n.Tags, Source: n.Source, File: n.File,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": briefs})
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	n, err := s.store.GetNote(r.PathValue("category"), r.PathValue("file"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req struct {
		Category string   `json:"category"`
		Title    string   `json:"title"`
		Body     string   `json:"body"`
		Tags     []string `json:"tags"`
		Source   string   `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if req.Category == "" {
		req.Category = "默认"
	}
	if err := s.store.CreateCategory(req.Category); err != nil && !strings.Contains(err.Error(), "已存在") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	n, err := s.store.SaveNote(model.Note{
		Category: req.Category,
		Title:    req.Title,
		Body:     req.Body,
		Tags:     req.Tags,
		Source:   req.Source,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	category := r.PathValue("category")
	file := r.PathValue("file")
	var req struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Tags   []string `json:"tags"`
		Source string   `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	existing, err := s.store.GetNote(category, file)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	existing.Title = req.Title
	existing.Body = req.Body
	existing.Tags = req.Tags
	if req.Source != "" {
		existing.Source = req.Source
	}
	n, err := s.store.SaveNote(existing)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if err := s.store.DeleteNote(r.PathValue("category"), r.PathValue("file")); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMoveNote(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	n, err := s.store.MoveNote(r.PathValue("category"), r.PathValue("file"), req.Target)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// ---------- 标签与搜索 ----------

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	tags, err := s.store.AllTags()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	q := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	notes, err := s.store.Search(q, category)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes, "count": len(notes)})
}

// ---------- 网页摘录 ----------

type clipRequest struct {
	Category string   `json:"category"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	HTML     string   `json:"html"`
	Markdown string   `json:"markdown"`
	Body     string   `json:"body"`
	Excerpt  string   `json:"excerpt"`
	Tags     []string `json:"tags"`
	// Selection 为用户选中的文本片段，优先作为正文
	Selection string `json:"selection"`
}

// handleClip 接收网页摘录并落盘。
func (s *Server) handleClip(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	var req clipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}

	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Markdown)
	if body == "" {
		body = strings.TrimSpace(req.Body)
	}
	source := strings.TrimSpace(req.URL)

	// 有 HTML 就先提取正文
	if req.HTML != "" {
		res, err := clip.Extract(req.HTML, source, title)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if body == "" {
			body = res.Markdown
		}
		if title == "" {
			title = res.Title
		}
	}
	// 选中文本优先（用户在页面上框选的一段内容）
	if sel := strings.TrimSpace(req.Selection); sel != "" {
		body = sel
	}
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "摘录内容为空"})
		return
	}
	if title == "" {
		title = deriveTitle(source, body)
	}
	if req.Excerpt != "" {
		body = "> " + strings.ReplaceAll(strings.TrimSpace(req.Excerpt), "\n", "\n> ") + "\n\n" + body
	}
	if source != "" {
		body = "来源: [" + linkTitle(title, source) + "](" + source + ")\n\n" + body
	}

	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "摘录"
	}
	if err := s.store.CreateCategory(category); err != nil && !strings.Contains(err.Error(), "已存在") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	n, err := s.store.SaveNote(model.Note{
		Category: category,
		Title:    title,
		Body:     body,
		Tags:     req.Tags,
		Source:   source,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"id":       n.ID,
		"title":    n.Title,
		"file":     n.File,
		"category": n.Category,
		"user":     id.Username,
	})
}

// handleClipPreview 只解析不落盘，供扩展预览摘录效果。
func (s *Server) handleClipPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req clipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	res, err := clip.Extract(req.HTML, req.URL, req.Title)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------- 设置与令牌 ----------

// AppVersion 应用版本号，由 main 包在启动时注入（便于界面显示当前版本）。
var AppVersion = "dev"

// markdownThemes 支持的 Markdown 样式主题。
// 前端与此处保持一致；非法值一律忽略（保留当前设置），避免旧版本客户端写入脏数据。
var markdownThemes = map[string]bool{
	"default": true,
	"reading": true,
	"paper":   true,
	"card":    true,
	"compact": true,
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	meta := s.store.LoadMeta()
	writeJSON(w, http.StatusOK, map[string]any{
		"title":          meta.Title,
		"root":           s.store.Root(),
		"auth":           s.auth.Describe(),
		"category_order": meta.CategoryOrder,
		"markdown_theme": meta.MarkdownTheme,
		"version":        AppVersion,
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req struct {
		Title         string   `json:"title"`
		CategoryOrder []string `json:"category_order"`
		Password      string   `json:"password"`
		MarkdownTheme string   `json:"markdown_theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	meta := s.store.LoadMeta()
	if req.Title != "" {
		meta.Title = req.Title
	}
	if req.CategoryOrder != nil {
		meta.CategoryOrder = req.CategoryOrder
	}
	// Markdown 样式主题：只接受已知取值，非法值忽略（保留原设置）
	if req.MarkdownTheme != "" && markdownThemes[req.MarkdownTheme] {
		meta.MarkdownTheme = req.MarkdownTheme
	}
	if err := s.store.SaveMeta(meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// 口令变更：传空字符串表示保持不变，特殊值 "__clear__" 表示关闭口令校验
	if req.Password != "" {
		if req.Password == "__clear__" {
			if err := s.auth.SetPassword(""); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		} else {
			if err := s.auth.SetPassword(req.Password); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	username := req.Username
	if username == "" {
		username = id.Username
	}
	token, err := s.auth.IssueToken(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "username": username})
}

func (s *Server) handleRevokeTokens(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if err := s.auth.RevokeTokens(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 静态资源 ----------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := assetsFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "界面资源缺失", http.StatusInternalServerError)
		return
	}
	// 网关模式下，请求路径已被 ServeHTTP 剥离过前缀，这里不能再用
	// "路径是否以 prefix 开头"来判断（剥离后必然不成立），
	// 直接使用配置的前缀注入给前端，保证前端请求带前缀能命中网关路由。
	prefix := s.gatewayPrefix
	html := strings.ReplaceAll(string(data), "__CRAZYNOTE_BASE__", prefix)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(html))
}

// handleFavicon 提供站点图标，避免浏览器请求 /favicon.ico 返回 404。
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	data, err := assetsFS.ReadFile("web/favicon.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	p := path.Clean("/" + r.PathValue("path"))
	name := strings.TrimPrefix(p, "/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	// embed.FS 的路径分隔符固定为 "/"，不能用 filepath.Join
	data, err := assetsFS.ReadFile(path.Join("web", name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(name, ".png"):
		w.Header().Set("Content-Type", "image/png")
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// ---------- 工具 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

func deriveTitle(source, body string) string {
	if source != "" {
		if u, err := urlParse(source); err == nil && u.Host != "" {
			return u.Host + " 摘录"
		}
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, ">") {
			if len(line) > 40 {
				line = line[:40] + "…"
			}
			return line
		}
	}
	return "网页摘录"
}

func linkTitle(title, link string) string {
	if title == "" {
		return link
	}
	return title
}

// handleRender 渲染 Markdown 为 HTML，代码块带语法高亮。
//
// 前端预览时调用，避免在浏览器端重复实现 Markdown 解析。
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	var req struct {
		Markdown string `json:"markdown"`
		Body     string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	src := req.Markdown
	if src == "" {
		src = req.Body
	}
	writeJSON(w, http.StatusOK, map[string]any{"html": s.rewriteAssetURLs(render.Markdown(src))})
}

// appAssetSrcRe 匹配渲染结果里指向应用内图片目录的绝对路径。
var appAssetSrcRe = regexp.MustCompile(`src="/assets/images/`)

// rewriteAssetURLs 给渲染结果里的应用内图片地址补上网关前缀。
//
// 笔记正文里保存的是稳定的 /assets/images/xxx.png（与访问方式无关，
// 换局域网直连或用别的工具打开都还是同一个路径）；
// 但网关模式下浏览器需要 /app/crazynote/assets/images/xxx.png 才能命中网关路由，
// 所以在渲染时按需补前缀。
func (s *Server) rewriteAssetURLs(html string) string {
	if s.gatewayPrefix == "" {
		return html
	}
	return appAssetSrcRe.ReplaceAllString(html, `src="`+s.gatewayPrefix+`/assets/images/`)
}

// handleHighlightCSS 返回语法高亮主题样式。
//
// 该接口无需认证：只包含配色规则，不含任何用户数据。
func (s *Server) handleHighlightCSS(w http.ResponseWriter, r *http.Request) {
	theme := r.URL.Query().Get("theme")
	css := render.ThemeCSS(theme)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(css))
}

// handleUpload 接收 multipart 表单中的图片，落盘后返回可访问的 URL。
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	// 限制上传体积，避免被大文件拖垮（10MB）。
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "文件过大或表单格式错误"})
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未找到上传的图片"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "读取文件失败"})
		return
	}
	url, err := s.store.SaveImage(header.Filename, data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"url":  url,
		"name": filepath.Base(url),
	})
}

// handleImage 提供已上传图片的静态访问，需认证后方可读取。
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	name := r.PathValue("file")
	// 路径穿越防护：只允许基础文件名。
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(s.store.ImagePath(name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ctype := detectImageContentType(name, data); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// detectImageContentType 根据扩展名（或文件头）返回图片 MIME 类型。
func detectImageContentType(name string, data []byte) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	}
	return http.DetectContentType(data)
}

// ---------- 备份与恢复 ----------

// handleBackup 导出整个笔记目录为 ZIP，供用户下载保存。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	// 先打包到临时文件：既能在出错时干净地返回错误，也能给出准确的文件大小
	tmp, err := os.CreateTemp("", "crazynote-backup-*.zip")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "创建临时文件失败: " + err.Error()})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	stats, err := s.store.ExportBackup(tmp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "打包失败: " + err.Error()})
		return
	}
	fi, err := tmp.Stat()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	name := fmt.Sprintf("crazynote-backup-%s.zip", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-CrazyNote-Backup-Files", strconv.Itoa(stats.Files))
	w.Header().Set("X-CrazyNote-Backup-Notes", strconv.Itoa(stats.Notes))
	http.ServeContent(w, r, name, fi.ModTime(), tmp)
	log.Printf("导出备份完成: %s（%d 个文件 / %d 篇笔记 / %d 张图片）", name, stats.Files, stats.Notes, stats.Images)
}

// handleRestore 从上传的 ZIP 恢复笔记。
//
// 这是破坏性操作（会覆盖现有笔记），因此要求请求带上确认标记，
// 并在恢复前自动为现有内容创建快照。
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Header.Get("X-CrazyNote-Confirm") != "restore" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少恢复确认标记"})
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "备份文件过大或格式不正确"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未找到备份文件"})
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "crazynote-restore-*.zip")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "创建临时文件失败: " + err.Error()})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, file); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "读取备份文件失败: " + err.Error()})
		return
	}

	stats, err := s.store.RestoreBackup(tmp.Name())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	log.Printf("恢复完成: %s（%d 个文件 / %d 篇笔记），快照: %s", header.Filename, stats.Files, stats.Notes, stats.Snapshot)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"stats": stats,
	})
}

var _ = errors.New
var _ = fmt.Sprintf
var _ = os.Getenv
var _ = strconv.Itoa
