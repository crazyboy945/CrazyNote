package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMetaFileNotDirectory 回归：早期版本用 MkdirAll(metaPath()) 把 meta.json
// 建成目录，之后保存元数据时又被改写成文件，导致再次启动时报
// "mkdir .../.crazynote/meta.json: not a directory" 而起不来。
func TestMetaFileNotDirectory(t *testing.T) {
	dir := t.TempDir()

	// 第一次启动：应正常创建，且 meta.json 必须是文件（不能是目录）
	s, err := New(dir)
	if err != nil {
		t.Fatalf("首次 New 失败: %v", err)
	}
	metaPath := filepath.Join(dir, ".crazynote", "meta.json")
	if fi, err := os.Stat(metaPath); err == nil && fi.IsDir() {
		t.Fatalf("meta.json 不应是目录")
	}

	// 保存元数据
	meta := s.LoadMeta()
	meta.Title = "我的笔记"
	meta.MarkdownTheme = "paper"
	if err := s.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta 失败: %v", err)
	}
	fi, err := os.Stat(metaPath)
	if err != nil {
		t.Fatalf("meta.json 应存在: %v", err)
	}
	if fi.IsDir() {
		t.Fatalf("meta.json 被写成了目录")
	}

	// 第二次启动（模拟重启）：必须能正常打开，且元数据能读回
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("二次 New 失败（这就是线上报的 not a directory）: %v", err)
	}
	got := s2.LoadMeta()
	if got.Title != "我的笔记" || got.MarkdownTheme != "paper" {
		t.Fatalf("元数据未持久化: %+v", got)
	}
}

// TestMetaDirectorySelfHeal 模拟老版本留下的脏状态：meta.json 是目录。
// 新版本启动时应自动清理，不应报错。
func TestMetaDirectorySelfHeal(t *testing.T) {
	dir := t.TempDir()
	metaDirPath := filepath.Join(dir, ".crazynote", "meta.json")
	if err := os.MkdirAll(metaDirPath, 0o755); err != nil {
		t.Fatalf("准备脏状态失败: %v", err)
	}

	s, err := New(dir)
	if err != nil {
		t.Fatalf("存在脏目录时 New 失败: %v", err)
	}
	// 脏目录应已被清理
	if fi, err := os.Stat(metaDirPath); err == nil && fi.IsDir() {
		t.Fatalf("meta.json 目录未被清理")
	}
	// 且能正常写入文件
	if err := s.SaveMeta(s.LoadMeta()); err != nil {
		t.Fatalf("清理后 SaveMeta 失败: %v", err)
	}
	if fi, err := os.Stat(metaDirPath); err != nil || fi.IsDir() {
		t.Fatalf("清理后 meta.json 应为文件")
	}
}

// TestMetaFileSurvivesRestart 模拟线上实际场景：meta.json 已是正常文件，
// 但它曾被误建成目录。启动不得因 MkdirAll 撞到文件而失败。
func TestMetaFileSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	if err := s.SaveMeta(s.LoadMeta()); err != nil {
		t.Fatalf("SaveMeta 失败: %v", err)
	}
	// 连续多次 New，都不应报错
	for i := 0; i < 3; i++ {
		if _, err := New(dir); err != nil {
			t.Fatalf("第 %d 次重启失败: %v", i+2, err)
		}
	}
}
