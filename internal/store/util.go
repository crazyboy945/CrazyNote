package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// encodeJSON 序列化。
func encodeJSON(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

// decodeJSON 反序列化，忽略失败以保证元数据损坏时应用仍可启动。
func decodeJSON(b []byte, v any) error { return json.Unmarshal(b, v) }

// newID 生成随机笔记 ID。
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte("fallback"))
	}
	return hex.EncodeToString(b)
}

// writeFileAtomic 先写临时文件再原子改名，避免写入中断产生半截文件。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	// Windows 上 rename 不能覆盖已存在的目标，需要先删除再改名
	if err := os.Rename(tmpName, path); err != nil {
		if rmErr := os.Remove(path); rmErr != nil {
			return err
		}
		if err2 := os.Rename(tmpName, path); err2 != nil {
			return err2
		}
	}
	return nil
}
