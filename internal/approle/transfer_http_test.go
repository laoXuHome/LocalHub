package approle

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
	"github.com/laoXuHome/LocalHub/internal/transfer"
)

func importJSONBody(t *testing.T, format string, data []byte, extra map[string]any) string {
	t.Helper()
	m := map[string]any{"format": format, "data_base64": base64.StdEncoding.EncodeToString(data)}
	for k, v := range extra {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("构造导入请求失败：%v", err)
	}
	return string(b)
}

func adminCookie(t *testing.T, ts *httptest.Server, s *server) string {
	t.Helper()
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != http.StatusOK || login.cookie == "" {
		t.Fatalf("管理员登录失败：%d %s", login.code, login.body)
	}
	return login.cookie
}

func listTotal(t *testing.T, ts *httptest.Server, ck string) int {
	t.Helper()
	r := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil)
	if r.code != http.StatusOK {
		t.Fatalf("读取书签列表失败：%d %s", r.code, r.body)
	}
	return int(r.json(t)["total"].(float64))
}

func bookmarkID(t *testing.T, r response) int64 {
	t.Helper()
	return int64(r.json(t)["id"].(float64))
}

const transferHTML = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
<DT><H3>工作</H3>
<DL><p>
<DT><H3>项目</H3>
<DL><p>
<DT><A HREF="https://first.example.com/" ADD_DATE="1700000001">First</A>
</DL><p>
<DT><A HREF="https://second.example.com/path" ADD_DATE="1700000002">Second</A>
</DL><p>
</DL><p>`

func TestImportPreviewThenExecuteWithDuplicates(t *testing.T) {
	s, ts := newTestServer(t, nil)
	ck := adminCookie(t, ts, s)

	old := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"Old Second","wan_url":"https://second.example.com/path","visibility":"private"}`, ck, nil)
	if old.code != http.StatusCreated {
		t.Fatalf("预置书签失败：%d %s", old.code, old.body)
	}
	oldID := bookmarkID(t, old)

	// 预览：不写库
	pv := do(t, ts, "POST", "/api/admin/bookmarks/import/preview", importJSONBody(t, "auto", []byte(transferHTML), nil), ck, nil)
	if pv.code != http.StatusOK {
		t.Fatalf("预览失败：%d %s", pv.code, pv.body)
	}
	m := pv.json(t)
	if m["format"] != transfer.FormatHTML || int(m["total"].(float64)) != 2 || int(m["new"].(float64)) != 1 || int(m["duplicates"].(float64)) != 1 || int(m["invalid"].(float64)) != 0 {
		t.Fatalf("预览统计不符：%v", m)
	}
	items := m["items"].([]any)
	folder0 := items[0].(map[string]any)["folder_path"].([]any)
	if len(folder0) != 2 || folder0[0] != "工作" || folder0[1] != "项目" {
		t.Fatalf("预览未展示完整层级：%v", items[0])
	}
	if listTotal(t, ts, ck) != 1 {
		t.Fatalf("预览阶段不得修改数据库")
	}

	// 确认执行：默认跳过重复
	ex := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(transferHTML), nil), ck, nil)
	if ex.code != http.StatusOK {
		t.Fatalf("执行导入失败：%d %s", ex.code, ex.body)
	}
	sum := ex.json(t)
	if int(sum["created"].(float64)) != 1 || int(sum["skipped"].(float64)) != 1 || int(sum["failed"].(float64)) != 0 || int(sum["total"].(float64)) != 2 {
		t.Fatalf("导入结果不符：%v", sum)
	}
	if listTotal(t, ts, ck) != 2 {
		t.Fatalf("导入后应有 2 条书签")
	}
	listAfter := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil).json(t)
	firstID := int64(sum["items"].([]any)[0].(map[string]any)["bookmark_id"].(float64))
	first := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d", firstID), "", ck, nil).json(t)
	p := first["folder_path"].([]any)
	if len(p) != 2 || p[0] != "工作" || p[1] != "项目" {
		t.Fatalf("导入后层级未持久化：%v（列表 %v）", first, listAfter)
	}

	// 重复执行：全部跳过
	again := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(transferHTML), nil), ck, nil)
	if again.code != http.StatusOK {
		t.Fatalf("重复导入失败：%d %s", again.code, again.body)
	}
	if sum := again.json(t); int(sum["created"].(float64)) != 0 || int(sum["skipped"].(float64)) != 2 {
		t.Fatalf("重复导入应全部跳过：%v", sum)
	}

	// 显式覆盖：只覆盖来源支持字段，原有 private 权限必须保留
	ow := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(transferHTML), map[string]any{"duplicates": "overwrite"}), ck, nil)
	if ow.code != http.StatusOK {
		t.Fatalf("覆盖导入失败：%d %s", ow.code, ow.body)
	}
	if sum := ow.json(t); int(sum["overwritten"].(float64)) != 2 || int(sum["created"].(float64)) != 0 {
		t.Fatalf("覆盖结果不符：%v", sum)
	}
	got := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d", oldID), "", ck, nil)
	if got.code != http.StatusOK || got.json(t)["visibility"] != bookmark.VisibilityPrivate {
		t.Fatalf("覆盖不得把 private 书签意外公开：%d %v", got.code, got.json(t))
	}

	// keep_both：再新增两条
	kb := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(transferHTML), map[string]any{"duplicates": "keep_both"}), ck, nil)
	if sum := kb.json(t); int(sum["created"].(float64)) != 2 {
		t.Fatalf("keep_both 应新增 2 条：%v", sum)
	}

	// 选定范围：只导入 index=0，其余跳过
	sel := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(transferHTML), map[string]any{"selected": []int{0}, "duplicates": "keep_both"}), ck, nil)
	if sum := sel.json(t); int(sum["created"].(float64)) != 1 || int(sum["skipped"].(float64)) != 1 {
		t.Fatalf("选定导入结果不符：%v", sum)
	}

	// 批内重复 + overwrite：第二条应覆盖刚创建的第一条，而不是再新增一条
	batchHTML := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="https://batch.example.com/">批内一</A>
<DT><A HREF="https://batch.example.com/">批内二</A>
</DL>`
	beforeBatch := listTotal(t, ts, ck)
	batch := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(batchHTML), map[string]any{"duplicates": "overwrite"}), ck, nil)
	if batch.code != http.StatusOK {
		t.Fatalf("批内覆盖导入失败：%d %s", batch.code, batch.body)
	}
	if sum := batch.json(t); int(sum["created"].(float64)) != 1 || int(sum["overwritten"].(float64)) != 1 {
		t.Fatalf("批内重复 overwrite 结果不符：%v", sum)
	}
	if afterBatch := listTotal(t, ts, ck); afterBatch != beforeBatch+1 {
		t.Fatalf("批内重复 overwrite 不应新增两条：before=%d after=%d", beforeBatch, afterBatch)
	}
}

func TestLocalHubJSONExportImportWithIcon(t *testing.T) {
	s, ts := newTestServer(t, nil)
	ck := adminCookie(t, ts, s)

	created := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"迁移站","wan_url":"https://migrate.example.com","visibility":"authenticated","folder_path":["迁移","图标"]}`, ck, nil)
	if created.code != http.StatusCreated {
		t.Fatalf("创建书签失败：%d %s", created.code, created.body)
	}
	id := bookmarkID(t, created)
	png := pngBody(t, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if up := doRaw(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), png, ck, "image/png"); up.code != http.StatusOK {
		t.Fatalf("上传图标失败：%d %s", up.code, up.body)
	}

	ex := do(t, ts, "GET", "/api/admin/bookmarks/export?format=json", "", ck, nil)
	if ex.code != http.StatusOK || !strings.Contains(ex.header.Get("Content-Type"), "application/json") {
		t.Fatalf("JSON 导出失败：%d %s", ex.code, ex.header.Get("Content-Type"))
	}
	if !strings.Contains(ex.header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("导出应带下载文件名：%v", ex.header)
	}
	var doc struct {
		Format    string `json:"format"`
		Version   int    `json:"version"`
		Count     int    `json:"count"`
		Bookmarks []struct {
			IconDataBase64 string   `json:"icon_data_base64"`
			Visibility     string   `json:"visibility"`
			FolderPath     []string `json:"folder_path"`
		} `json:"bookmarks"`
	}
	if err := json.Unmarshal(ex.body, &doc); err != nil {
		t.Fatalf("导出 JSON 非法：%v", err)
	}
	if doc.Format != transfer.JSONFormatName || doc.Version != transfer.JSONFormatVersion || doc.Count != 1 {
		t.Fatalf("导出头信息不符：%+v", doc)
	}
	if doc.Bookmarks[0].IconDataBase64 == "" || doc.Bookmarks[0].Visibility != bookmark.VisibilityAuthenticated || len(doc.Bookmarks[0].FolderPath) != 2 {
		t.Fatalf("导出未包含图标、权限或层级：%+v", doc.Bookmarks[0])
	}
	if strings.Contains(string(ex.body), "icons/") {
		t.Fatalf("导出不得包含服务器本地路径")
	}

	html := do(t, ts, "GET", "/api/admin/bookmarks/export?format=html", "", ck, nil)
	if html.code != http.StatusOK || !strings.Contains(string(html.body), "NETSCAPE-Bookmark-file-1") || !strings.Contains(string(html.body), "LOCALHUB_VISIBILITY=") {
		t.Fatalf("HTML 导出不兼容：%d %s", html.code, html.body)
	}
	if !strings.Contains(string(html.body), "<H3") || !strings.Contains(string(html.body), "迁移") || !strings.Contains(string(html.body), "图标") {
		t.Fatalf("HTML 导出未表达文件夹层级：%s", html.body)
	}

	selected := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/export?format=json&scope=selected&ids=%d", id), "", ck, nil)
	if selected.code != http.StatusOK || !strings.Contains(string(selected.body), `"count": 1`) {
		t.Fatalf("选定导出不符：%d %s", selected.code, selected.body)
	}
	if bad := do(t, ts, "GET", "/api/admin/bookmarks/export?format=json&scope=selected&ids=abc", "", ck, nil); bad.code != http.StatusBadRequest {
		t.Fatalf("非法 ID 应 400，got %d", bad.code)
	}
	vis := do(t, ts, "GET", "/api/admin/bookmarks/export?format=json&visibility=authenticated", "", ck, nil)
	if vis.code != http.StatusOK || !strings.Contains(string(vis.body), `"count": 1`) {
		t.Fatalf("权限过滤导出不符：%d %s", vis.code, vis.body)
	}
	publicOnly := do(t, ts, "GET", "/api/admin/bookmarks/export?format=json&visibility=public", "", ck, nil)
	if publicOnly.code != http.StatusOK || !strings.Contains(string(publicOnly.body), `"count": 0`) {
		t.Fatalf("public 过滤应为空：%d %s", publicOnly.code, publicOnly.body)
	}

	// 删除后从导出文件恢复，图标必须随 JSON 迁移
	if del := do(t, ts, "DELETE", fmt.Sprintf("/api/admin/bookmarks/%d", id), "", ck, nil); del.code != http.StatusOK {
		t.Fatalf("删除书签失败：%d %s", del.code, del.body)
	}
	if listTotal(t, ts, ck) != 0 {
		t.Fatalf("删除后应为空")
	}
	imp := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "localhub", ex.body, nil), ck, nil)
	if imp.code != http.StatusOK {
		t.Fatalf("JSON 导入失败：%d %s", imp.code, imp.body)
	}
	if sum := imp.json(t); int(sum["created"].(float64)) != 1 || int(sum["icons_imported"].(float64)) != 1 {
		t.Fatalf("JSON 导入结果不符：%v", sum)
	}
	if listTotal(t, ts, ck) != 1 {
		t.Fatalf("恢复后应有 1 条书签")
	}
	list := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil).json(t)
	newItem := list["items"].([]any)[0].(map[string]any)
	newID := int64(newItem["id"].(float64))
	if p := newItem["folder_path"].([]any); len(p) != 2 || p[0] != "迁移" || p[1] != "图标" {
		t.Fatalf("JSON 回导后层级不一致：%v", newItem)
	}
	icon := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", newID), "", ck, nil)
	if icon.code != http.StatusOK || icon.header.Get("Content-Type") != "image/png" {
		t.Fatalf("迁移后的图标不可读：%d %s", icon.code, icon.header.Get("Content-Type"))
	}
	if string(icon.body) != string(png) {
		t.Fatalf("迁移后的图标内容不一致")
	}
	// 覆盖导入：内嵌图标可重新写入，不报“本地图标”错误
	ow := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "localhub", ex.body, map[string]any{"duplicates": "overwrite"}), ck, nil)
	if sum := ow.json(t); int(sum["overwritten"].(float64)) != 1 || int(sum["icons_imported"].(float64)) != 1 {
		t.Fatalf("图标覆盖导入失败：%v", sum)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d/icon", newID), "", ck, nil); r.code != http.StatusOK {
		t.Fatalf("覆盖后图标不可读：%d", r.code)
	}
	// 再次导入：跳过
	re := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "localhub", ex.body, nil), ck, nil)
	if sum := re.json(t); int(sum["skipped"].(float64)) != 1 || int(sum["created"].(float64)) != 0 {
		t.Fatalf("再次导入应跳过：%v", sum)
	}
}

func TestImportPartialFailureAndGuards(t *testing.T) {
	s, ts := newTestServer(t, nil)
	ck := adminCookie(t, ts, s)

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/admin/bookmarks/import/preview", `{"format":"auto","data_base64":""}`},
		{"POST", "/api/admin/bookmarks/import/execute", `{"format":"auto","data_base64":""}`},
		{"GET", "/api/admin/bookmarks/export", ""},
	} {
		if r := do(t, ts, tc.method, tc.path, tc.body, "", nil); r.code != http.StatusUnauthorized {
			t.Fatalf("访客访问 %s 必须 401，got %d", tc.path, r.code)
		}
	}

	if r := do(t, ts, "POST", "/api/admin/bookmarks/import/preview", `{"format":"auto","data_base64":"!!!"}`, ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("非法 base64 应 400，got %d", r.code)
	}
	if r := do(t, ts, "POST", "/api/admin/bookmarks/import/preview", importJSONBody(t, "auto", []byte("hello world"), nil), ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("无法识别的文件应 400，got %d", r.code)
	}

	sun := `{"data":[{"name":"工作","apps":[{"name":"有效","url":"https://ok.example.com"},{"name":"","url":"https://broken.example.com"}]}]}`
	pv := do(t, ts, "POST", "/api/admin/bookmarks/import/preview", importJSONBody(t, "auto", []byte(sun), nil), ck, nil)
	if pv.code != http.StatusOK {
		t.Fatalf("Sun-Panel 预览失败：%d %s", pv.code, pv.body)
	}
	m := pv.json(t)
	if int(m["total"].(float64)) != 2 || int(m["new"].(float64)) != 1 || int(m["invalid"].(float64)) != 1 {
		t.Fatalf("Sun-Panel 预览统计不符：%v", m)
	}
	if p := m["items"].([]any)[0].(map[string]any)["folder_path"].([]any); len(p) != 1 || p[0] != "工作" {
		t.Fatalf("Sun-Panel 预览未展示层级：%v", m["items"].([]any)[0])
	}
	ex := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(sun), nil), ck, nil)
	if sum := ex.json(t); int(sum["created"].(float64)) != 1 || int(sum["failed"].(float64)) != 1 {
		t.Fatalf("部分失败结果不符：%v", sum)
	}
	if listTotal(t, ts, ck) != 1 {
		t.Fatalf("失败记录不应写入数据库")
	}
	okItem := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil).json(t)["items"].([]any)[0].(map[string]any)
	if p := okItem["folder_path"].([]any); len(p) != 1 || p[0] != "工作" {
		t.Fatalf("Sun-Panel 层级未持久化：%v", okItem)
	}

	if r := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "auto", []byte(sun), map[string]any{"duplicates": "drop"}), ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("非法 duplicates 应 400，got %d", r.code)
	}
}

func TestImportInvalidIconDoesNotBlockBookmark(t *testing.T) {
	s, ts := newTestServer(t, nil)
	ck := adminCookie(t, ts, s)

	badIcon := base64.StdEncoding.EncodeToString([]byte("not-an-image"))
	raw := `{"format":"localhub-bookmarks","version":1,"bookmarks":[{"name":"坏图标","wan_url":"https://badicon.example.com","icon_type":"file","icon_data_base64":"` + badIcon + `"}]}`
	pv := do(t, ts, "POST", "/api/admin/bookmarks/import/preview", importJSONBody(t, "localhub", []byte(raw), nil), ck, nil)
	if pv.code != http.StatusOK {
		t.Fatalf("预览失败：%d %s", pv.code, pv.body)
	}
	warns := pv.json(t)["warnings"].([]any)
	var found bool
	for _, raw := range warns {
		if raw.(map[string]any)["code"] == "icon_invalid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应提示内嵌图标非法：%v", warns)
	}
	ex := do(t, ts, "POST", "/api/admin/bookmarks/import/execute", importJSONBody(t, "localhub", []byte(raw), nil), ck, nil)
	sum := ex.json(t)
	if int(sum["created"].(float64)) != 1 || int(sum["icons_failed"].(float64)) != 1 {
		t.Fatalf("坏图标不应阻断书签导入：%v", sum)
	}
	list := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil).json(t)
	id := int64(list["items"].([]any)[0].(map[string]any)["id"].(float64))
	if r := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d/icon", id), "", ck, nil); r.code == http.StatusOK {
		t.Fatalf("坏图标不应被保存")
	}
}
