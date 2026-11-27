package approle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type stubFetcher struct {
	data []byte
	mime string
	err  error
}

func (s stubFetcher) Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	return s.data, s.mime, s.err
}

func pngBody(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码 PNG 失败：%v", err)
	}
	return buf.Bytes()
}

// doRaw 发送原始字节请求体，独立于 JSON 辅助函数。
func doRaw(t *testing.T, ts *httptest.Server, method, path string, body []byte, cookie, contentType string) response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败：%v", err)
	}
	req.Header.Set("Origin", ts.URL)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cookie != "" {
		req.Header.Set("Cookie", sessionCookie+"="+cookie)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	out := response{code: resp.StatusCode, body: data, header: resp.Header}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			out.cookie = c.Value
		}
	}
	return out
}

func dirEntries(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取图标目录失败：%v", err)
	}
	return entries
}

func TestIconUploadReadReplaceDelete(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	ck := login.cookie

	created := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"图标","wan_url":"https://icon.example.com"}`, ck, nil)
	if created.code != http.StatusCreated {
		t.Fatalf("创建书签失败：%d %s", created.code, created.body)
	}
	id := int64(created.json(t)["id"].(float64))

	png1 := pngBody(t, color.RGBA{R: 255, A: 255})
	up := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), png1, ck, "image/png")
	if up.code != http.StatusOK {
		t.Fatalf("上传图标失败：%d %s", up.code, up.body)
	}
	if m := up.json(t); m["has_icon"] != true || m["icon_mime"] != "image/png" {
		t.Fatalf("上传响应不符：%v", m)
	}

	// 访客可读公开书签图标
	got := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", nil)
	if got.code != http.StatusOK || got.header.Get("Content-Type") != "image/png" {
		t.Fatalf("读取图标失败：%d %s body=%s", got.code, got.header.Get("Content-Type"), got.body)
	}
	if !bytes.Equal(got.body, png1) {
		t.Fatalf("图标内容与上传不一致")
	}
	etag := got.header.Get("ETag")
	if etag == "" || got.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("缺少安全响应头：%v", got.header)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", map[string]string{"If-None-Match": etag}); r.code != http.StatusNotModified {
		t.Fatalf("ETag 未生效：%d", r.code)
	}

	// 替换为不同内容，旧文件应被清理
	png2 := pngBody(t, color.RGBA{B: 255, A: 255})
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), png2, ck, "image/png"); r.code != http.StatusOK {
		t.Fatalf("替换图标失败：%d %s", r.code, r.body)
	}
	if entries := dirEntries(t, s.icons.Dir()); len(entries) != 1 {
		t.Fatalf("替换后应只剩 1 个文件，实际 %d", len(entries))
	}
	got2 := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", nil)
	if !bytes.Equal(got2.body, png2) {
		t.Fatalf("替换后内容未更新")
	}

	// 删除图标
	if r := do(t, ts, "DELETE", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), "", ck, nil); r.code != http.StatusOK {
		t.Fatalf("删除图标失败：%d %s", r.code, r.body)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", nil); r.code != http.StatusNotFound {
		t.Fatalf("删除后图标应 404，got %d", r.code)
	}
	if entries := dirEntries(t, s.icons.Dir()); len(entries) != 0 {
		t.Fatalf("删除后文件应清空，剩余 %d", len(entries))
	}
	bm := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d", id), "", ck, nil).json(t)
	if bm["has_icon"] != false || bm["icon_type"] != "none" {
		t.Fatalf("删除后书签图标状态不符：%v", bm)
	}
}

func TestIconUploadValidationAndLimits(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	ck := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil).cookie
	created := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"x","wan_url":"https://x.example.com"}`, ck, nil)
	id := int64(created.json(t)["id"].(float64))

	// 非图片 Content-Type
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), []byte("hello"), ck, "text/plain"); r.code != http.StatusBadRequest {
		t.Fatalf("非图片类型应 400，got %d", r.code)
	}
	// 伪装成图片的 HTML
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), []byte("<html>x</html>"), ck, "image/png"); r.code != http.StatusBadRequest {
		t.Fatalf("伪造图片应 400，got %d body=%s", r.code, r.body)
	}
	// 超过 256KiB
	big := bytes.Repeat([]byte{0xff}, 300*1024)
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), big, ck, "image/png"); r.code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超大图标应 413，got %d body=%s", r.code, r.body)
	}
	// 不存在书签
	if r := doRaw(t, ts, "PUT", "/api/admin/bookmarks/9999/icon", pngBody(t, color.RGBA{A: 255}), ck, "image/png"); r.code != http.StatusNotFound {
		t.Fatalf("不存在书签应 404，got %d", r.code)
	}
	if entries := dirEntries(t, s.icons.Dir()); len(entries) != 0 {
		t.Fatalf("失败上传不应留下文件：%d", len(entries))
	}
}

func TestIconPermissionBoundaries(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	ck := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil).cookie
	priv := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"私","wan_url":"https://p.example.com","visibility":"private"}`, ck, nil)
	privID := int64(priv.json(t)["id"].(float64))
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", privID), pngBody(t, color.RGBA{G: 255, A: 255}), ck, "image/png"); r.code != http.StatusOK {
		t.Fatalf("管理员上传失败：%d", r.code)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", privID), "", "", nil); r.code != http.StatusNotFound {
		t.Fatalf("访客不应读到私有图标，got %d", r.code)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", privID), "", ck, nil); r.code != http.StatusOK {
		t.Fatalf("管理员应可读私有图标，got %d", r.code)
	}
	for _, tc := range []struct{ method, path string }{
		{"PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", privID)},
		{"DELETE", fmt.Sprintf("/api/admin/bookmarks/%d/icon", privID)},
		{"POST", fmt.Sprintf("/api/admin/bookmarks/%d/favicon", privID)},
	} {
		if r := doRaw(t, ts, tc.method, tc.path, pngBody(t, color.RGBA{A: 255}), "", "image/png"); r.code != http.StatusUnauthorized {
			t.Fatalf("访客 %s %s 应 401，got %d", tc.method, tc.path, r.code)
		}
	}
	// 退出后同样拒绝
	_ = do(t, ts, "POST", "/api/auth/logout", "", ck, nil)
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", privID), "", ck, nil); r.code != http.StatusNotFound {
		t.Fatalf("退出后应按访客处理，got %d", r.code)
	}
}

func TestIconDeletedWithBookmarkAndMissingFileTolerated(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	ck := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil).cookie

	a := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"A","wan_url":"https://a.example.com"}`, ck, nil)
	b := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"B","wan_url":"https://b.example.com"}`, ck, nil)
	idA := int64(a.json(t)["id"].(float64))
	idB := int64(b.json(t)["id"].(float64))
	for _, id := range []int64{idA, idB} {
		if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), pngBody(t, color.RGBA{R: 10, A: 255}), ck, "image/png"); r.code != http.StatusOK {
			t.Fatalf("上传失败：%d", r.code)
		}
	}

	// B 的图标文件被外部删除：只影响 B，不影响列表与 A
	bm := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d", idB), "", ck, nil).json(t)
	if bm["has_icon"] != true {
		t.Fatalf("B 应有图标")
	}
	for _, e := range dirEntries(t, s.icons.Dir()) {
		if err := os.Remove(s.icons.Dir() + "/" + e.Name()); err != nil {
			t.Fatalf("删除测试文件失败：%v", err)
		}
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", idB), "", "", nil); r.code != http.StatusNotFound {
		t.Fatalf("文件缺失应 404，got %d", r.code)
	}
	if r := do(t, ts, "GET", "/api/bookmarks", "", "", nil); r.code != 200 {
		t.Fatalf("列表不应受图标缺失影响：%d", r.code)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", idA), "", "", nil); r.code != http.StatusNotFound {
		t.Fatalf("文件被外部删除后 A 也 404（预期一致降级），got %d", r.code)
	}

	// 重新上传后删除书签，图标文件应被清理
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", idA), pngBody(t, color.RGBA{R: 20, A: 255}), ck, "image/png"); r.code != http.StatusOK {
		t.Fatalf("重新上传失败：%d", r.code)
	}
	if r := do(t, ts, "DELETE", fmt.Sprintf("/api/admin/bookmarks/%d", idA), "", ck, nil); r.code != 200 {
		t.Fatalf("删除书签失败：%d", r.code)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", idA), "", "", nil); r.code != http.StatusNotFound {
		t.Fatalf("书签删除后图标应 404，got %d", r.code)
	}
	if entries := dirEntries(t, s.icons.Dir()); len(entries) != 0 {
		t.Fatalf("书签删除后图标文件应清理，剩余 %d", len(entries))
	}
}

func TestIconRemovedWhenIconTypeChanges(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	ck := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil).cookie
	c := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"切换","wan_url":"https://s.example.com"}`, ck, nil)
	id := int64(c.json(t)["id"].(float64))
	if r := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), pngBody(t, color.RGBA{G: 128, A: 255}), ck, "image/png"); r.code != http.StatusOK {
		t.Fatalf("上传失败：%d", r.code)
	}
	if r := do(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d", id), `{"icon_type":"emoji","icon_value":"🔖"}`, ck, nil); r.code != 200 {
		t.Fatalf("切换图标类型失败：%d %s", r.code, r.body)
	}
	bm := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d", id), "", ck, nil).json(t)
	if bm["has_icon"] != false || bm["icon_type"] != "emoji" {
		t.Fatalf("切换后应清理本地图标：%v", bm)
	}
	if entries := dirEntries(t, s.icons.Dir()); len(entries) != 0 {
		t.Fatalf("切换后文件应清理，剩余 %d", len(entries))
	}
	// 显式写入 icon_type=file 必须被拒绝
	if r := do(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d", id), `{"icon_type":"file"}`, ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("icon_type=file 应 400，got %d", r.code)
	}
}

func TestFaviconEndpointSuccessFailureAndAsync(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	ck := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil).cookie
	stub := stubFetcher{data: pngBody(t, color.RGBA{B: 200, A: 255}), mime: "image/png"}
	s.favicon = stub

	c := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"抓取","wan_url":"https://fav.example.com"}`, ck, nil)
	id := int64(c.json(t)["id"].(float64))
	fr := do(t, ts, "POST", fmt.Sprintf("/api/admin/bookmarks/%d/favicon", id), "", ck, nil)
	if fr.code != http.StatusOK || fr.json(t)["has_icon"] != true {
		t.Fatalf("手动抓取失败：%d %s", fr.code, fr.body)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", nil); r.code != 200 {
		t.Fatalf("抓取后图标不可读：%d", r.code)
	}

	// 抓取失败：502，且保留已有图标
	s.favicon = stubFetcher{err: errors.New("目标不允许访问")}
	before := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", nil).body
	if r := do(t, ts, "POST", fmt.Sprintf("/api/admin/bookmarks/%d/favicon", id), "", ck, nil); r.code != http.StatusBadGateway {
		t.Fatalf("抓取失败应 502，got %d %s", r.code, r.body)
	}
	after := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", id), "", "", nil).body
	if !bytes.Equal(before, after) {
		t.Fatalf("抓取失败不应破坏已有图标")
	}

	// 异步：创建 icon_type=favicon 的书签，后台抓取最终落库
	s.favicon = stub
	s.autoFavicon = true
	created := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"自动","wan_url":"https://auto.example.com","icon_type":"favicon"}`, ck, nil)
	if created.code != http.StatusCreated {
		t.Fatalf("创建失败：%d", created.code)
	}
	aid := int64(created.json(t)["id"].(float64))
	if created.json(t)["has_icon"] != false {
		t.Fatalf("创建响应应立即可返回（此时尚无图标）")
	}
	okIcon := false
	for i := 0; i < 40; i++ {
		time.Sleep(20 * time.Millisecond)
		if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", aid), "", "", nil); r.code == 200 {
			okIcon = true
			break
		}
	}
	if !okIcon {
		t.Fatalf("后台抓取未在预期时间内完成")
	}
}
