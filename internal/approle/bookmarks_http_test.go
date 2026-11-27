package approle

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBookmarkAdminCRUDAndReorder(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 || login.cookie == "" {
		t.Fatalf("管理员登录失败：%d", login.code)
	}
	ck := login.cookie

	create := func(body string) response {
		t.Helper()
		return do(t, ts, "POST", "/api/admin/bookmarks", body, ck, nil)
	}

	r1 := create(`{"name":"NAS","lan_url":"http://192.168.1.10:5000","wan_url":"https://nas.example.com","description":"群晖","icon_type":"emoji","icon_value":"💾","visibility":"public"}`)
	if r1.code != http.StatusCreated {
		t.Fatalf("新增书签应 201，got %d body=%s", r1.code, r1.body)
	}
	id1 := int64(r1.json(t)["id"].(float64))

	r2 := create(`{"name":"面板","wan_url":"https://panel.example.com","visibility":"authenticated","open_mode":"self"}`)
	if r2.code != http.StatusCreated {
		t.Fatalf("新增书签应 201，got %d body=%s", r2.code, r2.body)
	}
	id2 := int64(r2.json(t)["id"].(float64))

	r3 := create(`{"name":"私藏","wan_url":"https://secret.example.com","visibility":"private"}`)
	if r3.code != http.StatusCreated {
		t.Fatalf("新增书签应 201，got %d body=%s", r3.code, r3.body)
	}
	id3 := int64(r3.json(t)["id"].(float64))

	r4 := create(`{"name":"停用","wan_url":"https://off.example.com","enabled":false}`)
	if r4.code != http.StatusCreated {
		t.Fatalf("新增停用书签应 201，got %d body=%s", r4.code, r4.body)
	}
	id4 := int64(r4.json(t)["id"].(float64))

	// 管理端列表含停用项
	list := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil)
	if list.code != 200 {
		t.Fatalf("管理列表失败：%d", list.code)
	}
	if total := list.json(t)["total"].(float64); int(total) != 4 {
		t.Fatalf("管理列表应含 4 条，got %v", total)
	}

	// 单条读取
	got := do(t, ts, "GET", fmt.Sprintf("/api/admin/bookmarks/%d", id1), "", ck, nil)
	if got.code != 200 {
		t.Fatalf("读取单条失败：%d body=%s", got.code, got.body)
	}
	m := got.json(t)
	if m["name"] != "NAS" || m["lan_url"] != "http://192.168.1.10:5000" || m["effective_visibility"] != "public" {
		t.Fatalf("单条字段不正确：%v", m)
	}

	// 编辑：只改名称，其余保留
	upd := do(t, ts, "PUT", fmt.Sprintf("/api/admin/bookmarks/%d", id1), `{"name":"NAS 主控"}`, ck, nil)
	if upd.code != 200 {
		t.Fatalf("编辑失败：%d body=%s", upd.code, upd.body)
	}
	if m := upd.json(t); m["name"] != "NAS 主控" || m["wan_url"] != "https://nas.example.com" || m["icon_value"] != "💾" {
		t.Fatalf("编辑应保留未提供字段：%v", m)
	}

	// 编辑不存在 → 404
	if r := do(t, ts, "PUT", "/api/admin/bookmarks/9999", `{"name":"x"}`, ck, nil); r.code != http.StatusNotFound {
		t.Fatalf("编辑不存在应 404，got %d", r.code)
	}

	// 排重排序：3,1,2,4
	order := fmt.Sprintf(`{"ids":[%d,%d,%d,%d]}`, id3, id1, id2, id4)
	if r := do(t, ts, "POST", "/api/admin/bookmarks/order", order, ck, nil); r.code != 200 {
		t.Fatalf("排序失败：%d body=%s", r.code, r.body)
	}
	items := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil).json(t)["items"].([]any)
	want := []int64{id3, id1, id2, id4}
	for i, it := range items {
		if int64(it.(map[string]any)["id"].(float64)) != want[i] || int(it.(map[string]any)["sort_order"].(float64)) != i {
			t.Fatalf("第 %d 项排序不正确：%v", i, it)
		}
	}

	// 排序含未知 ID → 错误且不改动
	if r := do(t, ts, "POST", "/api/admin/bookmarks/order", fmt.Sprintf(`{"ids":[%d,9999]}`, id1), ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("排序含未知 ID 应 400，got %d body=%s", r.code, r.body)
	}
	// 排序重复 ID → 400
	if r := do(t, ts, "POST", "/api/admin/bookmarks/order", fmt.Sprintf(`{"ids":[%d,%d]}`, id1, id1), ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("排序重复 ID 应 400，got %d", r.code)
	}

	// 删除单条
	if r := do(t, ts, "DELETE", fmt.Sprintf("/api/admin/bookmarks/%d", id4), "", ck, nil); r.code != 200 {
		t.Fatalf("删除失败：%d body=%s", r.code, r.body)
	}
	if r := do(t, ts, "DELETE", fmt.Sprintf("/api/admin/bookmarks/%d", id4), "", ck, nil); r.code != http.StatusNotFound {
		t.Fatalf("重复删除应 404，got %d", r.code)
	}

	// 批量删除含不存在 ID
	bd := do(t, ts, "POST", "/api/admin/bookmarks/batch-delete", fmt.Sprintf(`{"ids":[%d,9998]}`, id2), ck, nil)
	if bd.code != 200 {
		t.Fatalf("批量删除失败：%d body=%s", bd.code, bd.body)
	}
	m = bd.json(t)
	if int(m["deleted"].(float64)) != 1 {
		t.Fatalf("deleted 应为 1：%v", m)
	}
	missing := m["missing"].([]any)
	if len(missing) != 1 || int64(missing[0].(float64)) != 9998 {
		t.Fatalf("missing 应为 [9998]：%v", m)
	}

	// 批量超上限
	ids := ""
	for i := 0; i < 201; i++ {
		ids += "1,"
	}
	if r := do(t, ts, "POST", "/api/admin/bookmarks/batch-delete", `{"ids":[`+ids[:len(ids)-1]+`]}`, ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("批量超上限应 400，got %d", r.code)
	}
}

func TestBookmarkValidationRejected(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	ck := login.cookie

	cases := []string{
		`{"name":"x"}`, // 没有任何 URL
		`{"name":"x","wan_url":"javascript:alert(1)"}`,                           // 非 http(s)
		`{"name":"x","lan_url":"http://"}`,                                       // 无主机
		`{"name":"","wan_url":"https://a.example.com"}`,                          // 名称空
		`{"name":"x","wan_url":"https://a.example.com","visibility":"root"}`,     // 权限非法
		`{"name":"x","wan_url":"https://a.example.com","visibility":"disabled"}`, // 禁用不是可写入的权限
		`{"name":"x","wan_url":"https://a.example.com","open_mode":"popup"}`,
		`{"name":"x","wan_url":"https://a.example.com","icon_type":"url","icon_value":"file:///etc/passwd"}`,
		`{"name":"x","wan_url":"https://a.example.com","sort_order":-3}`,
	}
	for _, body := range cases {
		if r := do(t, ts, "POST", "/api/admin/bookmarks", body, ck, nil); r.code != http.StatusBadRequest {
			t.Fatalf("非法输入应 400，got %d body=%s input=%s", r.code, r.body, body)
		}
	}
	if r := do(t, ts, "GET", "/api/admin/bookmarks/abc", "", ck, nil); r.code != http.StatusBadRequest {
		t.Fatalf("非法 ID 应 400，got %d", r.code)
	}
}

func TestBookmarkPermissionBoundaries(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	admin := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if admin.code != 200 {
		t.Fatalf("登录失败：%d", admin.code)
	}
	ck := admin.cookie

	pub := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"公开","wan_url":"https://pub.example.com","visibility":"public"}`, ck, nil)
	authn := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"登录","wan_url":"https://authn.example.com","visibility":"authenticated"}`, ck, nil)
	priv := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"私有","wan_url":"https://priv.example.com","visibility":"private"}`, ck, nil)
	off := do(t, ts, "POST", "/api/admin/bookmarks", `{"name":"停用","wan_url":"https://off.example.com","enabled":false}`, ck, nil)
	for _, r := range []response{pub, authn, priv, off} {
		if r.code != http.StatusCreated {
			t.Fatalf("准备数据失败：%d body=%s", r.code, r.body)
		}
	}
	pubID := int64(pub.json(t)["id"].(float64))
	authnID := int64(authn.json(t)["id"].(float64))
	privID := int64(priv.json(t)["id"].(float64))
	offID := int64(off.json(t)["id"].(float64))

	// 访客：列表只有公开项
	guest := do(t, ts, "GET", "/api/bookmarks", "", "", nil)
	if guest.code != 200 {
		t.Fatalf("访客读取公开列表失败：%d", guest.code)
	}
	m := guest.json(t)
	if m["role"] != "guest" || int(m["total"].(float64)) != 1 {
		t.Fatalf("访客应只看到 1 条公开书签：%v", m)
	}
	if items := m["items"].([]any); len(items) != 1 || items[0].(map[string]any)["name"] != "公开" {
		t.Fatalf("访客列表内容不正确：%v", items)
	}

	// 访客单条：公开可读，其余一律 404（不泄露存在性）
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d", pubID), "", "", nil); r.code != 200 {
		t.Fatalf("访客应可读公开书签，got %d", r.code)
	}
	for _, id := range []int64{authnID, privID, offID} {
		if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d", id), "", "", nil); r.code != http.StatusNotFound {
			t.Fatalf("访客读取 ID=%d 应 404，got %d body=%s", id, r.code, r.body)
		}
	}

	// 管理员：公开列表可见 public+authenticated+private，不含停用
	admList := do(t, ts, "GET", "/api/bookmarks", "", ck, nil)
	if admList.code != 200 {
		t.Fatalf("管理员读取列表失败：%d", admList.code)
	}
	m = admList.json(t)
	if m["role"] != "admin" || int(m["total"].(float64)) != 3 {
		t.Fatalf("管理员应可见 3 条启用书签：%v", m)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d", privID), "", ck, nil); r.code != 200 {
		t.Fatalf("管理员应可读私有书签，got %d", r.code)
	}
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d", offID), "", ck, nil); r.code != http.StatusNotFound {
		t.Fatalf("停用书签在公开接口应 404，got %d", r.code)
	}

	// 访客访问管理接口：全部 401
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/admin/bookmarks", ""},
		{"GET", fmt.Sprintf("/api/admin/bookmarks/%d", pubID), ""},
		{"POST", "/api/admin/bookmarks", `{"name":"x","wan_url":"https://x.example.com"}`},
		{"PUT", fmt.Sprintf("/api/admin/bookmarks/%d", pubID), `{"name":"hacked"}`},
		{"DELETE", fmt.Sprintf("/api/admin/bookmarks/%d", pubID), ""},
		{"POST", "/api/admin/bookmarks/batch-delete", `{"ids":[1]}`},
		{"POST", "/api/admin/bookmarks/order", `{"ids":[1]}`},
	} {
		if r := do(t, ts, tc.method, tc.path, tc.body, "", nil); r.code != http.StatusUnauthorized {
			t.Fatalf("访客 %s %s 应 401，got %d body=%s", tc.method, tc.path, r.code, r.body)
		}
	}
	// 确认越权尝试没有改动数据
	if r := do(t, ts, "GET", fmt.Sprintf("/api/bookmarks/%d", pubID), "", "", nil); r.code != 200 {
		t.Fatalf("公开书签应仍在，got %d", r.code)
	}

	// 退出后（会话失效）再访问管理接口 → 401
	if r := do(t, ts, "POST", "/api/auth/logout", "", ck, nil); r.code != 200 {
		t.Fatalf("退出失败：%d", r.code)
	}
	if r := do(t, ts, "GET", "/api/admin/bookmarks", "", ck, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("退出后管理接口应 401，got %d", r.code)
	}
}
