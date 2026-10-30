package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/local/crazynote/internal/config"
	"github.com/local/crazynote/internal/server"
)

var version = "1.0.0"

func main() {
	var (
		dataDir    = flag.String("data", "", "笔记根目录（每个分类是其中的一个子目录）")
		appDataDir = flag.String("appdata", "", "配置目录（保存口令哈希与会话密钥）")
		addr       = flag.String("addr", "", "局域网监听地址，如 :8899 或 0.0.0.0:8899")
		socket     = flag.String("socket", "", "fnOS 统一网关 Unix Socket 路径")
		prefix     = flag.String("prefix", "", "网关路径前缀，默认 /app/crazynote")
		perUser    = flag.Bool("per-user", false, "按用户隔离笔记目录")
		daemon     = flag.Bool("daemon", false, "后台运行（脱离当前终端/窗口）")
		logFile    = flag.String("log", "", "日志文件路径（后台模式下建议指定）")
		showVer    = flag.Bool("version", false, "打印版本号")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("CrazyNote " + version)
		return
	}

	// 把版本号交给 server 包，界面设置弹窗里会显示当前版本
	server.AppVersion = version

	// 后台模式：重新以脱离方式启动自身，然后当前进程退出
	if *daemon {
		if err := detach(); err != nil {
			log.Fatalf("后台启动失败: %v", err)
		}
		return
	}

	// 指定日志文件时，同时写入文件（后台模式下 stdout 不可用）
	if *logFile != "" {
		if f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			log.SetOutput(io.MultiWriter(os.Stdout, f))
		} else {
			log.Printf("打开日志文件失败: %v", err)
		}
	}

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[CrazyNote] ")

	cfg := config.Resolve(*dataDir, *appDataDir, *addr, *socket, *prefix, *perUser)

	log.Printf("版本 %s", version)
	log.Printf("笔记目录: %s", cfg.DataDir)
	log.Printf("配置目录: %s", cfg.AppDataDir)
	if cfg.Socket != "" {
		log.Printf("网关模式: socket=%s prefix=%s", cfg.Socket, cfg.Prefix)
	}
	log.Printf("局域网监听: %s", cfg.Addr)

	if err := server.Run(cfg); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	os.Exit(0)
}
