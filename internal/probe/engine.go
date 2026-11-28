package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/network"
)

const (
	DefaultTimeout      = 4 * time.Second
	DefaultMaxBodyBytes = int64(64 << 10)
)

// Prober 是单目标探测能力；Manager 依赖该接口便于测试替换。
type Prober interface {
	Probe(ctx context.Context, target network.Target) Result
}

// HTTPProber 使用 HTTP/HTTPS 请求探测目标，禁止重定向、代理和任意 URL。
type HTTPProber struct {
	Client       *http.Client
	Timeout      time.Duration
	MaxBodyBytes int64
}

// NewHTTPProber 创建面向内网设备的探测客户端。
// 注意：Proxy 显式设为 nil，避免请求被环境代理转发；重定向一律不跟随。
func NewHTTPProber() *HTTPProber {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          3,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   DefaultTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &HTTPProber{Client: client, Timeout: DefaultTimeout, MaxBodyBytes: DefaultMaxBodyBytes}
}

// Probe 对单个目标执行探测；一个目标的失败只影响自己的结果。
// 先尝试 HTTP；仅在连接层失败时尝试 HTTPS，避免对普通 HTTP 设备重复等待。
func (p *HTTPProber) Probe(ctx context.Context, target network.Target) Result {
	result := Result{
		Key: target.Key, Label: target.Label, Address: target.Address, Enabled: target.Enabled,
		Status: StatusNotProbed,
	}
	if !target.Enabled {
		result.Status = StatusDisabled
		return result
	}
	if err := network.ValidateAddress(target.Address); err != nil {
		result.Status = StatusProbeError
		result.FailureInfo = "地址未通过校验，未发起请求"
		result.CheckedAt = formatTime(time.Now())
		return result
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	first := p.attempt(probeCtx, "http", target)
	if first.Status == StatusSuccess || first.Status == StatusHTTPError || first.Status == StatusTimeout {
		return first
	}
	// 只有连接层失败才尝试 HTTPS；响应超限/读取异常等结果不能二次请求。
	if first.Status != StatusConnectionFailed {
		return first
	}

	second := p.attempt(probeCtx, "https", target)
	if second.Status == StatusSuccess || second.Status == StatusHTTPError || second.Status == StatusTimeout || second.Status == StatusProbeError {
		return second
	}
	if first.Status == StatusConnectionFailed && second.Status == StatusConnectionFailed {
		first.FailureInfo = "HTTP 与 HTTPS 均连接失败"
		return first
	}
	return second
}

func (p *HTTPProber) attempt(ctx context.Context, scheme string, target network.Target) Result {
	maxBody := p.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}
	host := target.Address
	if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	host = strings.TrimSuffix(host, ".")
	url := scheme + "://" + host + "/"

	result := Result{
		Key: target.Key, Label: target.Label, Address: target.Address, Enabled: target.Enabled,
		Status: StatusProbeError,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.FailureInfo = "无法构造探测请求"
		result.CheckedAt = formatTime(time.Now())
		return result
	}
	req.Header.Set("User-Agent", "LocalHub-Probe/1.0")
	req.Header.Set("Accept", "*/*")

	start := time.Now()
	resp, err := p.Client.Do(req)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.CheckedAt = formatTime(time.Now())
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			result.Status = StatusTimeout
			result.FailureInfo = "探测超时"
		} else {
			result.Status = StatusConnectionFailed
			result.FailureInfo = err.Error()
		}
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	result.CheckedAt = formatTime(time.Now())
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Status = StatusHTTPError
		result.FailureInfo = fmt.Sprintf("HTTP 状态码 %d", resp.StatusCode)
		return result
	}
	if resp.ContentLength > maxBody {
		result.Status = StatusProbeError
		result.FailureInfo = "响应体超过限制"
		return result
	}
	n, err := io.CopyN(io.Discard, resp.Body, maxBody+1)
	if err != nil && !errors.Is(err, io.EOF) {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			result.Status = StatusTimeout
			result.FailureInfo = "读取响应超时"
		} else {
			result.Status = StatusProbeError
			result.FailureInfo = "读取响应失败"
		}
		return result
	}
	if n > maxBody {
		result.Status = StatusProbeError
		result.FailureInfo = "响应体超过限制"
		return result
	}
	result.Status = StatusSuccess
	return result
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}
