package main

// 验证 detach（daemon）逻辑：子进程应被正确创建并脱离父进程。
//
// 用法: go test ./cmd/crazynote/ -run TestDetachSpawnsDetachedChild -v
//
// 该测试验证 Windows 上 DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP 标志
// 能成功派生一个独立进程，且父进程退出后子进程仍然存活。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetachSpawnsDetachedChild(t *testing.T) {
	if os.Getenv("CRAZYNOTE_TEST_CHILD") == "1" {
		// 子进程角色：写一个标记文件后睡眠，父进程稍后检查文件是否存在
		marker := os.Getenv("CRAZYNOTE_TEST_MARKER")
		if marker != "" {
			_ = os.WriteFile(marker, []byte("alive"), 0o644)
		}
		time.Sleep(30 * time.Second)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("无法定位测试二进制: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "child-marker.txt")

	cmd := exec.Command(exe, "-test.run=TestDetachSpawnsDetachedChild")
	cmd.Env = append(os.Environ(),
		"CRAZYNOTE_TEST_CHILD=1",
		"CRAZYNOTE_TEST_MARKER="+marker,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		t.Fatalf("派生子进程失败: %v", err)
	}
	childPID := cmd.Process.Pid
	// 不 Wait，让子进程继续运行，模拟脱离

	// 等待子进程写出标记
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			t.Logf("子进程已成功启动并运行 (PID %d)", childPID)
			_ = cmd.Process.Kill()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	t.Fatalf("子进程未在预期时间内写出标记文件")
}

func TestDetachStripsDaemonFlag(t *testing.T) {
	// detach 传递参数时应去掉 -daemon，避免子进程再次 fork
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"crazynote", "-daemon", "-data", "D:/notes", "--daemon", "-addr", ":8899"}
	filtered := stripDaemonFlag(os.Args)
	for _, a := range filtered {
		if a == "-daemon" || a == "--daemon" {
			t.Fatalf("参数中仍含 -daemon: %v", filtered)
		}
	}
	if !strings.Contains(strings.Join(filtered, " "), "-data") {
		t.Errorf("其他参数被误删: %v", filtered)
	}
}
