package probe

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/network"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testTarget(address string) network.Target {
	return network.Target{Key: network.TargetRouter, Label: "主路由器", Address: address, Enabled: true}
}

func testClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func routingProber(t *testing.T, handler http.HandlerFunc, timeout time.Duration, maxBody int64) (*HTTPProber, *atomic.Int32) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dest := strings.TrimPrefix(ts.URL, "http://")
	var count atomic.Int32
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, networkName, _ string) (net.Conn, error) {
			count.Add(1)
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, networkName, dest)
		},
	}
	return &HTTPProber{Client: testClient(transport), Timeout: timeout, MaxBodyBytes: maxBody}, &count
}

func TestProbeSuccessAndHTTPError(t *testing.T) {
	prober, _ := routingProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}, time.Second, DefaultMaxBodyBytes)
	got := prober.Probe(context.Background(), testTarget("router.test"))
	if got.Status != StatusSuccess || got.HTTPStatus != 200 {
		t.Fatalf("200 应成功：%+v", got)
	}
	if got.LatencyMS < 0 || got.CheckedAt == "" {
		t.Fatalf("成功结果应含耗时与时间：%+v", got)
	}

	errorProber, _ := routingProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}, time.Second, DefaultMaxBodyBytes)
	got = errorProber.Probe(context.Background(), testTarget("router.test"))
	if got.Status != StatusHTTPError || got.HTTPStatus != 503 {
		t.Fatalf("503 应归类为 HTTP 错误：%+v", got)
	}
}

func TestProbeDoesNotFollowRedirect(t *testing.T) {
	var finalHits atomic.Int32
	prober, _ := routingProber(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			finalHits.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}, time.Second, DefaultMaxBodyBytes)
	got := prober.Probe(context.Background(), testTarget("router.test"))
	if got.Status != StatusHTTPError || got.HTTPStatus != http.StatusFound {
		t.Fatalf("302 应作为 HTTP 错误且不跟随：%+v", got)
	}
	if finalHits.Load() != 0 {
		t.Fatal("探测客户端不能跟随重定向")
	}
}

func TestProbeTimeoutAndConnectionFailure(t *testing.T) {
	prober, _ := routingProber(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
	}, 100*time.Millisecond, DefaultMaxBodyBytes)
	got := prober.Probe(context.Background(), testTarget("router.test"))
	if got.Status != StatusTimeout {
		t.Fatalf("慢响应应归类为超时：%+v", got)
	}

	failTransport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	})
	failProber := &HTTPProber{Client: testClient(failTransport), Timeout: time.Second, MaxBodyBytes: DefaultMaxBodyBytes}
	got = failProber.Probe(context.Background(), testTarget("offline.test"))
	if got.Status != StatusConnectionFailed {
		t.Fatalf("连接拒绝应归类为连接失败：%+v", got)
	}
}

func TestProbeBodyLimitAndInvalidTarget(t *testing.T) {
	prober, _ := routingProber(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.CopyN(w, strings.NewReader(strings.Repeat("x", 40<<10)), 40<<10)
	}, time.Second, 16<<10)
	got := prober.Probe(context.Background(), testTarget("router.test"))
	if got.Status != StatusProbeError || !strings.Contains(got.FailureInfo, "响应体超过限制") {
		t.Fatalf("超限响应应失败：%+v", got)
	}

	var calls atomic.Int32
	invalidProber := &HTTPProber{
		Client: testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("must not be called")
		})),
		Timeout: time.Second, MaxBodyBytes: DefaultMaxBodyBytes,
	}
	got = invalidProber.Probe(context.Background(), testTarget("http://evil.test/path"))
	if got.Status != StatusProbeError || calls.Load() != 0 {
		t.Fatalf("非法地址不能发起请求：%+v calls=%d", got, calls.Load())
	}
}

func TestProbeDisabledDoesNotRequest(t *testing.T) {
	var calls atomic.Int32
	prober := &HTTPProber{Client: testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))}
	target := testTarget("router.test")
	target.Enabled = false
	got := prober.Probe(context.Background(), target)
	if got.Status != StatusDisabled || calls.Load() != 0 {
		t.Fatalf("禁用目标不能发起请求：%+v calls=%d", got, calls.Load())
	}
}

func TestProbeFallsBackToHTTPSOnConnectionFailure(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Scheme == "http" {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		}
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})
	prober := &HTTPProber{Client: testClient(transport), Timeout: time.Second, MaxBodyBytes: DefaultMaxBodyBytes}
	got := prober.Probe(context.Background(), testTarget("nas.test"))
	if got.Status != StatusSuccess || got.HTTPStatus != 200 {
		t.Fatalf("HTTP 连接失败后应尝试 HTTPS：%+v", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("HTTP 与 HTTPS 应各尝试一次，got %d", calls.Load())
	}
}
