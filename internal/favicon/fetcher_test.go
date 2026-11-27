package favicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/icon"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	img.Set(0, 0, color.RGBA{G: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码 PNG 失败：%v", err)
	}
	return buf.Bytes()
}

func newTestFetcher(t *testing.T, loopback bool, resolve func(string) []net.IP) *Fetcher {
	t.Helper()
	f := New()
	f.allowLoopback = loopback
	if resolve != nil {
		f.resolve = func(ctx context.Context, host string) ([]net.IP, error) {
			ips := resolve(host)
			if len(ips) == 0 {
				return nil, errors.New("no such host")
			}
			return ips, nil
		}
	}
	return f
}

func TestFetchRejectsUnsafeTargets(t *testing.T) {
	toLoopback := func(host string) []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} }
	cases := []struct {
		name    string
		target  string
		resolve func(string) []net.IP
	}{
		{"IPv4 回环", "http://127.0.0.1/favicon.ico", nil},
		{"IPv6 回环", "http://[::1]/favicon.ico", nil},
		{"localhost 解析到回环", "http://localhost/favicon.ico", toLoopback},
		{"A 类私有", "http://10.1.2.3/", nil},
		{"B 类私有", "http://172.20.0.1/", nil},
		{"C 类私有", "http://192.168.1.10/", nil},
		{"云元数据 169.254.169.254", "http://169.254.169.254/latest/meta-data/", nil},
		{"阿里云元数据 100.100.100.200", "http://100.100.100.200/latest/meta-data/", nil},
		{"未指定地址", "http://0.0.0.0/", nil},
		{"IPv6 ULA", "http://[fc00::1]/", nil},
		{"IPv6 链路本地", "http://[fe80::1]/", nil},
		{"IPv4 映射回环", "http://[::ffff:127.0.0.1]/", nil},
		{"运营商级 NAT", "http://100.64.0.5/", nil},
		{"DNS 重绑到回环", "http://rebind.example.com/", toLoopback},
		{"DNS 混合公私地址", "http://mixed.example.com/", func(string) []net.IP {
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.0.0.5")}
		}},
		{"只允许 http/https", "ftp://example.com/favicon.ico", nil},
		{"file 协议", "file:///etc/passwd", nil},
		{"gopher 协议", "gopher://127.0.0.1:70/", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newTestFetcher(t, false, c.resolve)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _, err := f.Fetch(ctx, c.target)
			if err == nil {
				t.Fatalf("应拒绝：%s", c.target)
			}
			if !errors.Is(err, ErrBlocked) && !strings.Contains(err.Error(), "不允许") {
				t.Fatalf("错误未体现地址拦截：%v", err)
			}
		})
	}
}

func TestFetchSuccessFromHTMLIconLink(t *testing.T) {
	pngData := testPNG(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><head><link rel="shortcut icon" href="/static/logo.png"></head></body></html>`)
	})
	mux.HandleFunc("/static/logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	f := newTestFetcher(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, mimeType, err := f.Fetch(ctx, ts.URL+"/")
	if err != nil {
		t.Fatalf("抓取失败：%v", err)
	}
	if mimeType != "image/png" || !bytes.Equal(data, pngData) {
		t.Fatalf("返回内容不符：%s len=%d", mimeType, len(data))
	}
}

func TestFetchFallbackFaviconICO(t *testing.T) {
	pngData := testPNG(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>无图标声明</title></head></html>`)
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	f := newTestFetcher(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, mimeType, err := f.Fetch(ctx, ts.URL+"/page"); err != nil || mimeType != "image/png" {
		t.Fatalf("回退 /favicon.ico 失败：%v %s", err, mimeType)
	}
}

func TestFetchDirectImageURL(t *testing.T) {
	pngData := testPNG(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer ts.Close()
	f := newTestFetcher(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, mimeType, err := f.Fetch(ctx, ts.URL); err != nil || mimeType != "image/png" {
		t.Fatalf("直接图片 URL 应成功：%v", err)
	}
}

func TestFetchBlocksRedirectToPrivate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer ts.Close()
	f := newTestFetcher(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := f.Fetch(ctx, ts.URL)
	if err == nil || !strings.Contains(err.Error(), "不允许") {
		t.Fatalf("重定向到元数据地址应被拦截：%v", err)
	}
}

func TestFetchBlocksRedirectScheme(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer ts.Close()
	f := newTestFetcher(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := f.Fetch(ctx, ts.URL); err == nil {
		t.Fatalf("重定向到 file:// 应被拦截")
	}
}

func TestFetchRejectsOversizeAndInvalidContent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/big-image", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(bytes.Repeat([]byte{0x89}, int(icon.MaxBytes)+64))
	})
	mux.HandleFunc("/big-page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(bytes.Repeat([]byte("a"), maxPageBytes+64))
	})
	mux.HandleFunc("/fake-image", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<html>not png</html>"))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"icon":"x"}`))
	})
	mux.HandleFunc("/no-icon", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html></html>`))
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	f := newTestFetcher(t, true, nil)
	for _, path := range []string{"/big-image", "/big-page", "/fake-image", "/json", "/no-icon"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err := f.Fetch(ctx, ts.URL+path)
		cancel()
		if err == nil {
			t.Fatalf("%s 应失败", path)
		}
	}
}

func TestIconLinks(t *testing.T) {
	base, _ := url.Parse("https://example.com/dir/page")
	page := []byte(`
<link rel="icon" href="/a.png">
<link rel="apple-touch-icon" href="b.png">
<link rel="stylesheet" href="/c.css">
<link rel="ICON" href="https://cdn.example.com/d.ico">
<link rel="icon" href="javascript:alert(1)">
<link rel="icon" href="/a.png">`)
	links := iconLinks(page, base)
	want := []string{"https://example.com/a.png", "https://example.com/dir/b.png", "https://cdn.example.com/d.ico"}
	if len(links) != len(want) {
		t.Fatalf("链接数量不符：%v", links)
	}
	for i := range want {
		if links[i] != want[i] {
			t.Fatalf("第 %d 个链接不符：%q != %q", i, links[i], want[i])
		}
	}
}

func TestFetchBlocksIconLinkToPrivate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><link rel="icon" href="http://169.254.169.254/x.png"></head></html>`)
	}))
	defer ts.Close()
	f := newTestFetcher(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := f.Fetch(ctx, ts.URL); err == nil {
		t.Fatalf("页面内指向元数据地址的图标链接应被拦截")
	}
}
