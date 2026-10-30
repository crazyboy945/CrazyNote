package model

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Note 是一篇笔记。磁盘上对应一个 .md 文件，YAML frontmatter 保存元数据，正文为 Markdown。
type Note struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Category string    `json:"category"`
	Tags     []string  `json:"tags"`
	Source   string    `json:"source"`
	Body     string    `json:"body"`
	File     string    `json:"file"` // 相对分类目录的文件名
}

// Category 是一个分类，对应磁盘上的一个子目录。
type Category struct {
	Name    string    `json:"name"`
	Count   int       `json:"count"`
	Updated time.Time `json:"updated"`
}

// frontmatter 是写入 .md 文件头部的 YAML 块。字段顺序即输出顺序。
type frontmatter struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Created  string   `yaml:"created"`
	Updated  string   `yaml:"updated"`
	Category string   `yaml:"category"`
	Tags     []string `yaml:"tags,omitempty"`
	Source   string   `yaml:"source,omitempty"`
}

const timeLayout = time.RFC3339

// SplitFrontmatter 拆出 YAML frontmatter 与正文。文件没有 frontmatter 时
// 返回空的 frontmatter 与完整正文，交由调用方按普通文本处理。
func SplitFrontmatter(raw string) (frontmatter, string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return frontmatter{}, raw
	}
	rest := normalized[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return frontmatter{}, raw
	}
	yamlBlock := rest[:end]
	body := rest[end+4:]
	body = strings.TrimPrefix(body, "\n")
	// frontmatter 结束行后若还有 "---" 残留则去掉
	body = strings.TrimPrefix(body, "---\n")
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return frontmatter{}, raw
	}
	return fm, body
}

// Render 把笔记序列化为磁盘上的 Markdown 文本。
func (n Note) Render() (string, error) {
	fm := frontmatter{
		ID:       n.ID,
		Title:    n.Title,
		Created:  n.Created.Format(timeLayout),
		Updated:  n.Updated.Format(timeLayout),
		Category: n.Category,
		Tags:     n.Tags,
		Source:   n.Source,
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(&fm); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	sb.WriteString("---\n\n")
	body := strings.TrimLeft(n.Body, "\n")
	sb.WriteString(body)
	if !strings.HasSuffix(body, "\n") && body != "" {
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// ParseNote 从 Markdown 原文解析出笔记元数据。
func ParseNote(raw, category, file string) Note {
	fm, body := SplitFrontmatter(raw)
	n := Note{
		ID:       fm.ID,
		Title:    fm.Title,
		Category: category,
		Tags:     fm.Tags,
		Source:   fm.Source,
		Body:     strings.TrimRight(body, "\n"),
		File:     file,
	}
	if n.Title == "" {
		n.Title = TitleFromFilename(file)
	}
	if n.Category == "" {
		n.Category = category
	}
	if t, err := time.Parse(timeLayout, fm.Created); err == nil {
		n.Created = t
	}
	if t, err := time.Parse(timeLayout, fm.Updated); err == nil {
		n.Updated = t
	}
	if n.Created.IsZero() {
		n.Created = n.Updated
	}
	if n.Updated.IsZero() {
		n.Updated = n.Created
	}
	return n
}

// TitleFromFilename 从文件名回退生成标题。
func TitleFromFilename(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if base == "" {
		return "未命名笔记"
	}
	return base
}

var unsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SafeFileName 把任意标题转换为安全的文件名（保留中文，去掉路径分隔符与控制字符）。
func SafeFileName(title string) string {
	name := strings.TrimSpace(title)
	name = unsafeName.ReplaceAllString(name, "-")
	name = strings.Trim(name, ". ")
	name = strings.ReplaceAll(name, "  ", " ")
	if name == "" {
		name = "未命名笔记"
	}
	if reservedNames[strings.ToUpper(name)] {
		name = "_" + name
	}
	// 限制长度，避免超出文件系统限制
	runes := []rune(name)
	if len(runes) > 80 {
		name = strings.TrimSpace(string(runes[:80]))
	}
	return name
}

// ValidCategoryName 校验分类名是否可用。
func ValidCategoryName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("分类名无效")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("分类名不能包含路径分隔符")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("分类名不能以点开头")
	}
	if len([]rune(name)) > 60 {
		return fmt.Errorf("分类名过长")
	}
	if unsafeName.MatchString(name) {
		return fmt.Errorf("分类名包含非法字符")
	}
	return nil
}

// ValidFileName 校验笔记文件名，防止路径穿越。
func ValidFileName(name string) error {
	if name == "" {
		return fmt.Errorf("文件名不能为空")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("文件名无效")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("文件名无效")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		return fmt.Errorf("只支持 .md 文件")
	}
	return nil
}

// EnsureDir 确保目录存在。
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
