package datadir

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Resolved 是解析并校验后的数据目录信息。
type Resolved struct {
	Path        string // dataDir 的 realpath
	AllowedRoot string // 允许根的 realpath
}

// Resolve 解析数据目录的真实路径，并确保它位于允许根之内。
// 任何符号链接逃逸、越界路径都会被拒绝。
func Resolve(dataDir, allowedRoot string) (Resolved, error) {
	cleanData := filepath.Clean(dataDir)
	cleanRoot := filepath.Clean(allowedRoot)

	root := cleanRoot
	if !filepath.IsAbs(root) {
		return Resolved{}, fmt.Errorf("允许根必须是绝对路径：%q", allowedRoot)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Resolved{}, fmt.Errorf("解析允许根 %q 失败：%w", cleanRoot, err)
	}

	if !filepath.IsAbs(cleanData) {
		return Resolved{}, fmt.Errorf("数据目录必须是绝对路径：%q", dataDir)
	}
	// dataDir 可以尚不存在（首次启动），此时解析其父目录，再拼回末级。
	realData, err := filepath.EvalSymlinks(cleanData)
	if err != nil {
		if !os.IsNotExist(err) {
			return Resolved{}, fmt.Errorf("解析数据目录 %q 失败：%w", cleanData, err)
		}
		parent, perr := filepath.EvalSymlinks(filepath.Dir(cleanData))
		if perr != nil {
			return Resolved{}, fmt.Errorf("解析数据目录父级 %q 失败：%w", filepath.Dir(cleanData), perr)
		}
		realData = filepath.Join(parent, filepath.Base(cleanData))
	}
	return Resolved{Path: realData, AllowedRoot: root}, Within(realData, root)
}

// Within 判断 path 是否位于 root 之内（root 本身视为在内）。
func Within(path, root string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("路径 %q 与允许根 %q 不可比较：%w", path, root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("路径越界：%q 不在允许根 %q 之内（拒绝符号链接或 ../ 逃逸）", path, root)
	}
	return nil
}

// EnsureDirs 建立 P0 阶段约定的容器内目录结构。
func EnsureDirs(root string) error {
	for _, name := range []string{"", "icons", "backups", "logs"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0o750); err != nil {
			return fmt.Errorf("创建目录 %q 失败：%w", p, err)
		}
	}
	return nil
}

// FixOwnership 修正属主。P0 只修正顶层与已知子目录，数据库文件按需追加；
// recursive 为 true 时才递归整棵目录（大库首次迁移时用）。
func FixOwnership(root string, uid, gid int, recursive bool) error {
	paths := []string{root}
	for _, name := range []string{"icons", "backups", "logs"} {
		paths = append(paths, filepath.Join(root, name))
	}
	for _, name := range []string{"localhub.db", "localhub.db-wal", "localhub.db-shm"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			paths = append(paths, filepath.Join(root, name))
		}
	}
	fix := func(p string) error {
		if err := os.Chown(p, uid, gid); err != nil {
			return fmt.Errorf("修正属主 %q -> %d:%d 失败：%w", p, uid, gid, err)
		}
		return nil
	}
	for _, p := range paths {
		if err := fix(p); err != nil {
			return err
		}
	}
	if !recursive {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return fix(p)
	})
}

// CheckWrite 在数据目录内创建、写入、删除一个探测文件，验证真实可写性。
func CheckWrite(root string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	probe := filepath.Join(root, ".writetest-"+hex.EncodeToString(b))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return probe, fmt.Errorf("创建探测文件失败：%w", err)
	}
	if _, err := f.Write([]byte("ok")); err != nil {
		f.Close()
		os.Remove(probe)
		return probe, fmt.Errorf("写入探测文件失败：%w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(probe)
		return probe, fmt.Errorf("sync 探测文件失败：%w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(probe)
		return probe, fmt.Errorf("关闭探测文件失败：%w", err)
	}
	if err := os.Remove(probe); err != nil {
		return probe, fmt.Errorf("删除探测文件失败：%w", err)
	}
	return probe, nil
}
