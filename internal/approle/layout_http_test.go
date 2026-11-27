package approle

import (
	"fmt"
	"net/http"
	"testing"
)

func TestLayoutPermissionsAndPersistence(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	// 访客只能读，不能写。
	if r := do(t, ts, "GET", "/api/bookmarks/layout", "", "", nil); r.code != http.StatusOK {
		t.Fatalf("访客读取布局应 200，got %d body=%s", r.code, r.body)
	}
	if r := do(t, ts, "PUT", "/api/admin/bookmarks/layout", `{"desktop":[],"mobile":[]}`, "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("访客写布局必须 401，got %d", r.code)
	}

	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 || login.cookie == "" {
		t.Fatalf("管理员登录失败：%d", login.code)
	}
	ck := login.cookie
	create := func(body string) int64 {
		t.Helper()
		r := do(t, ts, "POST", "/api/admin/bookmarks", body, ck, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("新增失败：%d body=%s", r.code, r.body)
		}
		return int64(r.json(t)["id"].(float64))
	}
	pub := create(`{"name":"公开","wan_url":"https://pub.example.com","visibility":"public"}`)
	priv := create(`{"name":"私有","wan_url":"https://priv.example.com","visibility":"private"}`)
	guestHidden := create(`{"name":"登录可见","wan_url":"https://auth.example.com","visibility":"authenticated"}`)

	body := fmt.Sprintf(`{"desktop":[{"id":%d,"size":"large"},{"id":%d,"size":"small"},{"id":%d,"size":"medium"}],"mobile":[{"id":%d,"size":"medium"}]}`,
		priv, pub, guestHidden, pub)
	if r := do(t, ts, "PUT", "/api/admin/bookmarks/layout", body, ck, nil); r.code != http.StatusOK {
		t.Fatalf("管理员保存布局失败：%d body=%s", r.code, r.body)
	}

	// 管理员读取到全部三项。
	adminLayout := do(t, ts, "GET", "/api/bookmarks/layout", "", ck, nil)
	if adminLayout.code != http.StatusOK {
		t.Fatalf("管理员读取布局失败：%d", adminLayout.code)
	}
	desktop := adminLayout.json(t)["desktop"].([]any)
	if len(desktop) != 3 || int64(desktop[0].(map[string]any)["id"].(float64)) != priv {
		t.Fatalf("管理员布局不正确：%v", adminLayout.json(t))
	}

	// 访客只看到公开书签的布局项，私有 ID 不泄露。
	guestLayout := do(t, ts, "GET", "/api/bookmarks/layout", "", "", nil)
	if guestLayout.code != http.StatusOK {
		t.Fatalf("访客读取布局失败：%d", guestLayout.code)
	}
	gdesk := guestLayout.json(t)["desktop"].([]any)
	gmob := guestLayout.json(t)["mobile"].([]any)
	if len(gdesk) != 1 || int64(gdesk[0].(map[string]any)["id"].(float64)) != pub || len(gmob) != 1 {
		t.Fatalf("访客布局未按可见性过滤：%s", guestLayout.body)
	}

	// 非法尺寸/重复/不存在 ID 均拒绝，且不破坏已保存布局。
	for _, bad := range []string{
		fmt.Sprintf(`{"desktop":[{"id":%d,"size":"huge"}],"mobile":[]}`, pub),
		fmt.Sprintf(`{"desktop":[{"id":%d,"size":"small"},{"id":%d,"size":"small"}],"mobile":[]}`, pub, pub),
		`{"desktop":[{"id":9999,"size":"small"}],"mobile":[]}`,
	} {
		if r := do(t, ts, "PUT", "/api/admin/bookmarks/layout", bad, ck, nil); r.code != http.StatusBadRequest {
			t.Fatalf("非法布局应 400，got %d body=%s", r.code, r.body)
		}
	}
	after := do(t, ts, "GET", "/api/bookmarks/layout", "", ck, nil).json(t)
	if len(after["desktop"].([]any)) != 3 {
		t.Fatalf("非法保存不应破坏旧布局：%v", after)
	}

	// 删除书签后对应布局行清理。
	if r := do(t, ts, "DELETE", fmt.Sprintf("/api/admin/bookmarks/%d", priv), "", ck, nil); r.code != http.StatusOK {
		t.Fatalf("删除失败：%d", r.code)
	}
	clean := do(t, ts, "GET", "/api/bookmarks/layout", "", ck, nil).json(t)
	if len(clean["desktop"].([]any)) != 2 {
		t.Fatalf("删除后布局未清理：%v", clean)
	}
}
