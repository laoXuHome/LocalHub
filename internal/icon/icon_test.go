package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码 PNG 失败：%v", err)
	}
	return buf.Bytes()
}

func icoBytes() []byte {
	data := make([]byte, 6+16+8)
	binary.LittleEndian.PutUint16(data[2:4], 1)
	binary.LittleEndian.PutUint16(data[4:6], 1)
	data[6] = 32 // width
	data[7] = 32 // height
	binary.LittleEndian.PutUint32(data[6+8:6+12], 1)
	binary.LittleEndian.PutUint32(data[6+12:6+16], 6+16)
	binary.LittleEndian.PutUint32(data[6+16:6+20], 8)
	return data
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		mime string
		ok   bool
	}{
		{"png", pngBytes(t, 32, 32), "image/png", true},
		{"ico", icoBytes(), "image/x-icon", true},
		{"空文件", nil, "", false},
		{"html 伪装", []byte("<html><script>alert(1)</script></html>"), "", false},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "", false},
		{"随机字节", []byte("not an image at all"), "", false},
		{"超尺寸", pngBytes(t, 1200, 1), "", false},
		{"png 头损坏", append(append([]byte{}, magicPNG...), []byte("garbage")...), "", false},
		{"太大", bytes.Repeat([]byte{0xff}, MaxBytes+1), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mime, ext, err := Validate(c.data)
			if !c.ok {
				if err == nil {
					t.Fatalf("应被拒绝，却通过：%s %s", mime, ext)
				}
				return
			}
			if err != nil {
				t.Fatalf("应通过：%v", err)
			}
			if mime != c.mime || ext == "" {
				t.Fatalf("mime/ext 不符：%q %q", mime, ext)
			}
		})
	}
}

func TestStoreSaveReadDelete(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := pngBytes(t, 16, 16)
	file, mime, err := st.Save(7, data)
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" || !strings.HasPrefix(file, "7-") || !strings.HasSuffix(file, ".png") || strings.Contains(file, "/") {
		t.Fatalf("文件名不符合内部命名规则：%q %q", file, mime)
	}
	got, err := st.Read(file)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("读取图标失败：%v", err)
	}
	if _, err := st.Read("nope.png"); err == nil {
		t.Fatalf("不存在文件应报错")
	}
	if err := st.Delete(file); err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if _, err := st.Read(file); err == nil {
		t.Fatalf("删除后仍可读取")
	}
	if err := st.Delete(file); err != nil {
		t.Fatalf("重复删除应幂等：%v", err)
	}
}

func TestStoreRejectsTraversalAndBadName(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../x.png", "a/b.png", `a\b.png`, ".hidden.png", "/etc/passwd", "x.svg", "x.png.exe", ""} {
		if _, err := st.Path(name); err == nil {
			t.Fatalf("非法文件名应被拒绝：%q", name)
		}
	}
}

func TestDeleteForIDCleansOldVersions(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f1, _, err := st.Save(3, pngBytes(t, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	// 手工制造同 ID 旧文件，模拟替换后残留。
	old := filepath.Join(st.Dir(), "3-deadbeefdeadbeef.png")
	if err := os.WriteFile(old, pngBytes(t, 8, 8), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteForID(3); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(st.Dir())
	if len(entries) != 0 {
		t.Fatalf("应清理干净，剩余 %d 个文件（含 %s）", len(entries), f1)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := st.Save(9, pngBytes(t, 12, 12))
	if err != nil {
		t.Fatal(err)
	}
	st2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st2.Read(file)
	if err != nil || len(got) == 0 {
		t.Fatalf("重开存储后读取失败：%v", err)
	}
}
