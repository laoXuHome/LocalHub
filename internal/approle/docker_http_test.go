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

func TestDockerBookmarkLinksAdminMissingAndOutage(t *testing.T) {
	idOK := strings.Repeat("a", 64)
	idMissing := strings.Repeat("b", 64)
	idSecond := strings.Repeat("c", 64)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/containers/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"Id":"` + idOK + `","Names":["/web"],"Image":"nginx:latest","Created":1700000000,"State":"running","Status":"Up 1 hour","Ports":[],"Labels":{}},{"Id":"` + idSecond + `","Names":["/cache"],"Image":"redis:7","Created":1700000001,"State":"exited","Status":"Exited","Ports":[],"Labels":{}}]`))
		case strings.HasPrefix(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			_, _ = w.Write([]byte(`{"State":{"StartedAt":"2026-11-28T06:00:00Z"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	s, ts := newTestServer(t, nil)
	s.docker, _ = dockerclient.New(fixture.URL, 2*time.Second)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if r := do(t, ts, "GET", "/api/admin/docker/bookmark-links", "", "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("访客读取关联应 401，got %d", r.code)
	}
	body := `{"name":"Web","wan_url":"https://web.example.com","docker_container":"` + idOK + `"}`
	if r := do(t, ts, "POST", "/api/admin/bookmarks", body, login.cookie, nil); r.code != http.StatusCreated {
		t.Fatalf("关联书签创建失败：%d %s", r.code, r.body)
	}
	body = `{"name":"Cache","wan_url":"https://cache.example.com","docker_container":"` + idSecond + `"}`
	if r := do(t, ts, "POST", "/api/admin/bookmarks", body, login.cookie, nil); r.code != http.StatusCreated {
		t.Fatalf("第二关联书签创建失败：%d", r.code)
	}
	body = `{"name":"Old","wan_url":"https://old.example.com","docker_container":"` + idMissing + `"}`
	if r := do(t, ts, "POST", "/api/admin/bookmarks", body, login.cookie, nil); r.code != http.StatusCreated {
		t.Fatalf("历史关联书签创建失败：%d", r.code)
	}
	r := do(t, ts, "GET", "/api/admin/docker/bookmark-links", "", login.cookie, nil)
	if r.code != 200 {
		t.Fatalf("关联读取应 200，got %d", r.code)
	}
	m := r.json(t)
	if m["status"] != "connected" {
		t.Fatalf("状态错误：%v", m)
	}
	containers := m["containers"].([]any)
	if len(containers) != 2 {
		t.Fatalf("容器数错误：%v", containers)
	}
	var webLinks, cacheLinks []any
	for _, raw := range containers {
		c := raw.(map[string]any)
		switch c["name"] {
		case "web":
			webLinks = c["bookmarks"].([]any)
		case "cache":
			cacheLinks = c["bookmarks"].([]any)
		}
	}
	if len(webLinks) != 1 || webLinks[0].(map[string]any)["name"] != "Web" {
		t.Fatalf("第一个容器关联错误：%v", webLinks)
	}
	if len(cacheLinks) != 1 || cacheLinks[0].(map[string]any)["name"] != "Cache" {
		t.Fatalf("第二个容器关联错误：%v", cacheLinks)
	}
	missing := m["missing"].([]any)
	if len(missing) != 1 || missing[0].(map[string]any)["bookmark_name"] != "Old" {
		t.Fatalf("缺失关联错误：%v", missing)
	}

	fixture.Close()
	r = do(t, ts, "GET", "/api/admin/docker/bookmark-links", "", login.cookie, nil)
	m = r.json(t)
	if m["status"] == "connected" {
		t.Fatalf("故障后不应仍为 connected：%v", m)
	}
	if len(m["missing"].([]any)) != 0 {
		t.Fatalf("Docker 故障时不得判定容器不存在：%v", m["missing"])
	}
}
