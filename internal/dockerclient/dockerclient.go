// Package dockerclient 提供 P5.1 所需的 Docker 只读连接探测。
//
// 本包只实现“读取 /version”这一种能力，不包含容器、镜像、网络、卷或
// exec 的任何写操作。所有访问必须经由部署方提供的 Docker Socket Proxy，
// 不直接连接 Docker Unix Socket。
package dockerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Status 是连接状态枚举。
type Status string

const (
	StatusConnected          Status = "connected"
	StatusNotConnected       Status = "not_connected"
	StatusConnectionFailed   Status = "connection_failed"
	StatusPermissionDenied   Status = "permission_denied"
	StatusServiceUnavailable Status = "service_unavailable"
	StatusUnknown            Status = "unknown"
)

const (
	// defaultTimeout 在调用方未给出有效超时时使用。
	defaultTimeout = 3 * time.Second
	// maxResponseBytes 限制 /version 响应体，避免异常服务占用内存。
	maxResponseBytes = 1 << 20
)

// CheckResult 是给管理界面使用的脱敏状态。
// 不包含连接地址、Socket 路径、响应体或底层错误文本。
type CheckResult struct {
	Status     Status `json:"status"`
	APIVersion string `json:"api_version,omitempty"`
	CheckedAt  string `json:"checked_at"`
	Message    string `json:"message"`
	ReadOnly   bool   `json:"read_only"`
}

// Client 是只读 Docker API 客户端。它只允许访问 /version。
type Client struct {
	baseURL    string
	configured bool
	http       *http.Client
}

// New 创建只读客户端。apiURL 为空表示未配置 Docker 连接。
// 仅接受 http:// 或 https://；tcp:// 会按 Docker 惯例转换为 http://。
// unix:// 被拒绝，防止绕过 Socket Proxy 直接访问 Docker Socket。
func New(apiURL string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	trimmed := strings.TrimSpace(apiURL)
	if trimmed == "" {
		return &Client{
			configured: false,
			http: &http.Client{
				Timeout: timeout,
			},
		}, nil
	}
	baseURL, err := normalizeBaseURL(trimmed)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:    baseURL,
		configured: true,
		http: &http.Client{
			Timeout: timeout,
			// 不跟随重定向：避免把只读请求带往其他地址。
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "unix://") {
		return "", errors.New("不允许使用 unix:// 连接 Docker Socket，必须通过 Docker Socket Proxy")
	}
	if strings.HasPrefix(lower, "tcp://") {
		raw = "http://" + raw[len("tcp://"):]
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("Docker 连接地址非法：%w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("Docker 连接地址只支持 http://、https:// 或 tcp://")
	}
	if u.User != nil {
		return "", errors.New("Docker 连接地址不得包含用户名或口令")
	}
	if u.Host == "" {
		return "", errors.New("Docker 连接地址缺少主机")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("Docker 连接地址不得包含查询参数或片段")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("Docker 连接地址不得包含路径，请只填写代理根地址")
	}
	host := u.Hostname()
	if host == "" {
		return "", errors.New("Docker 连接地址缺少主机")
	}
	if port := u.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return "", errors.New("Docker 连接地址端口非法")
		}
	}
	return u.Scheme + "://" + u.Host, nil
}

// Check 查询 Docker /version，并把结果映射为固定状态。
// 网络、代理、权限等原因都不会返回底层错误文本。
func (c *Client) Check(ctx context.Context) CheckResult {
	now := time.Now().UTC().Format(time.RFC3339)
	if c == nil || !c.configured {
		return CheckResult{
			Status:    StatusNotConnected,
			CheckedAt: now,
			Message:   "尚未配置 Docker 连接",
			ReadOnly:  true,
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/version", nil)
	if err != nil {
		return CheckResult{Status: StatusUnknown, CheckedAt: now, Message: "Docker 状态未知", ReadOnly: true}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LocalHub-DockerReadOnly/1")

	resp, err := c.http.Do(req)
	if err != nil {
		return CheckResult{Status: StatusConnectionFailed, CheckedAt: now, Message: "无法连接 Docker 服务", ReadOnly: true}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if err != nil {
			return CheckResult{Status: StatusConnectionFailed, CheckedAt: now, Message: "无法连接 Docker 服务", ReadOnly: true}
		}
		var payload struct {
			APIVersion string `json:"ApiVersion"`
			Version    string `json:"Version"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.APIVersion == "" || payload.Version == "" {
			return CheckResult{Status: StatusUnknown, CheckedAt: now, Message: "Docker 状态未知", ReadOnly: true}
		}
		return CheckResult{
			Status:     StatusConnected,
			APIVersion: payload.APIVersion,
			CheckedAt:  now,
			Message:    "Docker 只读连接正常",
			ReadOnly:   true,
		}
	case http.StatusUnauthorized, http.StatusForbidden:
		return CheckResult{Status: StatusPermissionDenied, CheckedAt: now, Message: "Docker 连接权限不足", ReadOnly: true}
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return CheckResult{Status: StatusServiceUnavailable, CheckedAt: now, Message: "Docker 服务暂时不可用", ReadOnly: true}
	default:
		return CheckResult{Status: StatusUnknown, CheckedAt: now, Message: "Docker 状态未知", ReadOnly: true}
	}
}
