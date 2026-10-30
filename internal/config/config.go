package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 汇总运行期配置。优先级：命令行 > 环境变量 > 向导写入的配置文件 > 默认值。
type Config struct {
	DataDir    string // 笔记根目录，每个分类是一个子目录
	AppDataDir string // 配置与状态目录（密码哈希、会话密钥、API Token）
	Addr       string // 局域网 TCP 监听地址
	Socket     string // fnOS 统一网关 Unix Socket 路径
	Prefix     string // 网关路径前缀，例如 /app/crazynote
	PerUser    bool   // 是否按用户隔离笔记目录
}

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// Resolve 合并各来源的配置。向导在安装时把用户选择的笔记目录写入
// <appData>/data-dir，这里优先读取它。
func Resolve(dataDir, appDataDir, addr, socket, prefix string, perUser bool) *Config {
	cfg := &Config{
		AppDataDir: appDataDir,
		Addr:       addr,
		Socket:     socket,
		Prefix:     prefix,
		PerUser:    perUser,
	}

	cfg.AppDataDir = firstNonEmpty(
		cfg.AppDataDir,
		envStr("CRAZYNOTE_APPDATA", ""),
		pathFromEnv("TRIM_PKGETC", "crazynote"),
		defaultUserDir(),
	)
	cfg.AppDataDir = filepath.Clean(cfg.AppDataDir)

	cfg.DataDir = firstNonEmpty(
		dataDir,
		envStr("CRAZYNOTE_DATA_DIR", ""),
		readDataDirFile(cfg.AppDataDir),
		pathFromEnv("TRIM_PKGVAR", "notes"),
		filepath.Join(cfg.AppDataDir, "notes"),
	)
	cfg.DataDir = filepath.Clean(cfg.DataDir)

	cfg.Addr = firstNonEmpty(cfg.Addr, envStr("CRAZYNOTE_ADDR", ""), envStr("CRAZYNOTE_PORT", ""), ":8899")
	if !strings.Contains(cfg.Addr, ":") {
		cfg.Addr = ":" + cfg.Addr
	}
	cfg.Socket = firstNonEmpty(cfg.Socket, envStr("CRAZYNOTE_SOCKET", ""))
	cfg.Prefix = firstNonEmpty(cfg.Prefix, envStr("CRAZYNOTE_PREFIX", ""), "/app/crazynote")
	cfg.Prefix = "/" + strings.Trim(cfg.Prefix, "/")
	cfg.PerUser = perUser || envBool("CRAZYNOTE_PER_USER", false)

	return cfg
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func pathFromEnv(envKey, sub string) string {
	base := strings.TrimSpace(os.Getenv(envKey))
	if base == "" {
		return ""
	}
	return filepath.Join(base, sub)
}

func readDataDirFile(appDataDir string) string {
	b, err := os.ReadFile(filepath.Join(appDataDir, "data-dir"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func defaultUserDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "crazynote")
	}
	return filepath.Join(".", ".crazynote")
}

// Port 从 Addr 中提取纯数字端口，供 manifest 使用。
func (c *Config) Port() string {
	idx := strings.LastIndex(c.Addr, ":")
	if idx < 0 {
		return ""
	}
	port := c.Addr[idx+1:]
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}
	return port
}
