package datadir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	root := "/data"
	if err := Within("/data", root); err != nil {
		t.Errorf("根自身应视为在内：%v", err)
	}
	if err := Within("/data/icons", root); err != nil {
		t.Errorf("子目录应视为在内：%v", err)
	}
	if err := Within("/etc", root); err == nil {
		t.Error("越界路径应被拒绝")
	}
	if err := Within("/data/../etc", root); err == nil {
		t.Error("包含 .. 的越界路径应被拒绝")
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	// /<root>/escape -> 外部目录
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("无法创建符号链接：%v", err)
	}
	if _, err := Resolve(filepath.Join(root, "escape"), root); err == nil {
		t.Fatal("符号链接逃逸必须被拒绝")
	}
	if _, err := Resolve(filepath.Join(root, "icons"), root); err != nil {
		t.Errorf("正常子目录应通过：%v", err)
	}
}

func TestCheckWrite(t *testing.T) {
	root := t.TempDir()
	if _, err := CheckWrite(root); err != nil {
		t.Fatalf("临时目录应可写：%v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("写自检后不应残留文件，实际 %d 个", len(entries))
	}
}

func TestCheckWriteUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 下无法构造不可写目录（root 会绕过权限）")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o700)
	if _, err := CheckWrite(root); err == nil {
		t.Error("不可写目录必须报错")
	}
}

func TestEnsureDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	if err := EnsureDirs(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "icons", "backups", "logs"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("目录 %q 未创建：%v", name, err)
		}
	}
}
