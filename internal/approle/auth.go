package approle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/auth"
)

const (
	sessionCookie = "localhub_session"
	maxBodyBytes  = 8 << 10
)

// 用于在“用户不存在”时也执行一次 bcrypt 比较，避免时间差泄露账户是否存在。
var dummyHash = "$2a$12$C6UzMDM.H6dfI/f/IKcEe.uN0vJ0m9L6e.7dQpPNrTPn4P4Vq5FyW"

type authState struct {
	store    *auth.Store
	sessions *auth.Manager
	limiter  *auth.Limiter
}

func newAuthState(s *server) *authState {
	database := s.db
	return &authState{
		store:    &auth.Store{DB: database},
		sessions: &auth.Manager{DB: database, TTL: s.cfg.SessionTTL},
		limiter: &auth.Limiter{
			DB:          database,
			MaxFailures: s.cfg.LoginMaxFailures,
			Window:      s.cfg.LoginWindow,
			Lockout:     s.cfg.LoginLockout,
		},
	}
}

type sessionCtxKey struct{}

func sessionFrom(ctx context.Context) *auth.Session {
	s, _ := ctx.Value(sessionCtxKey{}).(*auth.Session)
	return s
}

func (s *server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"error": errCode, "message": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "只支持 POST")
		return false
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "需要 application/json")
		return false
	}
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_body", "请求体不是合法 JSON")
		return false
	}
	return true
}

// sameOriginOK 对写操作做来源校验，缓解 CSRF（配合 SameSite=Lax）。
func sameOriginOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 非浏览器客户端（curl、内部调用）没有 Origin
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// requireAdmin 仅允许持有有效管理员会话的请求通过；权限判断在后端完成。
func (s *server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOriginOK(r) {
			writeError(w, http.StatusForbidden, "csrf_rejected", "请求来源与站点不一致")
			return
		}
		if s.auth == nil {
			writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "认证子系统不可用")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "需要管理员登录")
			return
		}
		sess, err := s.auth.sessions.Get(r.Context(), c.Value, time.Now())
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "会话无效或已过期")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionCtxKey{}, sess)))
	})
}

func (s *server) currentSession(r *http.Request) (*auth.Session, bool) {
	if s.auth == nil {
		return nil, false
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, false
	}
	sess, err := s.auth.sessions.Get(r.Context(), c.Value, time.Now())
	if err != nil {
		return nil, false
	}
	return sess, true
}

// handleAuthStatus 公开接口：告知前端当前身份、是否需要初始化。
func (s *server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "只支持 GET")
		return
	}
	resp := map[string]any{
		"authenticated":       false,
		"role":                "guest",
		"needs_setup":         true,
		"session_ttl_seconds": int(s.cfg.SessionTTL.Seconds()),
	}
	if s.db == nil || s.auth == nil {
		resp["error"] = "database_unavailable"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	n, err := s.auth.store.CountAdmins(r.Context())
	if err != nil {
		resp["error"] = "database_unavailable"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["needs_setup"] = n == 0
	if sess, ok := s.currentSession(r); ok {
		resp["authenticated"] = true
		resp["role"] = "admin"
		resp["username"] = sess.Username
		resp["session_expires_at"] = sess.ExpiresAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLogin 公开接口：登录并下发 HttpOnly 会话 Cookie。
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "认证子系统不可用")
		return
	}
	if !sameOriginOK(r) {
		writeError(w, http.StatusForbidden, "csrf_rejected", "请求来源与站点不一致")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	username, err := auth.NormalizeUsername(req.Username)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	ip := s.clientIP(r)
	now := time.Now()

	admins, err := s.auth.store.CountAdmins(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	if admins == 0 {
		writeError(w, http.StatusServiceUnavailable, "not_initialized", "管理员尚未初始化：请设置 LOCALHUB_ADMIN_PASSWORD 后重启容器")
		return
	}

	locked, retry, err := s.auth.limiter.Locked(r.Context(), ip, username, now)
	if err != nil {
		s.log.Error("登录限速检查失败", slog.String("error", err.Error()))
		writeError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "登录限速不可用")
		return
	}
	if locked {
		secs := int(retry.Seconds()) + 1
		w.Header().Set("Retry-After", itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":               "too_many_attempts",
			"message":             "登录失败次数过多，请稍后再试",
			"retry_after_seconds": secs,
		})
		s.log.Warn("登录被限速", slog.String("ip", ip), slog.String("username", username), slog.Int("retry_after_seconds", secs))
		return
	}

	admin, gerr := s.auth.store.GetByUsername(r.Context(), username)
	ok := false
	if gerr == nil {
		ok = auth.ComparePassword(admin.PasswordHash, req.Password)
	} else {
		// 用户不存在也做一次等时比较，避免账户枚举。
		auth.ComparePassword(dummyHash, req.Password)
	}

	if !ok {
		_ = s.auth.limiter.Record(r.Context(), ip, username, false, now)
		s.log.Warn("登录失败", slog.String("ip", ip), slog.String("username", username), slog.String("code", "INVALID_CREDENTIALS"))
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}

	_ = s.auth.limiter.Record(r.Context(), ip, username, true, now)
	_ = s.auth.limiter.Clear(r.Context(), ip, username)

	token, sess, err := s.auth.sessions.Create(r.Context(), admin.ID, ip, r.UserAgent(), now)
	if err != nil {
		s.log.Error("创建会话失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "session_create_failed", "无法创建会话")
		return
	}
	s.setSessionCookie(w, token, sess.ExpiresAt)
	s.log.Info("管理员登录成功", slog.String("username", admin.Username), slog.String("ip", ip), slog.String("session_expires_at", sess.ExpiresAt.Format(time.RFC3339)))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"username":           admin.Username,
		"role":               "admin",
		"session_expires_at": sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func (s *server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// handleLogout 管理员接口：吊销当前会话。
func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !sameOriginOK(r) {
		writeError(w, http.StatusForbidden, "csrf_rejected", "请求来源与站点不一致")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "只支持 POST")
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.auth.sessions.Revoke(r.Context(), c.Value, time.Now())
	}
	s.clearSessionCookie(w)
	if sess := sessionFrom(r.Context()); sess != nil {
		s.log.Info("管理员退出登录", slog.String("username", sess.Username), slog.String("ip", s.clientIP(r)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleChangePassword 管理员接口：校验旧口令后更新哈希，并吊销其他会话。
func (s *server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "需要管理员登录")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	admin, err := s.auth.store.GetByID(r.Context(), sess.AdminID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "账户不存在")
		return
	}
	if !auth.ComparePassword(admin.PasswordHash, req.CurrentPassword) {
		s.log.Warn("修改口令失败：原口令错误", slog.String("username", admin.Username), slog.String("ip", s.clientIP(r)))
		writeError(w, http.StatusForbidden, "invalid_current_password", "当前密码不正确")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	if auth.ComparePassword(admin.PasswordHash, req.NewPassword) {
		writeError(w, http.StatusBadRequest, "same_password", "新密码不能与当前密码相同")
		return
	}
	now := time.Now()
	if err := s.auth.store.UpdatePassword(r.Context(), admin.ID, hash, now); err != nil {
		s.log.Error("更新口令失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "password_update_failed", "更新密码失败")
		return
	}
	var keep string
	if c, err := r.Cookie(sessionCookie); err == nil {
		keep = c.Value
	}
	n, _ := s.auth.sessions.RevokeOthers(r.Context(), admin.ID, keep, now)
	s.log.Info("管理员修改口令", slog.String("username", admin.Username), slog.String("ip", s.clientIP(r)), slog.Int64("revoked_other_sessions", n))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked_other_sessions": n})
}

// handleAdminWhoami 管理员专属接口，用于验证后端权限边界。
func (s *server) handleAdminWhoami(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/admin/whoami" {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "只支持 GET")
		return
	}
	sess := sessionFrom(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "需要管理员登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                  sess.AdminID,
		"username":            sess.Username,
		"role":                "admin",
		"session_created_at":  sess.CreatedAt.UTC().Format(time.RFC3339),
		"session_expires_at":  sess.ExpiresAt.UTC().Format(time.RFC3339),
		"session_ttl_seconds": int(s.cfg.SessionTTL.Seconds()),
	})
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	out := make([]byte, 0, 12)
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}
