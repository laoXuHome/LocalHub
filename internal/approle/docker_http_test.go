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
