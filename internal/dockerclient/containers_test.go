package dockerclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListContainersMapsStatusesAndCounts(t *testing.T) {
	var detailCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			if r.URL.Query().Get("all") != "1" {
				t.Fatalf("必须请求 all=1，实际 query=%q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"Id":"aaa","Names":["/web"],"Image":"nginx:latest","Created":1700000000,"State":"running","Status":"Up 2 hours","Ports":[{"IP":"0.0.0.0","PrivatePort":80,"PublicPort":8080,"Type":"tcp"},{"PrivatePort":443,"Type":"tcp"}],"Labels":{"com.example.role":"web"}},
				{"Id":"bbb","Names":["/db"],"Image":"postgres:16","Created":1700000001,"State":"exited","Status":"Exited (0) 1 hour ago","Ports":[],"Labels":{}},
				{"Id":"ccc","Names":["/restart"],"Image":"app:1","Created":1700000002,"State":"restarting","Status":"Restarting (1) 10 seconds ago","Ports":[],"Labels":{"com.example.role":"worker"}},
				{"Id":"ddd","Names":["/weird"],"Image":"app:2","Created":1700000003,"State":"","Status":"","Ports":[],"Labels":{}},
				"not-an-object"
			]`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			detailCalls++
			if strings.Contains(r.URL.Path, "ccc") {
				http.Error(w, "fixture detail failure", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"State":{"StartedAt":"2026-11-28T06:00:00.123456789Z"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := New(srv.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	list := client.ListContainers(context.Background())
	if list.Status != StatusConnected {
		t.Fatalf("状态应为 connected，得到 %s", list.Status)
	}
	if len(list.Containers) != 4 || list.Skipped != 1 {
		t.Fatalf("应有 4 个有效容器、1 个跳过，得到 %d/%d", len(list.Containers), list.Skipped)
	}
	if list.Counts != (Counts{Total: 4, Running: 1, Stopped: 1, Error: 1, Unknown: 1}) {
		t.Fatalf("统计错误：%+v", list.Counts)
	}
	if list.Containers[0].Name != "web" || list.Containers[0].Image != "nginx:latest" {
		t.Fatalf("名称/镜像解析错误：%+v", list.Containers[0])
	}
	if list.Containers[0].StartedAt == "" {
		t.Fatalf("running 容器应补充 StartedAt")
	}
	if list.Containers[2].StartedAt != "" {
		t.Fatalf("详情失败时不应影响列表，且 StartedAt 留空")
	}
	if got := list.Containers[0].Ports; len(got) != 2 || got[0] != "*:8080->80/tcp" || got[1] != "443/tcp" {
		t.Fatalf("端口格式化错误：%v", got)
	}
	if list.Containers[0].Labels["com.example.role"] != "web" {
		t.Fatalf("Labels 应保留为普通数据")
	}
	if detailCalls < 4 {
		t.Fatalf("应为容器请求详情，实际 %d", detailCalls)
	}
}

func TestListContainersPermissionAndConnectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden upstream body", http.StatusForbidden)
	}))
	defer srv.Close()
	client, err := New(srv.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	list := client.ListContainers(context.Background())
	if list.Status != StatusPermissionDenied || len(list.Containers) != 0 {
		t.Fatalf("403 应返回 permission_denied 且空列表：%+v", list)
	}

	client, _ = New("http://127.0.0.1:1", time.Second)
	list = client.ListContainers(context.Background())
	if list.Status != StatusConnectionFailed || len(list.Containers) != 0 {
		t.Fatalf("连接失败应返回 connection_failed 且空列表：%+v", list)
	}
	if list.Counts.Stopped != 0 {
		t.Fatalf("连接失败不得把容器统计为 stopped：%+v", list.Counts)
	}
}

func TestListContainersNotConfiguredAndInvalidBody(t *testing.T) {
	client, _ := New("", time.Second)
	list := client.ListContainers(context.Background())
	if list.Status != StatusNotConnected || len(list.Containers) != 0 {
		t.Fatalf("未配置状态错误：%+v", list)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client, _ = New(srv.URL, time.Second)
	list = client.ListContainers(context.Background())
	if list.Status != StatusUnknown || len(list.Containers) != 0 {
		t.Fatalf("非法 JSON 应返回 unknown：%+v", list)
	}
}

func TestContainerStatusDoesNotBlindlyMapDockerState(t *testing.T) {
	cases := []struct {
		state, status string
		want          ContainerStatus
	}{
		{"running", "Up 2 hours (healthy)", ContainerRunning},
		{"running", "Up 2 hours (unhealthy)", ContainerError},
		{"exited", "Exited (0)", ContainerStopped},
		{"created", "Created", ContainerStopped},
		{"restarting", "Restarting (1)", ContainerError},
		{"dead", "Dead", ContainerError},
		{"paused", "Up (Paused)", ContainerError},
		{"", "", ContainerUnknown},
		{"mystery", "whatever", ContainerUnknown},
	}
	for _, tc := range cases {
		if got := containerStatus(tc.state, tc.status); got != tc.want {
			t.Errorf("state=%q status=%q 期望 %s，得到 %s", tc.state, tc.status, tc.want, got)
		}
	}
}
