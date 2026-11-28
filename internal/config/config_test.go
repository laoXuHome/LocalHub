package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c, err := LoadFrom(nil)
	if err != nil {
		t.Fatalf("默认配置应合法，得到错误：%v", err)
	}
	if c.HTTPPort != 11030 {
		t.Errorf("默认端口应为 11030，得到 %d", c.HTTPPort)
	}
	if c.DataDir != "/data" {
		t.Errorf("默认数据目录应为 /data，得到 %q", c.DataDir)
	}
	if c.DBPath != "/data/localhub.db" {
		t.Errorf("默认数据库路径应为 /data/localhub.db，得到 %q", c.DBPath)
	}
	if c.PUID != 1000 || c.PGID != 1000 {
		t.Errorf("默认 PUID/PGID 应为 1000/1000，得到 %d/%d", c.PUID, c.PGID)
	}
	if c.AllowRoot {
		t.Error("默认不允许 root")
	}
	if !c.WriteCheckFatal {
		t.Error("默认写自检应为致命")
	}
	if c.AdminUsername != "admin" {
		t.Errorf("默认管理员用户名应为 admin，得到 %q", c.AdminUsername)
	}
	if c.SessionTTL != 12*time.Hour {
		t.Errorf("默认会话有效期应为 12h，得到 %v", c.SessionTTL)
	}
	if c.LoginMaxFailures != 5 || c.LoginWindow != 15*time.Minute || c.LoginLockout != 15*time.Minute {
		t.Errorf("默认登录限制参数不符：%d %v %v", c.LoginMaxFailures, c.LoginWindow, c.LoginLockout)
	}
	if c.CookieSecure || c.TrustProxy {
		t.Error("默认不要求 Secure Cookie、默认不信任代理头")
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{"端口非法", []string{"LOCALHUB_HTTP_PORT=70000"}, "LOCALHUB_HTTP_PORT"},
		{"数据目录必须绝对", []string{"LOCALHUB_DATA_DIR=relative/path"}, "LOCALHUB_DATA_DIR"},
		{"数据库越界", []string{"LOCALHUB_DB_PATH=/etc/passwd"}, "LOCALHUB_DB_PATH"},
		{"日志级别非法", []string{"LOCALHUB_LOG_LEVEL=verbose"}, "LOCALHUB_LOG_LEVEL"},
		{"PUID 为 0 被拒", []string{"PUID=0"}, "ALLOW_ROOT=1"},
		{"PUID 非法", []string{"PUID=abc"}, "PUID"},
		{"布尔值非法", []string{"WRITE_CHECK_FATAL=maybe"}, "WRITE_CHECK_FATAL"},
		{"初始口令过短", []string{"LOCALHUB_ADMIN_PASSWORD=short"}, "LOCALHUB_ADMIN_PASSWORD"},
		{"用户名含空白", []string{"LOCALHUB_ADMIN_USERNAME=a b"}, "LOCALHUB_ADMIN_USERNAME"},
		{"会话时长非法", []string{"LOCALHUB_SESSION_TTL=1m"}, "LOCALHUB_SESSION_TTL"},
		{"失败阈值非法", []string{"LOCALHUB_LOGIN_MAX_FAILURES=0"}, "LOCALHUB_LOGIN_MAX_FAILURES"},
		{"窗口时长非法", []string{"LOCALHUB_LOGIN_WINDOW=1s"}, "LOCALHUB_LOGIN_WINDOW"},
		{"锁定时长非法", []string{"LOCALHUB_LOGIN_LOCKOUT=99h"}, "LOCALHUB_LOGIN_LOCKOUT"},
		{"Cookie 布尔非法", []string{"LOCALHUB_COOKIE_SECURE=yes-please"}, "LOCALHUB_COOKIE_SECURE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFrom(tc.env)
			if err == nil {
				t.Fatalf("期望报错，但没有")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误信息应包含 %q，实际：%v", tc.want, err)
			}
		})
	}
}

func TestAllowRootExplicit(t *testing.T) {
	c, err := LoadFrom([]string{"PUID=0", "PGID=0", "ALLOW_ROOT=1"})
	if err != nil {
		t.Fatalf("显式 ALLOW_ROOT=1 时应允许 root：%v", err)
	}
	if !c.AllowRoot || c.PUID != 0 {
		t.Errorf("期望 AllowRoot=true 且 PUID=0，得到 %+v", c)
	}
}

func TestListenAddr(t *testing.T) {
	c, err := LoadFrom([]string{"LOCALHUB_HTTP_ADDR=127.0.0.1", "LOCALHUB_HTTP_PORT=18080"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ListenAddr(); got != "127.0.0.1:18080" {
		t.Errorf("ListenAddr 期望 127.0.0.1:18080，得到 %s", got)
	}
}

func TestDockerConfigDefaultsAndNormalization(t *testing.T) {
	c, err := LoadFrom(nil)
	if err != nil {
		t.Fatalf("默认配置不应报错：%v", err)
	}
	if c.DockerAPIURL != "" {
		t.Fatalf("默认 Docker 地址应为空，得到 %q", c.DockerAPIURL)
	}
	if c.DockerTimeout != 3*time.Second {
		t.Fatalf("默认 Docker 超时应为 3s，得到 %v", c.DockerTimeout)
	}

	c, err = LoadFrom([]string{
		"LOCALHUB_DOCKER_API_URL=tcp://docker-socket-proxy:2375",
		"LOCALHUB_DOCKER_TIMEOUT=5s",
	})
	if err != nil {
		t.Fatalf("合法的 Docker 配置不应报错：%v", err)
	}
	if c.DockerAPIURL != "http://docker-socket-proxy:2375" {
		t.Fatalf("tcp:// 应归一化为 http://，得到 %q", c.DockerAPIURL)
	}
	if c.DockerTimeout != 5*time.Second {
		t.Fatalf("Docker 超时错误：%v", c.DockerTimeout)
	}
}

func TestDockerConfigRejectsUnsafeValues(t *testing.T) {
	cases := []struct{ name, value, want string }{
		{"Unix Socket 被拒", "LOCALHUB_DOCKER_API_URL=unix:///var/run/docker.sock", "LOCALHUB_DOCKER_API_URL"},
		{"非 HTTP 协议", "LOCALHUB_DOCKER_API_URL=ftp://proxy:2375", "LOCALHUB_DOCKER_API_URL"},
		{"带路径", "LOCALHUB_DOCKER_API_URL=http://proxy:2375/v1.52/containers/json", "LOCALHUB_DOCKER_API_URL"},
		{"带用户信息", "LOCALHUB_DOCKER_API_URL=http://user:pass@proxy:2375", "LOCALHUB_DOCKER_API_URL"},
		{"超时过短", "LOCALHUB_DOCKER_TIMEOUT=100ms", "LOCALHUB_DOCKER_TIMEOUT"},
		{"超时过长", "LOCALHUB_DOCKER_TIMEOUT=60s", "LOCALHUB_DOCKER_TIMEOUT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFrom([]string{tc.value})
			if err == nil {
				t.Fatalf("期望报错，但没有")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误信息应包含 %q，实际：%v", tc.want, err)
			}
		})
	}
}
