package config

import (
	"strings"
	"testing"
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
