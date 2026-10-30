package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/local/crazynote/internal/model"
)

func TestSearchBody(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCategory("技术"); err != nil {
		t.Fatal(err)
	}
	n, err := s.SaveNote(model.Note{
		Category: "技术",
		Title:    "测试笔记",
		Body:     "# 标题\n\n正文 **加粗**。\n\n- 项目一\n",
		Tags:     []string{"go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("保存后 file=%q body=%q", n.File, n.Body)

	// 读回
	got, err := s.GetNote("技术", n.File)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("读回 body=%q", got.Body)

	raw, _ := os.ReadFile(filepath.Join(dir, "技术", n.File))
	t.Logf("磁盘内容:\n%s", string(raw))

	all, err := s.ListNotes("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ListNotes 返回 %d 条", len(all))
	for _, x := range all {
		t.Logf("  - %s body=%q", x.Title, x.Body)
	}

	hits, err := s.Search("加粗", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("搜索'加粗'命中 %d 条", len(hits))
	if len(hits) == 0 {
		t.Errorf("搜索未命中正文，这是 bug")
	}
}
