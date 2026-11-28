package approle

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/config"
	"github.com/laoXuHome/LocalHub/internal/dockerclient"
)

func TestDockerStatusRequiresAdminAndRedacts(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			t.Errorf("P5.1 只应请求 /version，实际 %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ApiVersion":"1.52","Version":"29.1.3"}`))
	}))
	defer fixture.Close()

	s, ts := newTestServer(t, func(c *config.Config) {
		c.DockerAPIURL = fixture.URL
		c.DockerTimeout = time.Second
	})
	s.docker, _ = dockerclient.New(fixture.URL, time.Second)
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	if r := do(t, ts, "GET", "/api/admin/docker/status", "", "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("访客访问 Docker 状态应 401，got %d", r.code)
	}

	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 {
		t.Fatalf("管理员登录失败：%d", login.code)
	}
	r := do(t, ts, "GET", "/api/admin/docker/status", "", login.cookie, nil)
	if r.code != 200 {
		t.Fatalf("管理员读取 Docker 状态应 200，got %d", r.code)
	}
	m := r.json(t)
	if m["status"] != "connected" || m["api_version"] != "1.52" || m["read_only"] != true {
		t.Fatalf("Docker 状态响应错误：%v", m)
	}
	body := string(r.body)
	for _, forbidden := range []string{fixture.URL, "127.0.0.1", "/var/run/docker.sock", "error"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Docker 状态响应不得包含 %q：%s", forbidden, body)
		}
	}
}

func TestDockerStatusPermissionDenied(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer fixture.Close()

	s, ts := newTestServer(t, nil)
	s.docker, _ = dockerclient.New(fixture.URL, time.Second)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	r := do(t, ts, "GET", "/api/admin/docker/status", "", login.cookie, nil)
	if r.code != 200 {
		t.Fatalf("权限不足应以 200 返回状态对象，got %d", r.code)
	}
	m := r.json(t)
	if m["status"] != "permission_denied" {
		t.Fatalf("状态期望 permission_denied，得到 %v", m)
	}
	if strings.Contains(string(r.body), "forbidden") {
		t.Fatalf("不得把上游响应体返回给前端：%s", r.body)
	}
}

func TestDockerContainersRequiresAdminAndReturnsRealFixture(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/containers/json":
			if r.URL.Query().Get("all") != "1" {
				t.Errorf("应请求 all=1，实际 %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"Id":"run123","Names":["/web"],"Image":"nginx:latest","Created":1700000000,"State":"running","Status":"Up 1 hour","Ports":[{"PrivatePort":80,"PublicPort":8080,"Type":"tcp","IP":"0.0.0.0"}],"Labels":{"com.example.role":"web"}},
				{"Id":"stop456","Names":["/job"],"Image":"busybox:latest","Created":1700000001,"State":"exited","Status":"Exited (0)","Ports":[],"Labels":{}},
				{"Id":"bad","Names":["/bad"],"Image":"x","Created":0,"State":"","Status":"","Ports":[],"Labels":{}}
			]`))
		case strings.HasPrefix(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"State":{"StartedAt":"2026-11-28T06:00:00Z"}}`))
		default:
			t.Errorf("不应请求其他 Docker 路径：%s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()

	s, ts := newTestServer(t, func(c *config.Config) {
		c.DockerRefreshInterval = 45 * time.Second
	})
	s.docker, _ = dockerclient.New(fixture.URL, 2*time.Second)
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	if r := do(t, ts, "GET", "/api/admin/docker/containers", "", "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("访客访问容器列表应 401，got %d", r.code)
	}
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	r := do(t, ts, "GET", "/api/admin/docker/containers", "", login.cookie, nil)
	if r.code != 200 {
		t.Fatalf("管理员容器列表应 200，got %d", r.code)
	}
	m := r.json(t)
	if m["status"] != "connected" || m["read_only"] != true || m["refresh_interval_seconds"] != float64(45) {
		t.Fatalf("容器列表响应错误：%v", m)
	}
	containers, ok := m["containers"].([]any)
	if !ok || len(containers) != 3 {
		t.Fatalf("应有 3 个容器：%v", m["containers"])
	}
	first := containers[0].(map[string]any)
	if first["name"] != "web" || first["status"] != "running" || first["image"] != "nginx:latest" {
		t.Fatalf("容器字段错误：%v", first)
	}
	counts := m["counts"].(map[string]any)
	if counts["total"] != float64(3) || counts["running"] != float64(1) || counts["stopped"] != float64(1) || counts["unknown"] != float64(1) {
		t.Fatalf("容器统计错误：%v", counts)
	}
	body := string(r.body)
	for _, forbidden := range []string{fixture.URL, "127.0.0.1", "/var/run/docker.sock", "StartedAt"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("容器列表不得包含内部字段 %q：%s", forbidden, body)
		}
	}
}
