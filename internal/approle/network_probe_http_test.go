package approle

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/network"
	"github.com/laoXuHome/LocalHub/internal/probe"
)

func TestNetworkProbePermissionsAndPublicRedaction(t *testing.T) {
	_, ts := newTestServer(t, nil)

	public := do(t, ts, "GET", "/api/network/status", "", "", nil)
	if public.code != http.StatusOK {
		t.Fatalf("访客应能读取公开状态：%d body=%s", public.code, public.body)
	}
	if strings.Contains(string(public.body), "address") || strings.Contains(string(public.body), "router.local") {
		t.Fatalf("公开状态不能暴露目标地址：%s", public.body)
	}

	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/admin/network/results", ""},
		{"POST", "/api/admin/network/probe", ""},
		{"GET", "/api/admin/network/probes/probe-x", ""},
	} {
		r := do(t, ts, tc.method, tc.path, tc.body, "", nil)
		if r.code != http.StatusUnauthorized {
			t.Fatalf("%s %s 应拒绝访客，got %d body=%s", tc.method, tc.path, r.code, r.body)
		}
	}
	if r := do(t, ts, "GET", "/api/network/probe?url=http://evil.test", "", "", nil); r.code != http.StatusNotFound {
		t.Fatalf("不存在任意 URL 探测接口，got %d", r.code)
	}
}

func TestNetworkProbeAdminRunRedactionAndDedup(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 || login.cookie == "" {
		t.Fatalf("管理员登录失败：%d body=%s", login.code, login.body)
	}
	ck := login.cookie
	cfg := `{"targets":[
		{"key":"router","address":"10.20.0.1","enabled":true},
		{"key":"bypass_router","address":"10.20.0.2","enabled":true},
		{"key":"nas","address":"10.20.0.3","enabled":true}]}`
	if r := do(t, ts, "PUT", "/api/admin/network/targets", cfg, ck, nil); r.code != 200 {
		t.Fatalf("保存三个目标失败：%d body=%s", r.code, r.body)
	}

	block := make(chan struct{})
	stub := &probeStub{
		block: block,
		results: map[network.TargetKey]probe.Result{
			network.TargetRouter:       {Status: probe.StatusSuccess, HTTPStatus: 200, LatencyMS: 6, CheckedAt: "2026-11-28T02:20:00Z"},
			network.TargetBypassRouter: {Status: probe.StatusTimeout, FailureInfo: "dial tcp 10.20.0.2: i/o timeout", CheckedAt: "2026-11-28T02:20:00Z"},
			network.TargetNAS:          {Status: probe.StatusHTTPError, HTTPStatus: 500, FailureInfo: "HTTP 状态码 500", CheckedAt: "2026-11-28T02:20:00Z"},
		},
	}
	s.probe.Prober = stub

	start := do(t, ts, "POST", "/api/admin/network/probe", `{"url":"http://evil.test"}`, ck, nil)
	if start.code != http.StatusAccepted {
		t.Fatalf("管理员触发应 202：%d body=%s", start.code, start.body)
	}
	started := start.json(t)
	id, _ := started["id"].(string)
	if id == "" || started["status"] != "running" {
		t.Fatalf("触发响应错误：%v", started)
	}

	dup := do(t, ts, "POST", "/api/admin/network/probe", "", ck, nil)
	if dup.code != http.StatusConflict {
		t.Fatalf("重复触发应 409：%d body=%s", dup.code, dup.body)
	}
	if dup.json(t)["error"] != "probe_in_progress" {
		t.Fatalf("重复触发错误码错误：%s", dup.body)
	}

	close(block)
	var runBody map[string]any
	for i := 0; i < 60; i++ {
		r := do(t, ts, "GET", "/api/admin/network/probes/"+id, "", ck, nil)
		if r.code != 200 {
			t.Fatalf("读取执行记录失败：%d body=%s", r.code, r.body)
		}
		runBody = r.json(t)
		if runBody["status"] == "completed" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if runBody == nil || runBody["status"] != "completed" {
		t.Fatalf("探测未完成：%v", runBody)
	}
	results := runBody["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("单次执行应返回三个结果：%v", results)
	}
	if stub.callCount() != 3 {
		t.Fatalf("触发内容不能改变目标，got calls=%d", stub.callCount())
	}

	public := do(t, ts, "GET", "/api/network/status", "", "", nil)
	if public.code != 200 {
		t.Fatalf("公开状态读取失败：%d", public.code)
	}
	publicBody := string(public.body)
	for _, secret := range []string{"10.20.0.", "i/o timeout", "dial tcp", "address", "failure_reason"} {
		if strings.Contains(publicBody, secret) {
			t.Fatalf("公开状态泄露 %q：%s", secret, publicBody)
		}
	}
	if !strings.Contains(publicBody, `"status":"success"`) || !strings.Contains(publicBody, `"status":"timeout"`) {
		t.Fatalf("公开状态缺少预期状态：%s", publicBody)
	}

	admin := do(t, ts, "GET", "/api/admin/network/results", "", ck, nil)
	if admin.code != 200 {
		t.Fatalf("管理员读取完整结果失败：%d", admin.code)
	}
	adminBody := string(admin.body)
	for _, want := range []string{"10.20.0.1", "10.20.0.2", "10.20.0.3", "dial tcp", "HTTP 状态码 500"} {
		if !strings.Contains(adminBody, want) {
			t.Fatalf("管理员完整结果缺少 %q：%s", want, adminBody)
		}
	}

	if r := do(t, ts, "GET", "/api/admin/network/probes/not-found", "", ck, nil); r.code != http.StatusNotFound {
		t.Fatalf("未知执行记录应 404，got %d", r.code)
	}
}

type probeStub struct {
	mu      sync.Mutex
	calls   []network.Target
	block   <-chan struct{}
	results map[network.TargetKey]probe.Result
}

func (p *probeStub) Probe(ctx context.Context, target network.Target) probe.Result {
	p.mu.Lock()
	p.calls = append(p.calls, target)
	p.mu.Unlock()
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return probe.Result{Key: target.Key, Label: target.Label, Address: target.Address, Enabled: true,
				Status: probe.StatusTimeout, FailureInfo: "context timeout", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		}
	}
	result, ok := p.results[target.Key]
	if !ok {
		result = probe.Result{Status: probe.StatusSuccess, HTTPStatus: 200, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	result.Key = target.Key
	result.Label = target.Label
	result.Address = target.Address
	result.Enabled = target.Enabled
	return result
}

func (p *probeStub) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func TestPublicNetworkProbeTargetsMinimal(t *testing.T) {
	s, ts := newTestServer(t, nil)

	first := do(t, ts, "GET", "/api/network/targets", "", "", nil)
	if first.code != http.StatusOK {
		t.Fatalf("访客应能读取最小启用目标：%d body=%s", first.code, first.body)
	}
	body := string(first.body)
	if !strings.Contains(body, `"address":"router.local"`) {
		t.Fatalf("缺少启用的主路由目标：%s", body)
	}
	for _, leaked := range []string{"bypass_router", `"nas"`, "主路由器", "enabled", "updated_at", "failure_reason"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("最小目标接口泄露 %q：%s", leaked, body)
		}
	}
	if r := do(t, ts, "POST", "/api/network/targets", `{"targets":[]}`, "", nil); r.code != http.StatusNotFound && r.code != http.StatusMethodNotAllowed {
		t.Fatalf("公开目标接口不能写入，got %d body=%s", r.code, r.body)
	}

	ck := adminCookie(t, ts, s)
	cfg := `{"targets":[
		{"key":"router","address":"10.30.0.1","enabled":true},
		{"key":"bypass_router","address":"10.30.0.2","enabled":true},
		{"key":"nas","address":"10.30.0.3","enabled":false}]}`
	if r := do(t, ts, "PUT", "/api/admin/network/targets", cfg, ck, nil); r.code != http.StatusOK {
		t.Fatalf("保存目标失败：%d body=%s", r.code, r.body)
	}
	second := do(t, ts, "GET", "/api/network/targets", "", "", nil)
	if second.code != http.StatusOK {
		t.Fatalf("读取最小目标失败：%d", second.code)
	}
	for _, want := range []string{`"key":"router"`, `"address":"10.30.0.1"`, `"key":"bypass_router"`, `"address":"10.30.0.2"`} {
		if !strings.Contains(string(second.body), want) {
			t.Fatalf("缺少启用目标 %s：%s", want, second.body)
		}
	}
	if strings.Contains(string(second.body), "10.30.0.3") || strings.Contains(string(second.body), `"enabled"`) {
		t.Fatalf("禁用目标不应出现在最小配置：%s", second.body)
	}
}
