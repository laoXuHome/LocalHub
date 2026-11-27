package config

import (
	"fmt"
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

// ListenAddr 返回 http 监听地址。
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.HTTPAddr, c.HTTPPort)
}
