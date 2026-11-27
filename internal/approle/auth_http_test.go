package approle

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/auth"
	"github.com/laoXuHome/LocalHub/internal/config"
	"github.com/laoXuHome/LocalHub/internal/db"
)

func testConfig(t *testing.T, mutate func(*config.Config)) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		HTTPAddr:           "127.0.0.1",
		HTTPPort:           0,
		DataDir:            dir,
		AllowedRoot:        dir,
		DBPath:             filepath.Join(dir, "test.db"),
		LogLevel:           "error",
		TZ:                 "Asia/Shanghai",
		PUID:               1000,
		PGID:               1000,
		WriteCheckFatal:    false,
		WriteCheckInterval: time.Hour,
		AdminUsername:      "admin",
		SessionTTL:         12 * time.Hour,
		LoginMaxFailures:   5,
		LoginWindow:        15 * time.Minute,
		LoginLockout:       15 * time.Minute,
	}
	if mutate != nil {
		mutate(cfg)
	}
	return cfg
}

func newTestServer(t *testing.T, mutate func(*config.Config)) (*server, *httptest.Server) {
	t.Helper()
	cfg := testConfig(t, mutate)
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &server{cfg: cfg, log: log, db: database, state: &writeState{}, start: time.Now(), root: cfg.DataDir}
	s.auth = newAuthState(s)
	ts := httptest.NewServer(s.logMiddleware(s.routes()))
	t.Cleanup(ts.Close)
	return s, ts
}

func bootstrap(t *testing.T, s *server, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("生成口令哈希失败：%v", err)
	}
	if err := s.auth.store.CreateAdmin(context.Background(), s.cfg.AdminUsername, hash, time.Now()); err != nil {
		t.Fatalf("创建管理员失败：%v", err)
	}
}

type response struct {
	code   int
	body   []byte
	cookie string
	header http.Header
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("响应不是 JSON：%v，body=%s", err, r.body)
	}
	return m
}

func do(t *testing.T, ts *httptest.Server, method, path, body, cookie string, headers map[string]string) response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, rd)
	if err != nil {
		t.Fatalf("构造请求失败：%v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", ts.URL)
	if cookie != "" {
		req.Header.Set("Cookie", sessionCookie+"="+cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
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

func TestPublicRoutesAccessibleToVisitor(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	if r := do(t, ts, "GET", "/api/auth/status", "", "", nil); r.code != 200 {
		t.Fatalf("/api/auth/status 应公开可访问，got %d", r.code)
	} else if m := r.json(t); m["authenticated"] != false || m["role"] != "guest" {
		t.Fatalf("访客状态不正确：%v", m)
	}
	if r := do(t, ts, "GET", "/", "", "", nil); r.code != 200 {
		t.Fatalf("公开首页应可访问，got %d", r.code)
	}
}

func TestAdminAPIRejectsVisitor(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/admin/whoami", ""},
		{"POST", "/api/auth/logout", ""},
		{"POST", "/api/auth/password", `{"current_password":"x","new_password":"y"}`},
	} {
		r := do(t, ts, tc.method, tc.path, tc.body, "", nil)
		if r.code != http.StatusUnauthorized {
			t.Fatalf("%s %s 应被后端拒绝为 401，got %d（body=%s）", tc.method, tc.path, r.code, r.body)
		}
	}
}

func TestLoginLogoutFlow(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	if r := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"wrong-password-123"}`, "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401，got %d", r.code)
	} else if r.cookie != "" {
		t.Fatalf("登录失败不应下发会话 Cookie")
	}

	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 || login.cookie == "" {
		t.Fatalf("正确密码应登录成功并下发 Cookie，got %d cookie=%q", login.code, login.cookie)
	}
	if m := login.json(t); m["role"] != "admin" {
		t.Fatalf("登录响应角色错误：%v", m)
	}

	if r := do(t, ts, "GET", "/api/admin/whoami", "", login.cookie, nil); r.code != 200 {
		t.Fatalf("管理员应可访问 whoami，got %d", r.code)
	} else if m := r.json(t); m["username"] != "admin" {
		t.Fatalf("whoami 返回用户名错误：%v", m)
	}

	if r := do(t, ts, "POST", "/api/auth/logout", "", login.cookie, nil); r.code != 200 {
		t.Fatalf("退出登录失败：%d", r.code)
	}
	if r := do(t, ts, "GET", "/api/admin/whoami", "", login.cookie, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("退出后会话必须失效，got %d", r.code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	login := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if login.code != 200 {
		t.Fatalf("登录失败：%d", login.code)
	}
	// 直接把会话过期时间改到过去，模拟到期。
	if _, err := s.db.SQL.Exec(`UPDATE sessions SET expires_at = ?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("制造过期会话失败：%v", err)
	}
	if r := do(t, ts, "GET", "/api/admin/whoami", "", login.cookie, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("过期会话必须被拒绝，got %d", r.code)
	}
}

func TestLoginLockout(t *testing.T) {
	s, ts := newTestServer(t, func(c *config.Config) {
		c.LoginMaxFailures = 3
		c.LoginWindow = time.Minute
		c.LoginLockout = 5 * time.Minute
	})
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	for i := 0; i < 3; i++ {
		if r := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"bad-password-123"}`, "", nil); r.code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次失败应为 401，got %d", i+1, r.code)
		}
	}
	r := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"bad-password-123"}`, "", nil)
	if r.code != http.StatusTooManyRequests {
		t.Fatalf("超过阈值应被限速 429，got %d", r.code)
	}
	if r.header.Get("Retry-After") == "" {
		t.Fatalf("限速响应应包含 Retry-After")
	}
	// 锁定期间即使口令正确也必须拒绝。
	r = do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if r.code != http.StatusTooManyRequests {
		t.Fatalf("锁定期间正确口令也应 429，got %d", r.code)
	}
}

func TestChangePasswordRevokesOthers(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")

	a := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	b := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil)
	if a.code != 200 || b.code != 200 {
		t.Fatalf("两次登录应成功：%d/%d", a.code, b.code)
	}

	if r := do(t, ts, "POST", "/api/auth/password", `{"current_password":"wrong","new_password":"N3w-Passw0rd!2026"}`, a.cookie, nil); r.code != http.StatusForbidden {
		t.Fatalf("旧口令错误应 403，got %d", r.code)
	}
	r := do(t, ts, "POST", "/api/auth/password", `{"current_password":"Adm1nPassw0rd!2026","new_password":"N3w-Passw0rd!2026"}`, a.cookie, nil)
	if r.code != 200 {
		t.Fatalf("改密应成功，got %d body=%s", r.code, r.body)
	}
	// 当前会话保留，其他会话被吊销。
	if rr := do(t, ts, "GET", "/api/admin/whoami", "", a.cookie, nil); rr.code != 200 {
		t.Fatalf("改密后当前会话应继续有效，got %d", rr.code)
	}
	if rr := do(t, ts, "GET", "/api/admin/whoami", "", b.cookie, nil); rr.code != http.StatusUnauthorized {
		t.Fatalf("改密后其他会话应失效，got %d", rr.code)
	}
	// 新口令可登录，旧口令不可。
	if rr := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil); rr.code != http.StatusUnauthorized {
		t.Fatalf("旧口令应无法登录，got %d", rr.code)
	}
	if rr := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"N3w-Passw0rd!2026"}`, "", nil); rr.code != 200 {
		t.Fatalf("新口令应可登录，got %d", rr.code)
	}
}

func TestPasswordNotStoredInPlaintext(t *testing.T) {
	s, _ := newTestServer(t, nil)
	plain := "Adm1nPassw0rd!2026"
	bootstrap(t, s, plain)

	var hash string
	if err := s.db.SQL.QueryRow(`SELECT password_hash FROM admin_users LIMIT 1`).Scan(&hash); err != nil {
		t.Fatalf("读取口令哈希失败：%v", err)
	}
	if strings.Contains(hash, plain) {
		t.Fatalf("数据库中出现了明文口令")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("口令不是 bcrypt 哈希：%s", hash)
	}
	if !auth.ComparePassword(hash, plain) {
		t.Fatalf("哈希无法校验原口令")
	}
}

func TestNeedsSetupWhenNoAdmin(t *testing.T) {
	_, ts := newTestServer(t, nil)
	r := do(t, ts, "GET", "/api/auth/status", "", "", nil)
	if m := r.json(t); m["needs_setup"] != true {
		t.Fatalf("未初始化时应提示 needs_setup=true，got %v", m)
	}
	if rr := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", nil); rr.code != http.StatusServiceUnavailable {
		t.Fatalf("未初始化时登录应 503，got %d", rr.code)
	}
}

func TestCrossOriginWriteRejected(t *testing.T) {
	s, ts := newTestServer(t, nil)
	bootstrap(t, s, "Adm1nPassw0rd!2026")
	r := do(t, ts, "POST", "/api/auth/login", `{"username":"admin","password":"Adm1nPassw0rd!2026"}`, "", map[string]string{"Origin": "http://evil.example"})
	if r.code != http.StatusForbidden {
		t.Fatalf("跨站写请求应被拒绝为 403，got %d", r.code)
	}
}
