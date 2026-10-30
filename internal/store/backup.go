package store

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---------- 备份与恢复 ----------

// 备份相关的安全上限，防止损坏或恶意的压缩包写满磁盘。
const (
	// maxBackupEntries 压缩包内条目数上限。
	maxBackupEntries = 200000
	// maxBackupUnpack 解压后总体积上限（2GB）。
	maxBackupUnpack = 2 << 30
	// maxBackupFileSize 单个文件解压后体积上限（512MB）。
	maxBackupFileSize = 512 << 20
)

// restoreBackupDir 历史恢复快照的存放目录（相对 .crazynote）。
const restoreBackupDir = "restore-backups"

// BackupStats 描述一次备份或恢复的结果。
type BackupStats struct {
	// Files 参与的文件数。
	Files int `json:"files"`
	// Bytes 文件总字节数。
	Bytes int64 `json:"bytes"`
	// Notes 笔记（.md）数量。
	Notes int `json:"notes"`
	// Categories 分类数量。
	Categories int `json:"categories"`
	// Images 图片数量。
	Images int `json:"images"`
	// Snapshot 恢复前自动快照的目录（绝对路径），仅恢复时有值。
	Snapshot string `json:"snapshot,omitempty"`
}

// ExportBackup 把整个笔记目录打包成 ZIP 写入 w。
//
// 内容包含：所有分类的笔记、应用元数据（分类排序、样式偏好）与上传的图片。
// 历史恢复快照目录会被排除，避免备份文件无限膨胀。
func (s *Store) ExportBackup(w io.Writer) (BackupStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var st BackupStats
	zw := zip.NewWriter(w)
	root := s.root

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			// 排除历史恢复快照（位于 .crazynote/restore-backups/）
			skipPrefix := metaDirName + "/" + restoreBackupDir
			if rel == skipPrefix || strings.HasPrefix(rel, skipPrefix+"/") {
				return fs.SkipDir
			}
			return nil
		}
		// 只收常规文件，跳过符号链接等特殊文件
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		n, err := io.Copy(fw, src)
		src.Close()
		if err != nil {
			return err
		}
		st.countFile(rel, n)
		return nil
	})
	if err != nil {
		zw.Close()
		return st, err
	}
	if err := zw.Close(); err != nil {
		return st, err
	}
	st.Categories = s.countCategories()
	return st, nil
}

// RestoreBackup 从 ZIP 恢复笔记到笔记根目录。
//
// 安全措施：
//  1. 先完整校验压缩包（路径穿越、条目数、解压体积），不合格直接拒绝，**不动现有数据**；
//  2. 恢复前把现有内容整体移动到 .crazynote/restore-backups/<时间戳>/，可人工回退；
//  3. 解压时逐条再次校验目标路径，杜绝"校验与解压之间"的差异。
func (s *Store) RestoreBackup(zipPath string) (BackupStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return BackupStats{}, fmt.Errorf("无法打开备份文件: %w", err)
	}
	defer zr.Close()

	// ---------- 阶段一：校验（不改动现有数据）----------
	if len(zr.File) > maxBackupEntries {
		return BackupStats{}, fmt.Errorf("备份包条目过多（超过 %d 项）", maxBackupEntries)
	}
	var total int64
	for _, f := range zr.File {
		if err := s.validateBackupPath(f.Name); err != nil {
			return BackupStats{}, err
		}
		if f.UncompressedSize64 > maxBackupFileSize {
			return BackupStats{}, fmt.Errorf("备份包内文件过大: %s", f.Name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxBackupUnpack {
			return BackupStats{}, fmt.Errorf("备份包解压后体积过大")
		}
	}

	// ---------- 阶段二：给现有内容拍快照 ----------
	snapshot, err := s.snapshotCurrent()
	if err != nil {
		return BackupStats{}, fmt.Errorf("创建恢复前快照失败: %w", err)
	}

	// ---------- 阶段三：解压 ----------
	var st BackupStats
	for _, f := range zr.File {
		// 再次校验，确保目标落在笔记根目录内
		if err := s.validateBackupPath(f.Name); err != nil {
			return st, err
		}
		target := filepath.Join(s.root, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return st, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return st, err
		}
		rc, err := f.Open()
		if err != nil {
			return st, err
		}
		// 用 LimitReader 兜底：压缩包头里声明的大小可能撒谎（zip 炸弹）
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return st, err
		}
		n, err := io.Copy(out, io.LimitReader(rc, maxBackupFileSize+1))
		out.Close()
		rc.Close()
		if err != nil {
			return st, err
		}
		if n > maxBackupFileSize {
			return st, fmt.Errorf("备份包内文件过大: %s", f.Name)
		}
		st.countFile(filepath.ToSlash(f.Name), n)
	}
	st.Categories = s.countCategories()
	st.Snapshot = snapshot
	return st, nil
}

// RestoreBackupsDir 返回历史恢复快照所在目录。
func (s *Store) RestoreBackupsDir() string {
	return filepath.Join(s.metaDir(), restoreBackupDir)
}

// validateBackupPath 校验压缩包内的路径是否安全：
// 必须是相对路径、不含 ".."、且最终目标仍在笔记根目录内。
func (s *Store) validateBackupPath(name string) error {
	if name == "" {
		return fmt.Errorf("备份包内存在空路径")
	}
	n := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(n, "/") {
		return fmt.Errorf("备份包内存在绝对路径: %s", name)
	}
	// Windows 盘符，如 C:/xxx
	if len(n) > 1 && n[1] == ':' {
		return fmt.Errorf("备份包内存在绝对路径: %s", name)
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return fmt.Errorf("备份包内存在路径穿越: %s", name)
		}
	}
	clean := filepath.Clean(filepath.Join(s.root, filepath.FromSlash(n)))
	if clean != s.root && !strings.HasPrefix(clean, s.root+string(os.PathSeparator)) {
		return fmt.Errorf("备份包内路径越界: %s", name)
	}
	return nil
}

// snapshotCurrent 把现有笔记整体移动到 .crazynote/restore-backups/<时间戳>/。
//
// 恢复出问题时，可以把里面的内容手工移回原位。
func (s *Store) snapshotCurrent() (string, error) {
	base := filepath.Join(s.metaDir(), restoreBackupDir)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102-150405")
	dst := filepath.Join(base, ts)
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = filepath.Join(base, fmt.Sprintf("%s-%d", ts, i))
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}

	// 根目录下的内容（.crazynote 本身单独处理）
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Name() == metaDirName {
			continue
		}
		if err := movePath(filepath.Join(s.root, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return "", err
		}
	}

	// .crazynote 里的内容（restore-backups 自身保留）
	metaEntries, err := os.ReadDir(s.metaDir())
	if err != nil {
		return "", err
	}
	dstMeta := filepath.Join(dst, metaDirName)
	if err := os.MkdirAll(dstMeta, 0o755); err != nil {
		return "", err
	}
	for _, e := range metaEntries {
		if e.Name() == restoreBackupDir {
			continue
		}
		if err := movePath(filepath.Join(s.metaDir(), e.Name()), filepath.Join(dstMeta, e.Name())); err != nil {
			return "", err
		}
	}
	return dst, nil
}

// countFile 统计单个文件对备份结果的贡献。
func (st *BackupStats) countFile(rel string, size int64) {
	st.Files++
	st.Bytes += size
	lower := strings.ToLower(rel)
	switch {
	case strings.HasSuffix(lower, ".md"):
		st.Notes++
	case strings.Contains(lower, "/assets/images/"):
		st.Images++
	}
}

// countCategories 统计根目录下的分类数量（隐藏目录不计）。
func (s *Store) countCategories() int {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n
}

// movePath 移动文件或目录；跨设备时退化为「复制 + 删除」。
func movePath(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// copyTree 递归复制文件或目录。
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
