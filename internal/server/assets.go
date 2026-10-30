package server

import (
	"embed"
	"net/url"
)

// assetsFS 打包前端静态资源，单文件二进制无需外部依赖。
//
//go:embed web
var assetsFS embed.FS

func urlParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
