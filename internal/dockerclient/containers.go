package dockerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ContainerStatus 是 P5.2 对外暴露的四种统一状态。
type ContainerStatus string

const (
	ContainerRunning ContainerStatus = "running"
	ContainerStopped ContainerStatus = "stopped"
	ContainerError   ContainerStatus = "error"
	ContainerUnknown ContainerStatus = "unknown"
)

// Counts 是容器状态统计。
type Counts struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Error   int `json:"error"`
	Unknown int `json:"unknown"`
}

// Container 是脱敏后的容器发现信息。
// 不包含环境变量、容器配置、挂载详情或 Docker 内部错误。
type Container struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Status    ContainerStatus   `json:"status"`
	CreatedAt string            `json:"created_at,omitempty"`
	StartedAt string            `json:"started_at,omitempty"`
	Ports     []string          `json:"ports"`
	Labels    map[string]string `json:"labels"`
}

// ContainerList 是容器发现结果。连接失败时 Containers 为空，前端必须保留上一次有效数据。
type ContainerList struct {
	Status     Status
	CheckedAt  string
	Message    string
	ReadOnly   bool
	Containers []Container
	Counts     Counts
	Skipped    int
	Truncated  bool
}

type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type dockerContainer struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Ports   []dockerPort      `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

const (
	maxContainers       = 2000
	startedAtFetchLimit = 100
)

// ListContainers 通过只读 Socket Proxy 获取容器列表，并补充 Docker API 提供的启动时间。
// 单个容器数据异常会被跳过，不影响其余容器。
func (c *Client) ListContainers(ctx context.Context) ContainerList {
	now := time.Now().UTC().Format(time.RFC3339)
	if c == nil || !c.configured {
		return ContainerList{Status: StatusNotConnected, CheckedAt: now, Message: "尚未配置 Docker 连接", ReadOnly: true, Containers: []Container{}}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/containers/json?all=1", nil)
	if err != nil {
		return emptyList(StatusUnknown, now, "Docker 状态未知")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LocalHub-DockerReadOnly/1")

	resp, err := c.http.Do(req)
	if err != nil {
		return emptyList(StatusConnectionFailed, now, "无法连接 Docker 服务")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		status, message := mapHTTPStatus(resp.StatusCode)
		return emptyList(status, now, message)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return emptyList(StatusConnectionFailed, now, "无法连接 Docker 服务")
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		return emptyList(StatusUnknown, now, "Docker 状态未知")
	}

	containers := make([]Container, 0, len(raws))
	skipped := 0
	for _, raw := range raws {
		if len(containers) >= maxContainers {
			break
		}
		var item dockerContainer
		if err := json.Unmarshal(raw, &item); err != nil || item.ID == "" {
			skipped++
			continue
		}
		containers = append(containers, normalizeContainer(item))
	}

	// 启动时间仅在 /containers/{id}/json 中提供；最多补充 100 个，避免大量容器造成请求放大。
	enrichStartedAt(ctx, c, containers)

	counts := countContainers(containers)
	return ContainerList{
		Status:     StatusConnected,
		CheckedAt:  now,
		Message:    "Docker 容器列表已更新",
		ReadOnly:   true,
		Containers: containers,
		Counts:     counts,
		Skipped:    skipped,
		Truncated:  len(raws) > maxContainers,
	}
}

func emptyList(status Status, checkedAt, message string) ContainerList {
	return ContainerList{
		Status:     status,
		CheckedAt:  checkedAt,
		Message:    message,
		ReadOnly:   true,
		Containers: []Container{},
	}
}

func mapHTTPStatus(code int) (Status, string) {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return StatusPermissionDenied, "Docker 连接权限不足"
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return StatusServiceUnavailable, "Docker 服务暂时不可用"
	default:
		return StatusUnknown, "Docker 状态未知"
	}
}

func normalizeContainer(item dockerContainer) Container {
	name := ""
	if len(item.Names) > 0 {
		name = strings.TrimPrefix(item.Names[0], "/")
	}
	if name == "" {
		name = shortID(item.ID)
	}
	ports := make([]string, 0, len(item.Ports))
	for _, p := range item.Ports {
		if p.PrivatePort <= 0 {
			continue
		}
		if p.PublicPort > 0 {
			ip := p.IP
			if ip == "" || ip == "0.0.0.0" || ip == "::" {
				ip = "*"
			}
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", ip, p.PublicPort, p.PrivatePort, p.Type))
		} else {
			ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	sort.Strings(ports)
	labels := make(map[string]string, len(item.Labels))
	for k, v := range item.Labels {
		labels[k] = v
	}
	container := Container{
		ID:     item.ID,
		Name:   name,
		Image:  item.Image,
		Status: containerStatus(item.State, item.Status),
		Ports:  ports,
		Labels: labels,
	}
	if item.Created > 0 {
		container.CreatedAt = time.Unix(item.Created, 0).UTC().Format(time.RFC3339)
	}
	return container
}

func containerStatus(state, statusText string) ContainerStatus {
	switch strings.ToLower(state) {
	case "running":
		if strings.Contains(strings.ToLower(statusText), "unhealthy") {
			return ContainerError
		}
		return ContainerRunning
	case "exited", "created":
		return ContainerStopped
	case "restarting", "dead", "paused", "removing":
		return ContainerError
	default:
		return ContainerUnknown
	}
}

func countContainers(items []Container) Counts {
	counts := Counts{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case ContainerRunning:
			counts.Running++
		case ContainerStopped:
			counts.Stopped++
		case ContainerError:
			counts.Error++
		default:
			counts.Unknown++
		}
	}
	return counts
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func enrichStartedAt(ctx context.Context, c *Client, containers []Container) {
	if len(containers) == 0 {
		return
	}
	limit := len(containers)
	if limit > startedAtFetchLimit {
		limit = startedAtFetchLimit
	}
	// 只对列表前 100 个补充启动时间；其余保留空值，不影响列表可用性。
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			started, ok := c.containerStartedAt(ctx, containers[i].ID)
			if ok {
				containers[i].StartedAt = started
			}
		}(i)
	}
	wg.Wait()
}

func (c *Client) containerStartedAt(ctx context.Context, id string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LocalHub-DockerReadOnly/1")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", false
	}
	var detail struct {
		State struct {
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
	}
	if err := json.Unmarshal(body, &detail); err != nil || detail.State.StartedAt == "" {
		return "", false
	}
	parsed, err := time.Parse(time.RFC3339Nano, detail.State.StartedAt)
	if err != nil || parsed.Year() <= 1 {
		return "", false
	}
	return parsed.UTC().Format(time.RFC3339), true
}
