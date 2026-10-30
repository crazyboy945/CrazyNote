package server

import (
	"net"
	"os"
)

// listenUnix 创建 Unix Socket 监听器。
//
// 在 Linux（fnOS 运行环境）上可用；Windows 上仅在系统支持 AF_UNIX 时可用，
// 不可用时返回错误，此时仅以 TCP 方式提供服务，不影响局域网使用。
func listenUnix(path string) (net.Listener, error) {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// 允许 fnOS 网关进程访问（通常是 root 或 trim 用户）
	_ = os.Chmod(path, 0o660)
	return ln, nil
}
