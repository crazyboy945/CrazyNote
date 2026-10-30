//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach 通过 fork 后父进程退出的方式脱离当前会话，
// 使服务在启动器（终端 / 脚本）关闭后继续运行。
func detach() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	cmd := exec.Command(exePath, stripDaemonFlag(os.Args[1:])...)
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// 父进程立即退出，子进程独立运行
	os.Exit(0)
	return nil
}
