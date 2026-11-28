package dockerclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRejectsUnsafeURLs(t *testing.T) {
	cases := []string{
		"unix:///var/run/docker.sock",
		"ftp://docker-proxy:2375",
		"http://user:pass@docker-proxy:2375",
		"http://docker-proxy:2375/v1.52/containers/json",
		"http://docker-proxy:2375?x=1",
	}
	for _, raw := range cases {
		if _, err := New(raw, time.Second); err == nil {
			t.Fatalf("%q 应被拒绝", raw)
		}
	}
}

func TestNewNormalizesTCPAndEmpty(t *testing.T) {
	c, err := New("tcp://docker-proxy:2375", time.Second)
	if err != nil {
		t.Fatalf("tcp:// 应转换为 http://：%v", err)
	}
	if !c.configured || c.baseURL != "http://docker-proxy:2375" {
		t.Fatalf("连接地址归一化错误：configured=%v base=%q", c.configured, c.baseURL)
	}

	empty, err := New("  ", time.Second)
	if err != nil {
		t.Fatalf("空地址不应报错：%v", err)
	}
	if empty.configured {
		t.Fatal("空地址不应视为已配置")
	}
}

func TestCheckMapsStatuses(t *testing.T) {
	cases := []struct {
		name   string
		code   int
		body   string
		status Status
	}{
		{"连接成功", 200, `{"ApiVersion":"1.52","Version":"29.1.3"}`, StatusConnected},
		{"权限不足 401", 401, `{"message":"unauthorized"}`, StatusPermissionDenied},
		{"权限不足 403", 403, `forbidden`, StatusPermissionDenied},
		{"代理 502", 502, `bad gateway`, StatusServiceUnavailable},
		{"服务 503", 503, `unavailable`, StatusServiceUnavailable},
		{"其他异常", 500, `boom`, StatusUnknown},
		{"响应不是 Docker version", 200, `{}`, StatusUnknown},
		{"响应不是 JSON", 200, `<html>`, StatusUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/version" {
					t.Errorf("只读客户端只应访问 /version，实际 %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			c, err := New(ts.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			result := c.Check(context.Background())
			if result.Status != tc.status {
				t.Fatalf("状态期望 %s，得到 %s", tc.status, result.Status)
			}
			if result.APIVersion != "" && tc.status == StatusConnected && result.APIVersion != "1.52" {
				t.Fatalf("API 版本错误：%q", result.APIVersion)
			}
			if strings.Contains(result.Message, "127.0.0.1") || strings.Contains(result.Message, "http") {
				t.Fatalf("状态消息不得包含连接地址：%q", result.Message)
			}
		})
	}
}

func TestCheckConnectionRefusedAndTimeout(t *testing.T) {
	// 取一个已关闭的本地端口，制造真实的 TCP 连接拒绝。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	c, err := New("http://"+addr, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Check(context.Background()).Status; got != StatusConnectionFailed {
		t.Fatalf("连接拒绝应映射为 connection_failed，得到 %s", got)
	}

	// 接受连接但永不响应，触发客户端超时。
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()
	timeoutClient, err := New(ts.URL, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got := timeoutClient.Check(context.Background()).Status; got != StatusConnectionFailed {
		t.Fatalf("超时应映射为 connection_failed，得到 %s", got)
	}
}

func TestCheckNotConfigured(t *testing.T) {
	c, err := New("", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result := c.Check(context.Background())
	if result.Status != StatusNotConnected {
		t.Fatalf("未配置应映射为 not_connected，得到 %s", result.Status)
	}
	if !result.ReadOnly {
		t.Fatal("状态必须声明只读")
	}
}
