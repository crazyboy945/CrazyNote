package store

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/crazynote/internal/model"
)

// buildFixture 建一个含分类、笔记、图片与元数据的笔记目录。
func buildFixture(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	// 分类 + 笔记
	for _, cat := range []string{"技术", "读书"} {
		if err := s.CreateCategory(cat); err != nil {
			t.Fatalf("建分类失败: %v", err)
		}
	}
	if _, err := s.SaveNote(noteOf("技术", "深入理解 Go 的并发模型", "# 标题\n\n正文")); err != nil {
		t.Fatalf("存笔记失败: %v", err)
	}
	if _, err := s.SaveNote(noteOf("读书", "书摘", "内容")); err != nil {
		t.Fatalf("存笔记失败: %v", err)
	}
	// 图片
	if _, err := s.SaveImage("图.png", []byte("PNGDATA")); err != nil {
		t.Fatalf("存图片失败: %v", err)
	}
	// 元数据
	meta := s.LoadMeta()
	meta.Title = "我的笔记"
	meta.MarkdownTheme = "paper"
	if err := s.SaveMeta(meta); err != nil {
		t.Fatalf("存元数据失败: %v", err)
	}
	return s
}

func noteOf(cat, title, body string) model.Note {
	return model.Note{Category: cat, Title: title, Body: body}
}

// TestExportBackup 导出的 ZIP 应包含所有分类、笔记、图片与元数据。
func TestExportBackup(t *testing.T) {
	s := buildFixture(t)

	var buf bytes.Buffer
	stats, err := s.ExportBackup(&buf)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if stats.Notes != 2 {
		t.Errorf("笔记数 = %d, 期望 2", stats.Notes)
	}
	if stats.Categories != 2 {
		t.Errorf("分类数 = %d, 期望 2", stats.Categories)
	}
	if stats.Images != 1 {
		t.Errorf("图片数 = %d, 期望 1", stats.Images)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("读取 ZIP 失败: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{
		"技术/深入理解 Go 的并发模型.md",
		"读书/书摘.md",
		".crazynote/meta.json",
	} {
		if !names[want] {
			t.Errorf("ZIP 中缺少 %q，现有条目: %v", want, names)
		}
	}
	// 图片在 .crazynote/assets/images/ 下
	found := false
	for n := range names {
		if strings.Contains(n, "assets/images/") && strings.HasSuffix(n, ".png") {
			found = true
		}
	}
	if !found {
		t.Errorf("ZIP 中缺少图片条目: %v", names)
	}
}

// TestRestoreBackup 恢复后内容应与备份一致，且原内容被快照保留。
func TestRestoreBackup(t *testing.T) {
	src := buildFixture(t)
	var buf bytes.Buffer
	if _, err := src.ExportBackup(&buf); err != nil {
		t.Fatalf("导出失败: %v", err)
	}

	// 目标目录：先放一些"将被覆盖"的内容
	dstDir := t.TempDir()
	dst, err := New(dstDir)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	if err := dst.CreateCategory("旧分类"); err != nil {
		t.Fatalf("建分类失败: %v", err)
	}
	if _, err := dst.SaveNote(noteOf("旧分类", "旧笔记", "旧内容")); err != nil {
		t.Fatalf("存笔记失败: %v", err)
	}

	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写 ZIP 失败: %v", err)
	}

	stats, err := dst.RestoreBackup(zipPath)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if stats.Notes != 2 {
		t.Errorf("恢复后笔记数 = %d, 期望 2", stats.Notes)
	}
	if stats.Categories != 2 {
		t.Errorf("恢复后分类数 = %d, 期望 2", stats.Categories)
	}
	// 旧分类应已被覆盖掉
	if _, err := os.Stat(filepath.Join(dstDir, "旧分类")); !os.IsNotExist(err) {
		t.Errorf("旧分类未被覆盖")
	}
	// 新内容到位
	if _, err := dst.GetNote("技术", "深入理解 Go 的并发模型.md"); err != nil {
		t.Errorf("恢复后的笔记读不到: %v", err)
	}
	// 元数据也应恢复
	if meta := dst.LoadMeta(); meta.MarkdownTheme != "paper" {
		t.Errorf("样式偏好未恢复: %q", meta.MarkdownTheme)
	}
	// 快照目录应存在且保留旧内容
	if stats.Snapshot == "" {
		t.Fatal("未返回快照路径")
	}
	if _, err := os.Stat(filepath.Join(stats.Snapshot, "旧分类", "旧笔记.md")); err != nil {
		t.Errorf("快照里没有旧笔记: %v", err)
	}
}

// TestRestoreRejectsPathTraversal 压缩包内的路径穿越必须被拒绝，且**不动现有数据**。
func TestRestoreRejectsPathTraversal(t *testing.T) {
	cases := []string{
		"../evil.md",
		"技术/../../evil.md",
		"/etc/passwd",
		`C:\Windows\evil.md`,
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			s := buildFixture(t)
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			w, err := zw.Create(name)
			if err != nil {
				t.Fatalf("写 ZIP 条目失败: %v", err)
			}
			w.Write([]byte("pwned"))
			zw.Close()

			zipPath := filepath.Join(t.TempDir(), "bad.zip")
			os.WriteFile(zipPath, buf.Bytes(), 0o644)

			if _, err := s.RestoreBackup(zipPath); err == nil {
				t.Fatalf("路径穿越未被拒绝: %s", name)
			}
			// 关键：现有数据必须完好
			if _, err := s.GetNote("技术", "深入理解 Go 的并发模型.md"); err != nil {
				t.Errorf("拒绝后现有笔记被破坏: %v", err)
			}
			// 也不能在根目录外留下文件
			outside := filepath.Join(filepath.Dir(s.Root()), "evil.md")
			if _, err := os.Stat(outside); err == nil {
				t.Errorf("路径穿越文件被写到了根目录外: %s", outside)
				os.Remove(outside)
			}
		})
	}
}

// TestRestoreRejectsCorruptZip 损坏的备份包应报错且不破坏现有数据。
func TestRestoreRejectsCorruptZip(t *testing.T) {
	s := buildFixture(t)
	zipPath := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(zipPath, []byte("这不是一个 ZIP 文件"), 0o644)
	if _, err := s.RestoreBackup(zipPath); err == nil {
		t.Fatal("损坏的 ZIP 未被拒绝")
	}
	if _, err := s.GetNote("技术", "深入理解 Go 的并发模型.md"); err != nil {
		t.Errorf("拒绝后现有笔记被破坏: %v", err)
	}
}

// TestBackupExcludesRestoreSnapshots 备份不应把历史恢复快照再打包进去（否则会无限膨胀）。
func TestBackupExcludesRestoreSnapshots(t *testing.T) {
	s := buildFixture(t)
	// 造一个历史快照
	snap := filepath.Join(s.RestoreBackupsDir(), "20260101-000000")
	if err := os.MkdirAll(filepath.Join(snap, "技术"), 0o755); err != nil {
		t.Fatalf("造快照失败: %v", err)
	}
	os.WriteFile(filepath.Join(snap, "技术", "旧.md"), []byte("x"), 0o644)

	var buf bytes.Buffer
	if _, err := s.ExportBackup(&buf); err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("读 ZIP 失败: %v", err)
	}
	for _, f := range zr.File {
		if strings.Contains(f.Name, "restore-backups") {
			t.Errorf("备份里混入了历史快照: %s", f.Name)
		}
	}
}

// TestRestoreEmptyNotesDir 空目录恢复后内容正确。
func TestRestoreEmptyNotesDir(t *testing.T) {
	src := buildFixture(t)
	var buf bytes.Buffer
	src.ExportBackup(&buf)

	dst, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	zipPath := filepath.Join(t.TempDir(), "b.zip")
	os.WriteFile(zipPath, buf.Bytes(), 0o644)
	stats, err := dst.RestoreBackup(zipPath)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if stats.Notes != 2 || stats.Images != 1 {
		t.Errorf("恢复统计不对: %+v", stats)
	}
}
