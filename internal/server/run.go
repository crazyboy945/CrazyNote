package server

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/local/crazynote/internal/auth"
	"github.com/local/crazynote/internal/config"
	"github.com/local/crazynote/internal/store"
)

// Run 启动 HTTP 服务，同时监听 TCP 端口与（可选的）Unix Socket。
//
// 两种访问方式：
//   - 网关模式：Unix Socket，由 fnOS 统一网关转发，网关注入可信身份头
//   - 局域网直连：TCP 端口 + 应用自身账号口令
//
// 同一个进程同时提供两者，网关优先。
func Run(cfg *config.Config) error {
	st, err := store.New(cfg.DataDir)
	if err != nil {
		return err
	}

	mode := auth.ModeLocal
	if cfg.Socket != "" {
		mode = auth.ModeGateway
	}
	am, err := auth.New(cfg.AppDataDir, mode)
	if err != nil {
		return err
	}
	if pwd, err := am.EnsureDefaults(); err != nil {
		return err
	} else if pwd != "" {
		log.Printf("已生成初始访问口令: %s （可在设置中修改）", pwd)
		// 额外输出纯 ASCII 行，便于 start.bat 在任意控制台代码页下提取口令
		log.Printf("INITIAL_PASSWORD=%s", pwd)
	}

	// 网关服务：可信身份头
	gatewaySrv := New(st, am)
	gatewaySrv.SetTrustedGateway(true)
	gatewaySrv.SetGatewayPrefix(cfg.Prefix)

	// 局域网直连服务：不信任任何客户端自带的身份头
	localSrv := New(st, am)
	localSrv.SetTrustedGateway(false)

	// 静态资源在网关下需要带前缀，index.html 里用相对路径即可，
	// 这里为网关服务额外挂一层前缀剥离已在 ServeHTTP 中完成。
	errCh := make(chan error, 2)

	// TCP 监听
	if cfg.Addr != "" {
		go func() {
			log.Printf("局域网直连: %s", accessHint(cfg.Addr))
			srv := &http.Server{
				Addr:              cfg.Addr,
				Handler:           localSrv,
				ReadHeaderTimeout: 15 * time.Second,
			}
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	// Unix Socket 监听（网关模式）
	if cfg.Socket != "" {
		go func() {
			if err := os.MkdirAll(filepath.Dir(cfg.Socket), 0o755); err != nil {
				errCh <- err
				return
			}
			// 清理残留 socket 文件
			if _, err := os.Stat(cfg.Socket); err == nil {
				_ = os.Remove(cfg.Socket)
			}
			ln, err := listenUnix(cfg.Socket)
			if err != nil {
				errCh <- err
				return
			}
			log.Printf("fnOS 统一网关: socket=%s prefix=%s", cfg.Socket, cfg.Prefix)
			srv := &http.Server{
				Handler:           gatewaySrv,
				ReadHeaderTimeout: 15 * time.Second,
			}
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	if cfg.Addr == "" && cfg.Socket == "" {
		return nil
	}
	return <-errCh
}

// accessHint 生成便于复制的访问地址提示。
//
// 监听 0.0.0.0 时提示用 <本机IP>，127.0.0.1 则直接给出地址。
func accessHint(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "http://<本机IP>:" + port
	case "127.0.0.1", "localhost", "::1":
		return "http://" + host + ":" + port
	default:
		return "http://" + net.JoinHostPort(host, port)
	}
}
