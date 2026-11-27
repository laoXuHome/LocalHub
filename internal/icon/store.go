package icon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store 管理数据目录下 icons/ 的图标文件，文件名由内部生成，与用户输入无关。
type Store struct {
	dir string
}

// New 创建图标存储，目录固定为 <dataDir>/icons。
func New(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "icons")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建图标目录失败：%w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir 返回图标目录绝对路径（测试与运维核对用）。
func (s *Store) Dir() string { return s.dir }

var extByMime = map[string]string{
	"image/png":    ".png",
	"image/jpeg":   ".jpg",
	"image/gif":    ".gif",
	"image/x-icon": ".ico",
}

// Save 校验并写入图标，返回内部文件名与 mime。文件名为 <书签ID>-<内容哈希前缀><扩展名>，
// 不含用户提供的任何字符串；写入采用临时文件 + rename，避免半截文件被读取。
func (s *Store) Save(id int64, data []byte) (file, mime string, err error) {
	mime, ext, err := Validate(data)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	name := fmt.Sprintf("%d-%s%s", id, hex.EncodeToString(sum[:8]), ext)
	tmp, err := os.CreateTemp(s.dir, ".upload-*.tmp")
	if err != nil {
		return "", "", fmt.Errorf("创建临时图标文件失败：%w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", "", fmt.Errorf("写图标文件失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", "", fmt.Errorf("关闭图标文件失败：%w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", "", fmt.Errorf("设置图标权限失败：%w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(s.dir, name)); err != nil {
		return "", "", fmt.Errorf("保存图标文件失败：%w", err)
	}
	return name, mime, nil
}

// Path 把内部文件名解析为绝对路径，拒绝任何路径分隔符或穿越片段。
func (s *Store) Path(file string) (string, error) {
	if file == "" || strings.ContainsAny(file, `/\`) || strings.HasPrefix(file, ".") || strings.Contains(file, "..") {
		return "", fmt.Errorf("非法图标文件名")
	}
	switch filepath.Ext(file) {
	case ".png", ".jpg", ".gif", ".ico":
	default:
		return "", fmt.Errorf("非法图标扩展名")
	}
	return filepath.Join(s.dir, file), nil
}

// Read 读取图标文件；不存在或损坏时返回错误，由调用方降级为 404。
func (s *Store) Read(file string) ([]byte, error) {
	p, err := s.Path(file)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// Delete 删除指定图标文件，文件不存在视为成功。
func (s *Store) Delete(file string) error {
	p, err := s.Path(file)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除图标文件失败：%w", err)
	}
	return nil
}

// DeleteForID 清理某书签遗留的所有图标文件（含替换后残留的旧版本）。
func (s *Store) DeleteForID(id int64) error {
	matches, err := filepath.Glob(filepath.Join(s.dir, fmt.Sprintf("%d-*", id)))
	if err != nil {
		return fmt.Errorf("匹配图标文件失败：%w", err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除图标文件失败：%w", err)
		}
	}
	return nil
}
