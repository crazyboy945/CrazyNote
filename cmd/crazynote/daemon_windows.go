//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// Windows 进程创建标志
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// detach 脱离当前控制台，使进程在启动器退出后继续运行。
//
// Windows 没有 fork，需要借助 DETACHED_PROCESS 与 CREATE_NEW_PROCESS_GROUP
// 标志派生独立进程，并重新分配标准输出到 NUL，避免父进程退出后写入无效句柄。
func detach() error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	os.Stdout = null
	os.Stderr = null
	os.Stdin = null

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	// 去掉 -daemon 标志，避免子进程再次脱离；
	// 其余参数（含 -log）保留，让子进程自己写日志文件。
	cmd := exec.Command(exePath, stripDaemonFlag(os.Args[1:])...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	// 父进程立即退出，子进程独立运行
	os.Exit(0)
	return nil
}
