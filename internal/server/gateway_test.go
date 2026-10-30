package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/crazynote/internal/auth"
	"github.com/local/crazynote/internal/store"
)

// newTestServer 建一套临时的存储与认证管理器。
func newTestServer(t *testing.T) (*store.Store, *auth.Manager) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "notes"))
	if err != nil {
		t.Fatalf("store.New 失败: %v", err)
	}
	am, err := auth.New(filepath.Join(dir, "appdata"), auth.ModeLocal)
	if err != nil {
		t.Fatalf("auth.New 失败: %v", err)
	}
	return st, am
}

// TestGatewayPrefixInjection 验证网关模式下注入前端的 BASE 前缀是否正确。
//
// fnOS 统一网关会把完整路径（含 /app/{appname} 前缀）转发给应用，
// ServeHTTP 负责剥离前缀。但剥离发生在路由之前，handleIndex 里再判断
// "路径是否以 prefix 开头" 就永远为 false —— 前缀会被注入成空串，
// 导致前端把 /assets/app.js、/api/xxx 请求到域名根路径，网关路由不到。
func TestGatewayPrefixInjection(t *testing.T) {
	st, am := newTestServer(t)
	s := New(st, am)
	s.SetTrustedGateway(true)
	s.SetGatewayPrefix("/app/crazynote")

	// 网关转发过来的完整路径
	req := httptest.NewRequest("GET", "/app/crazynote/", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `window.CRAZYNOTE_BASE = "/app/crazynote"`) {
		t.Errorf("注入的 BASE 前缀不正确，实际为：\n%s", extractBase(body))
	}
}

// TestGatewayPrefixStripping 验证带前缀的静态资源与 API 路径能被正确剥离并路由。
func TestGatewayPrefixStripping(t *testing.T) {
	st, am := newTestServer(t)
	s := New(st, am)
	s.SetTrustedGateway(true)
	s.SetGatewayPrefix("/app/crazynote")

	// 前端静态资源
	req := httptest.NewRequest("GET", "/app/crazynote/assets/app.js", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("静态资源状态码 = %d, 期望 200", rec.Code)
	}

	// API 路径应命中 /api/health
	req2 := httptest.NewRequest("GET", "/app/crazynote/api/health", nil)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("API 状态码 = %d, 期望 200", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `"status"`) {
		t.Errorf("API 未命中 health 处理器: %s", rec2.Body.String())
	}
}

// TestImageRouteWinsOverAsset 验证 /assets/images/{file} 不会被 /assets/{path...} 抢走。
func TestImageRouteWinsOverAsset(t *testing.T) {
	st, am := newTestServer(t)
	s := New(st, am)
	s.SetTrustedGateway(false)

	url, err := st.SaveImage("probe.png", []byte("PNGDATA"))
	if err != nil {
		t.Fatalf("SaveImage 失败: %v", err)
	}
	// 存一个会话，免登录令
	tok, err := am.IssueSession("admin", true)
	if err != nil {
		t.Fatalf("签发会话失败: %v", err)
	}

	req := httptest.NewRequest("GET", url, nil)
	req.AddCookie(&http.Cookie{Name: "crazynote_session", Value: tok})
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("图片状态码 = %d, 期望 200（路由可能被 /assets/{path...} 抢走）", rec.Code)
	}
	if rec.Body.String() != "PNGDATA" {
		t.Errorf("图片内容不对: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, 期望 image/png", ct)
	}
}

// extractBase 从 HTML 中取出注入的 BASE 值，便于报错展示。
func extractBase(html string) string {
	const marker = `window.CRAZYNOTE_BASE = "`
	i := strings.Index(html, marker)
	if i < 0 {
		return "(未找到 window.CRAZYNOTE_BASE)"
	}
	rest := html[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return "(未闭合)"
	}
	return `"` + rest[:j] + `"`
}

// TestRenderRewriteAssetURL 验证渲染时会给应用内图片地址补上网关前缀，
// 而局域网直连模式下保持原样（笔记里的地址不随访问方式变化）。
func TestRenderRewriteAssetURL(t *testing.T) {
	// 网关模式：应补前缀
	gw := New(nil, nil)
	gw.SetGatewayPrefix("/app/crazynote")
	got := gw.rewriteAssetURLs(`<img src="/assets/images/%E6%B5%8B.png" />`)
	if !strings.Contains(got, `src="/app/crazynote/assets/images/`) {
		t.Errorf("网关模式未补前缀: %s", got)
	}

	// 局域网直连：保持原样
	local := New(nil, nil)
	got2 := local.rewriteAssetURLs(`<img src="/assets/images/%E6%B5%8B.png" />`)
	if got2 != `<img src="/assets/images/%E6%B5%8B.png" />` {
		t.Errorf("直连模式不应改写: %s", got2)
	}
}

// TestRenderKeepsExternalImages 验证外部图片地址不会被误改写。
func TestRenderKeepsExternalImages(t *testing.T) {
	s := New(nil, nil)
	s.SetGatewayPrefix("/app/crazynote")
	in := `<img src="https://example.com/a.png" />` + `<img src="/other/x.png" />`
	got := s.rewriteAssetURLs(in)
	if got != in {
		t.Errorf("非应用内图片被误改写: %s", got)
	}
}
