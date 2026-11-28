package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config 是 P0 阶段的全部可配置项。所有默认值都在这里定义，代码中不得写死宿主机路径。
type Config struct {
	HTTPAddr string
	HTTPPort int

	// DataDir 是容器内持久化根目录，固定约定为 /data。
	DataDir string
	// AllowedRoot 是允许访问的根，默认与 DataDir 相同；用于拒绝符号链接逃逸。
	AllowedRoot string
	DBPath      string

	LogLevel string
	TZ       string

	PUID           int
	PGID           int
	AllowRoot      bool
	ChownRecursive bool

	WriteCheckFatal    bool
	WriteCheckInterval time.Duration

	// 账户与会话（P1.1）。
	AdminUsername string // 管理员用户名，默认 admin
	AdminPassword string // 仅在管理员尚未初始化时用于首次初始化，不会覆盖已有账户
	SessionTTL    time.Duration
	CookieSecure  bool
	TrustProxy    bool

	LoginMaxFailures int
	LoginWindow      time.Duration
	LoginLockout     time.Duration

	// P5.1 Docker 只读连接。DockerAPIURL 为空表示未配置，不应阻止启动。
	DockerAPIURL  string
	DockerTimeout time.Duration
}

// Load 从进程环境读取配置。
func Load() (*Config, error) {
	return LoadFrom(os.Environ())
}

// LoadFrom 便于测试：传入 KEY=VALUE 列表。
func LoadFrom(env []string) (*Config, error) {
	m := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	get := func(key, def string) string {
		if v, ok := m[key]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}

	c := &Config{
		HTTPAddr:           get("LOCALHUB_HTTP_ADDR", "0.0.0.0"),
		DataDir:            get("LOCALHUB_DATA_DIR", "/data"),
		LogLevel:           strings.ToLower(get("LOCALHUB_LOG_LEVEL", "info")),
		TZ:                 get("TZ", "Asia/Shanghai"),
		WriteCheckInterval: 60 * time.Second,
		DockerTimeout:      3 * time.Second,
	}

	var problems []string
	bad := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if v := get("LOCALHUB_HTTP_PORT", "11030"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			bad("LOCALHUB_HTTP_PORT=%q 非法，必须是 1-65535 的整数", v)
		} else {
			c.HTTPPort = n
		}
	}

	if !filepath.IsAbs(c.DataDir) {
		bad("LOCALHUB_DATA_DIR=%q 必须是绝对路径", c.DataDir)
	}
	c.DataDir = filepath.Clean(c.DataDir)

	c.AllowedRoot = get("LOCALHUB_DATA_ALLOWED_ROOT", c.DataDir)
	if !filepath.IsAbs(c.AllowedRoot) {
		bad("LOCALHUB_DATA_ALLOWED_ROOT=%q 必须是绝对路径", c.AllowedRoot)
	}
	c.AllowedRoot = filepath.Clean(c.AllowedRoot)

	c.DBPath = get("LOCALHUB_DB_PATH", filepath.Join(c.DataDir, "localhub.db"))
	if !filepath.IsAbs(c.DBPath) {
		bad("LOCALHUB_DB_PATH=%q 必须是绝对路径", c.DBPath)
	}
	c.DBPath = filepath.Clean(c.DBPath)
	if rel, err := filepath.Rel(c.DataDir, c.DBPath); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		bad("LOCALHUB_DB_PATH=%q 必须位于数据目录 %q 之内", c.DBPath, c.DataDir)
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		bad("LOCALHUB_LOG_LEVEL=%q 非法，可选 debug/info/warn/error", c.LogLevel)
	}

	c.PUID = 1000
	if v, ok := m["PUID"]; ok && strings.TrimSpace(v) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 || n > 65535 {
			bad("PUID=%q 非法，必须是 0-65535 的整数", v)
		} else {
			c.PUID = n
		}
	}
	c.PGID = 1000
	if v, ok := m["PGID"]; ok && strings.TrimSpace(v) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 || n > 65535 {
			bad("PGID=%q 非法，必须是 0-65535 的整数", v)
		} else {
			c.PGID = n
		}
	}

	parseBool := func(key string, def bool) (bool, error) {
		v, ok := m[key]
		if !ok || strings.TrimSpace(v) == "" {
			return def, nil
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true, nil
		case "0", "false", "no", "off":
			return false, nil
		}
		return def, fmt.Errorf("%s=%q 非法，可选 1/0 true/false", key, v)
	}

	var err error
	if c.AllowRoot, err = parseBool("ALLOW_ROOT", false); err != nil {
		problems = append(problems, err.Error())
	}
	if c.ChownRecursive, err = parseBool("CHOWN_RECURSIVE", false); err != nil {
		problems = append(problems, err.Error())
	}
	if c.WriteCheckFatal, err = parseBool("WRITE_CHECK_FATAL", true); err != nil {
		problems = append(problems, err.Error())
	}
	if c.CookieSecure, err = parseBool("LOCALHUB_COOKIE_SECURE", false); err != nil {
		problems = append(problems, err.Error())
	}
	if c.TrustProxy, err = parseBool("LOCALHUB_TRUST_PROXY", false); err != nil {
		problems = append(problems, err.Error())
	}

	// —— 账户与会话（P1.1）——
	c.AdminUsername = get("LOCALHUB_ADMIN_USERNAME", "admin")
	uname, err := authNameOK(c.AdminUsername)
	if err != nil {
		bad("LOCALHUB_ADMIN_USERNAME=%q 非法：%v", c.AdminUsername, err)
	}
	c.AdminUsername = uname

	c.AdminPassword = m["LOCALHUB_ADMIN_PASSWORD"]
	if c.AdminPassword != "" {
		if err := authPasswordOK(c.AdminPassword); err != nil {
			bad("LOCALHUB_ADMIN_PASSWORD 非法：%v", err)
		}
	}

	c.SessionTTL = 12 * time.Hour
	if v, ok := m["LOCALHUB_SESSION_TTL"]; ok && strings.TrimSpace(v) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil || d < 5*time.Minute || d > 30*24*time.Hour {
			bad("LOCALHUB_SESSION_TTL=%q 非法，需为 5m-720h 的时长（如 12h）", v)
		} else {
			c.SessionTTL = d
		}
	}

	c.LoginMaxFailures = 5
	if v, ok := m["LOCALHUB_LOGIN_MAX_FAILURES"]; ok && strings.TrimSpace(v) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 || n > 100 {
			bad("LOCALHUB_LOGIN_MAX_FAILURES=%q 非法，需为 1-100 的整数", v)
		} else {
			c.LoginMaxFailures = n
		}
	}
	c.LoginWindow = 15 * time.Minute
	if v, ok := m["LOCALHUB_LOGIN_WINDOW"]; ok && strings.TrimSpace(v) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil || d < time.Minute || d > 24*time.Hour {
			bad("LOCALHUB_LOGIN_WINDOW=%q 非法，需为 1m-24h 的时长", v)
		} else {
			c.LoginWindow = d
		}
	}
	c.LoginLockout = 15 * time.Minute
	if v, ok := m["LOCALHUB_LOGIN_LOCKOUT"]; ok && strings.TrimSpace(v) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil || d < time.Minute || d > 24*time.Hour {
			bad("LOCALHUB_LOGIN_LOCKOUT=%q 非法，需为 1m-24h 的时长", v)
		} else {
			c.LoginLockout = d
		}
	}

	// —— P5.1 Docker 只读连接 ——
	// 只接受 HTTP(S) 或 Docker 惯例的 tcp://；unix:// 被拒绝，防止绕过 Socket Proxy。
	if v := get("LOCALHUB_DOCKER_API_URL", ""); v != "" {
		u, err := normalizeDockerAPIURL(v)
		if err != nil {
			bad("LOCALHUB_DOCKER_API_URL=%q 非法：%v", v, err)
		} else {
			c.DockerAPIURL = u
		}
	}
	if v, ok := m["LOCALHUB_DOCKER_TIMEOUT"]; ok && strings.TrimSpace(v) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil || d < 500*time.Millisecond || d > 30*time.Second {
			bad("LOCALHUB_DOCKER_TIMEOUT=%q 非法，需为 500ms-30s 的时长（如 3s）", v)
		} else {
			c.DockerTimeout = d
		}
	}

	if c.PUID == 0 && !c.AllowRoot {
		bad("PUID=0 被拒绝：业务进程不得以 root 运行；如确需如此，必须同时设置 ALLOW_ROOT=1")
	}
	if c.PGID == 0 && !c.AllowRoot {
		bad("PGID=0 被拒绝：业务进程不得以 root 组运行；如确需如此，必须同时设置 ALLOW_ROOT=1")
	}
	if v, ok := m["WRITE_CHECK_INTERVAL"]; ok && strings.TrimSpace(v) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil || d < time.Second {
			bad("WRITE_CHECK_INTERVAL=%q 非法，必须是 >=1s 的时长（如 60s）", v)
		} else {
			c.WriteCheckInterval = d
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("配置校验失败：%s", strings.Join(problems, "；"))
	}
	return c, nil
}

// normalizeDockerAPIURL 校验并归一化 Docker 连接地址。
// 该函数不允许 unix://、用户信息、路径、查询参数或片段。
func normalizeDockerAPIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "unix://") {
		return "", fmt.Errorf("不允许 unix://，必须通过 Docker Socket Proxy")
	}
	if strings.HasPrefix(lower, "tcp://") {
		raw = "http://" + raw[len("tcp://"):]
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("地址无法解析")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("只支持 http://、https:// 或 tcp://")
	}
	if u.User != nil {
		return "", fmt.Errorf("不得包含用户名或口令")
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("缺少主机")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("不得包含路径")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("不得包含查询参数或片段")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("端口非法")
		}
	}
	return u.Scheme + "://" + u.Host, nil
}

// authNameOK 校验用户名（避免空白与超长）。
func authNameOK(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("用户名不能为空")
	}
	if len([]byte(v)) > 64 {
		return "", fmt.Errorf("用户名过长")
	}
	if strings.ContainsAny(v, " \t\r\n") {
		return "", fmt.Errorf("用户名不能包含空白字符")
	}
	return v, nil
}

// authPasswordOK 校验初始口令策略，与 internal/auth 的规则保持一致（长度 12-72 字节）。
func authPasswordOK(v string) error {
	n := len([]byte(v))
	if n < 12 || n > 72 {
		return fmt.Errorf("长度需在 12-72 字节之间（当前 %d 字节）", n)
	}
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("口令不能为空")
	}
	return nil
}

// ListenAddr 返回 http 监听地址。
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.HTTPAddr, c.HTTPPort)
}
