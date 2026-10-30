package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/local/crazynote/internal/model"
)

// Store 负责在磁盘上组织笔记。
//
// 目录结构：
//
//	<root>/
//	  .crazynote/meta.json   分类排序等应用元数据
//	  .crazynote/trash/      删除的分类（可恢复）
//	  <分类名>/<笔记>.md
//
// 每个分类是一个独立子目录，笔记是纯 Markdown 文件，任何编辑器都能直接打开。
type Store struct {
	root string
	mu   sync.RWMutex
}

const (
	metaDirName  = ".crazynote"
	metaFileName = "meta.json"
	trashDirName = "trash"
)

// Meta 是保存在 .crazynote/meta.json 中的应用级元数据。
type Meta struct {
	CategoryOrder []string `json:"category_order"`
	Title         string   `json:"title"`
	// MarkdownTheme 记录阅读视图使用的样式主题（default/reading/paper/card/compact）。
	MarkdownTheme string `json:"markdown_theme"`
}

// New 创建存储并确保根目录存在。
func New(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("创建笔记目录失败: %w", err)
	}
	s := &Store{root: abs}
	// 只创建 .crazynote 目录本身：meta.json 是文件，不能拿它当目录建。
	// （早期版本误用 MkdirAll(metaPath())，会把 meta.json 建成目录；
	//  之后保存元数据时又被改写成文件，于是每次启动都报 "not a directory" 而起不来。）
	if err := os.MkdirAll(s.metaDir(), 0o755); err != nil {
		return nil, fmt.Errorf("创建元数据目录失败: %w", err)
	}
	// 自愈：清理早期版本误建成的 meta.json 目录，让应用能正常启动
	if fi, err := os.Stat(s.metaPath()); err == nil && fi.IsDir() {
		if err := os.RemoveAll(s.metaPath()); err != nil {
			return nil, fmt.Errorf("清理异常的 meta.json 目录失败: %w", err)
		}
	}
	return s, nil
}

// Root 返回笔记根目录绝对路径。
func (s *Store) Root() string { return s.root }

func (s *Store) metaDir() string { return filepath.Join(s.root, metaDirName) }

func (s *Store) metaPath() string { return filepath.Join(s.metaDir(), metaFileName) }

func (s *Store) trashPath() string { return filepath.Join(s.root, metaDirName, trashDirName) }

// LoadMeta 读取应用元数据。
func (s *Store) LoadMeta() Meta {
	m := Meta{Title: "CrazyNote", MarkdownTheme: "default"}
	b, err := os.ReadFile(s.metaPath())
	if err == nil {
		_ = decodeJSON(b, &m)
	}
	return m
}

// SaveMeta 保存应用元数据。
func (s *Store) SaveMeta(m Meta) error {
	b, err := encodeJSON(m)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.metaPath(), b, 0o644)
}

// ---------- 图片存储 ----------

// 图片统一存放在隐藏的应用元数据目录内，避免被当作 .md 笔记扫描，
// 也不会因分类重命名/移动而失效。
const imageDirName = "images"

var unsafeImageName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func (s *Store) imageBaseDir() string {
	return filepath.Join(s.root, metaDirName, "assets", imageDirName)
}

// ImagePath 返回图片在磁盘上的绝对路径，供静态服务读取。
func (s *Store) ImagePath(name string) string {
	return filepath.Join(s.imageBaseDir(), name)
}

// SaveImage 保存上传的图片，返回可供前端引用的相对 URL（不含 BASE 前缀）。
// filename 仅取基础名并做安全化，杜绝路径穿越。
func (s *Store) SaveImage(filename string, data []byte) (string, error) {
	name := sanitizeImageName(filename)
	if name == "" {
		return "", fmt.Errorf("文件名无效")
	}
	dir := s.imageBaseDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	final := s.uniqueImageName(dir, name)
	if err := writeFileAtomic(filepath.Join(dir, final), data, 0o644); err != nil {
		return "", err
	}
	return "/assets/images/" + final, nil
}

// uniqueImageName 避免同名覆盖。
func (s *Store) uniqueImageName(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
}

// sanitizeImageName 从任意文件名中提取并净化基础名，仅保留常见图片扩展名。
func sanitizeImageName(filename string) string {
	name := filepath.Base(strings.TrimSpace(filename))
	name = unsafeImageName.ReplaceAllString(name, "-")
	name = strings.Trim(name, ". ")
	ext := strings.ToLower(filepath.Ext(name))
	allowed := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".webp": true, ".bmp": true, ".svg": true,
	}
	if !allowed[ext] {
		// 未知扩展名默认补 .png，避免暴露原始扩展名带来的风险
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ".png"
	}
	runes := []rune(name)
	if len(runes) > 120 {
		name = strings.TrimSpace(string(runes[:120]))
	}
	if name == "" {
		return ""
	}
	return name
}

// CategoryDir 返回分类对应的目录绝对路径。
func (s *Store) CategoryDir(name string) (string, error) {
	if err := model.ValidCategoryName(name); err != nil {
		return "", err
	}
	return filepath.Join(s.root, name), nil
}

// ListCategories 返回全部分类，按元数据顺序排列，未记录的排在后面按字母序。
func (s *Store) ListCategories() ([]model.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listCategories()
}

func (s *Store) listCategories() ([]model.Category, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	updated := map[string]time.Time{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		notes, err := s.listNotesIn(e.Name())
		if err != nil {
			continue
		}
		counts[e.Name()] = len(notes)
		for _, n := range notes {
			if n.Updated.After(updated[e.Name()]) {
				updated[e.Name()] = n.Updated
			}
		}
	}
	meta := s.LoadMeta()
	order := map[string]int{}
	for i, name := range meta.CategoryOrder {
		order[name] = i
	}
	cats := make([]model.Category, 0, len(counts))
	for name, c := range counts {
		cats = append(cats, model.Category{Name: name, Count: c, Updated: updated[name]})
	}
	sort.Slice(cats, func(i, j int) bool {
		oi, iok := order[cats[i].Name]
		oj, jok := order[cats[j].Name]
		switch {
		case iok && jok:
			return oi < oj
		case iok:
			return true
		case jok:
			return false
		default:
			return cats[i].Name < cats[j].Name
		}
	})
	return cats, nil
}

// CreateCategory 新建分类目录。
func (s *Store) CreateCategory(name string) error {
	if err := model.ValidCategoryName(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.CategoryDir(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("分类已存在")
	}
	return os.MkdirAll(dir, 0o755)
}

// RenameCategory 重命名分类（连同目录内全部笔记）。
func (s *Store) RenameCategory(oldName, newName string) error {
	if err := model.ValidCategoryName(newName); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldDir, err := s.CategoryDir(oldName)
	if err != nil {
		return err
	}
	newDir, err := s.CategoryDir(newName)
	if err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	if _, err := os.Stat(oldDir); err != nil {
		return fmt.Errorf("分类不存在")
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("目标分类已存在")
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}
	// 同步更新笔记 frontmatter 里的分类名
	notes, _ := s.listNotesIn(newName)
	for _, n := range notes {
		n.Category = newName
		s.writeNote(newDir, n)
	}
	// 更新排序元数据
	meta := s.LoadMeta()
	newOrder := make([]string, 0, len(meta.CategoryOrder))
	for _, c := range meta.CategoryOrder {
		if c == oldName {
			newOrder = append(newOrder, newName)
		} else {
			newOrder = append(newOrder, c)
		}
	}
	meta.CategoryOrder = newOrder
	return s.SaveMeta(meta)
}

// DeleteCategory 删除分类，移入回收站目录以便误删后恢复。
func (s *Store) DeleteCategory(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.CategoryDir(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("分类不存在")
	}
	trash := filepath.Join(s.trashPath(), fmt.Sprintf("%s-%d", name, time.Now().UnixNano()))
	if err := os.MkdirAll(s.trashPath(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(dir, trash); err != nil {
		return err
	}
	meta := s.LoadMeta()
	order := make([]string, 0, len(meta.CategoryOrder))
	for _, c := range meta.CategoryOrder {
		if c != name {
			order = append(order, c)
		}
	}
	meta.CategoryOrder = order
	return s.SaveMeta(meta)
}

// ListNotes 返回某个分类下的笔记摘要（不含正文）。category 为空时返回全部。
func (s *Store) ListNotes(category string) ([]model.Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if category != "" {
		if err := model.ValidCategoryName(category); err != nil {
			return nil, err
		}
		return s.listNotesIn(category)
	}
	cats, err := s.listCategories()
	if err != nil {
		return nil, err
	}
	var all []model.Note
	for _, c := range cats {
		notes, err := s.listNotesIn(c.Name)
		if err != nil {
			continue
		}
		all = append(all, notes...)
	}
	sortNotes(all)
	return all, nil
}

func (s *Store) listNotesIn(category string) ([]model.Note, error) {
	dir, err := s.CategoryDir(category)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var notes []model.Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		n := model.ParseNote(string(raw), category, e.Name())
		notes = append(notes, n)
	}
	sortNotes(notes)
	return notes, nil
}

func sortNotes(notes []model.Note) {
	sort.SliceStable(notes, func(i, j int) bool {
		return notes[i].Updated.After(notes[j].Updated)
	})
}

// GetNote 读取单篇笔记的完整内容。
func (s *Store) GetNote(category, file string) (model.Note, error) {
	if err := model.ValidCategoryName(category); err != nil {
		return model.Note{}, err
	}
	if err := model.ValidFileName(file); err != nil {
		return model.Note{}, err
	}
	dir, err := s.CategoryDir(category)
	if err != nil {
		return model.Note{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		if os.IsNotExist(err) {
			return model.Note{}, errors.New("笔记不存在")
		}
		return model.Note{}, err
	}
	return model.ParseNote(string(raw), category, file), nil
}

// SaveNote 保存笔记。已有笔记会按标题变化重命名文件，新建笔记自动生成文件名。
func (s *Store) SaveNote(n model.Note) (model.Note, error) {
	if err := model.ValidCategoryName(n.Category); err != nil {
		return model.Note{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.CategoryDir(n.Category)
	if err != nil {
		return model.Note{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.Note{}, err
	}

	now := time.Now()
	if n.Created.IsZero() {
		n.Created = now
	}
	n.Updated = now
	if n.ID == "" {
		n.ID = newID()
	}
	if strings.TrimSpace(n.Title) == "" {
		n.Title = "未命名笔记"
	}

	oldFile := n.File
	if oldFile != "" {
		if err := model.ValidFileName(oldFile); err != nil {
			return model.Note{}, err
		}
	}
	// 标题变化时同步重命名文件，便于在文件管理器中直接识别
	newFile := model.SafeFileName(n.Title) + ".md"
	if oldFile != "" && oldFile != newFile {
		if _, err := os.Stat(filepath.Join(dir, oldFile)); err == nil {
			newFile = s.uniqueFileName(dir, newFile)
			if err := os.Rename(filepath.Join(dir, oldFile), filepath.Join(dir, newFile)); err != nil {
				return model.Note{}, err
			}
		}
	} else if oldFile == "" {
		newFile = s.uniqueFileName(dir, newFile)
	}
	n.File = newFile

	if err := s.writeNote(dir, n); err != nil {
		return model.Note{}, err
	}
	return n, nil
}

func (s *Store) writeNote(dir string, n model.Note) error {
	content, err := n.Render()
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, n.File), []byte(content), 0o644)
}

// uniqueFileName 为同名文件追加序号，避免互相覆盖。
func (s *Store) uniqueFileName(dir, name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	candidate := name
	for i := 2; i < 1000; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
}

// DeleteNote 删除笔记文件。
func (s *Store) DeleteNote(category, file string) error {
	if err := model.ValidCategoryName(category); err != nil {
		return err
	}
	if err := model.ValidFileName(file); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.CategoryDir(category)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, file)); err != nil {
		if os.IsNotExist(err) {
			return errors.New("笔记不存在")
		}
		return err
	}
	return nil
}

// MoveNote 把笔记移动到另一个分类。
func (s *Store) MoveNote(category, file, targetCategory string) (model.Note, error) {
	if err := model.ValidCategoryName(targetCategory); err != nil {
		return model.Note{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.GetNote(category, file)
	if err != nil {
		return model.Note{}, err
	}
	targetDir, err := s.CategoryDir(targetCategory)
	if err != nil {
		return model.Note{}, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return model.Note{}, err
	}
	n.Category = targetCategory
	n.File = s.uniqueFileName(targetDir, model.SafeFileName(n.Title)+".md")
	if err := s.writeNote(targetDir, n); err != nil {
		return model.Note{}, err
	}
	if err := os.Remove(filepath.Join(s.root, category, file)); err != nil {
		return model.Note{}, err
	}
	return n, nil
}

// Search 在标题、标签与正文中做不区分大小写的子串匹配。
func (s *Store) Search(query, category string) ([]model.Note, error) {
	notes, err := s.ListNotes(category)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return notes, nil
	}
	var hits []model.Note
	for _, n := range notes {
		if strings.Contains(strings.ToLower(n.Title), q) ||
			strings.Contains(strings.ToLower(n.Body), q) ||
			strings.Contains(strings.ToLower(n.Source), q) ||
			containsTag(n.Tags, q) {
			hits = append(hits, n)
		}
	}
	return hits, nil
}

func containsTag(tags []string, q string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// AllTags 返回所有出现过的标签及出现次数。
func (s *Store) AllTags() (map[string]int, error) {
	notes, err := s.ListNotes("")
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, n := range notes {
		for _, t := range n.Tags {
			out[t]++
		}
	}
	return out, nil
}
