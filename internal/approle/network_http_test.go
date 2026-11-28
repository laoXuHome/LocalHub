package approle

import (
	"net/http"
	"testing"
)

func TestNetworkTargetsAdminOnlyAndValidation(t *testing.T) {
	s, ts := newTestServer(t, nil)

	// 访客不能读，也不能写。
	if r := do(t, ts, "GET", "/api/admin/network/targets", "", "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("访客读取应 401，got %d body=%s", r.code, r.body)
	}
	guestWrite := `{"targets":[
		{"key":"router","address":"10.0.0.1","enabled":true},
		{"key":"bypass_router","address":"","enabled":false},
		{"key":"nas","address":"","enabled":false}]}`
	if r := do(t, ts, "PUT", "/api/admin/network/targets", guestWrite, "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("访客写入应 401，got %d body=%s", r.code, r.body)
	}

	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 || login.cookie == "" {
		t.Fatalf("管理员登录失败：%d body=%s", login.code, login.body)
	}
	ck := login.cookie

	get := do(t, ts, "GET", "/api/admin/network/targets", "", ck, nil)
	if get.code != 200 {
		t.Fatalf("管理员读取失败：%d body=%s", get.code, get.body)
	}
	m := get.json(t)
	targets := m["targets"].([]any)
	if len(targets) != 3 {
		t.Fatalf("应有 3 个目标：%v", m)
	}
	first := targets[0].(map[string]any)
	if first["key"] != "router" || first["label"] != "主路由器" || first["enabled"] != true {
		t.Fatalf("默认主路由器不正确：%v", first)
	}
	if first["address"] == "192.168.1.1" {
		t.Fatal("默认配置不能写死常见用户网段地址")
	}

	// 合法保存：三个目标独立编辑，旁路由/NAS 启用。
	valid := `{"targets":[
		{"key":"router","address":"10.20.0.1","enabled":true},
		{"key":"bypass_router","address":"10.20.0.2","enabled":true},
		{"key":"nas","address":"nas.lab","enabled":false}]}`
	save := do(t, ts, "PUT", "/api/admin/network/targets", valid, ck, nil)
	if save.code != 200 {
		t.Fatalf("合法保存应 200，got %d body=%s", save.code, save.body)
	}
	saved := save.json(t)["targets"].([]any)
	if saved[1].(map[string]any)["address"] != "10.20.0.2" || saved[2].(map[string]any)["address"] != "nas.lab" {
		t.Fatalf("保存内容不正确：%v", saved)
	}

	// 刷新读取仍存在。
	again := do(t, ts, "GET", "/api/admin/network/targets", "", ck, nil).json(t)["targets"].([]any)
	if again[0].(map[string]any)["address"] != "10.20.0.1" || again[1].(map[string]any)["enabled"] != true {
		t.Fatalf("重新读取配置丢失：%v", again)
	}

	badCases := []struct {
		name string
		body string
	}{
		{"全部禁用", `{"targets":[
			{"key":"router","address":"10.20.0.1","enabled":false},
			{"key":"bypass_router","address":"10.20.0.2","enabled":false},
			{"key":"nas","address":"nas.lab","enabled":false}]}`},
		{"非法地址", `{"targets":[
			{"key":"router","address":"http://10.20.0.1","enabled":true},
			{"key":"bypass_router","address":"","enabled":false},
			{"key":"nas","address":"","enabled":false}]}`},
		{"非法 IPv4", `{"targets":[
			{"key":"router","address":"999.999.999.999","enabled":true},
			{"key":"bypass_router","address":"","enabled":false},
			{"key":"nas","address":"","enabled":false}]}`},
		{"缺少目标", `{"targets":[
			{"key":"router","address":"10.20.0.1","enabled":true}]}`},
		{"未知目标", `{"targets":[
			{"key":"router","address":"10.20.0.1","enabled":true},
			{"key":"switch","address":"10.20.0.2","enabled":false},
			{"key":"nas","address":"","enabled":false}]}`},
		{"重复目标", `{"targets":[
			{"key":"router","address":"10.20.0.1","enabled":true},
			{"key":"router","address":"10.20.0.2","enabled":true},
			{"key":"nas","address":"","enabled":false}]}`},
	}
	for _, tc := range badCases {
		r := do(t, ts, "PUT", "/api/admin/network/targets", tc.body, ck, nil)
		if r.code != http.StatusBadRequest {
			t.Errorf("%s 应 400，got %d body=%s", tc.name, r.code, r.body)
		}
	}

	// 非法保存不改变已持久化配置。
	final := do(t, ts, "GET", "/api/admin/network/targets", "", ck, nil).json(t)["targets"].([]any)
	if final[0].(map[string]any)["address"] != "10.20.0.1" || final[1].(map[string]any)["address"] != "10.20.0.2" {
		t.Fatalf("非法保存后配置被改动：%v", final)
	}

	// 只改启用状态也要能保存；至少保留一个启用。
	toggle := `{"targets":[
		{"key":"router","address":"10.20.0.1","enabled":false},
		{"key":"bypass_router","address":"10.20.0.2","enabled":true},
		{"key":"nas","address":"nas.lab","enabled":false}]}`
	if r := do(t, ts, "PUT", "/api/admin/network/targets", toggle, ck, nil); r.code != 200 {
		t.Fatalf("切换启用状态失败：%d body=%s", r.code, r.body)
	}
}
