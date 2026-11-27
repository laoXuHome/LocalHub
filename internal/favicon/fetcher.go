package favicon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/icon"
)

const (
	maxPageBytes  = 512 << 10
	maxRedirects  = 5
	maxCandidates = 4
)

// ErrBlocked 表示目标包含不允许访问的地址或协议。
var ErrBlocked = errors.New("目标地址不允许访问")

// Fetcher 负责从书签 URL 安全抓取 Favicon。
// 所有出站连接都经过地址校验，DNS 解析结果在拨号前逐个检查，重定向逐跳校验。
type Fetcher struct {
	Client  *http.Client
	maxPage int64

	// resolve 可注入，便于测试 DNS 相关场景。
	resolve func(ctx context.Context, host string) ([]net.IP, error)
	// allowLoopback 仅供同包测试使用；生产构造不会开启。
	allowLoopback bool
}

// New 构造带 SSRF 防护的抓取器：不使用环境代理，限制总超时与响应大小。
func New() *Fetcher {
	f := &Fetcher{
		maxPage: maxPageBytes,
		resolve: func(ctx context.Context, host string) ([]net.IP, error) {
			addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			ips := make([]net.IP, 0, len(addrs))
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
			return ips, nil
		},
	}
	f.Client = &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil, // 忽略 HTTP(S)_PROXY，避免被代理绕过校验
			DialContext:           f.safeDial,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			DisableKeepAlives:     true,
		},
		Timeout:       10 * time.Second,
		CheckRedirect: f.checkRedirect,
	}
	return f
}

// Fetch 抓取图标，返回通过校验的图片字节与规范 mime。
// 只允许 http/https；只接受图片响应或 HTML 页面中的 icon 链接。
func (f *Fetcher) Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := parseTarget(raw)
	if err != nil {
		return nil, "", err
	}
	if _, err := f.resolveChecked(ctx, u.Hostname()); err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("构造请求失败")
	}
	req.Header.Set("User-Agent", "LocalHub-favicon/1.0")
	req.Header.Set("Accept", "image/*, text/html;q=0.8")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("请求目标失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("目标返回状态 %d", resp.StatusCode)
	}

	base := resp.Request.URL
	switch mt := contentBase(resp.Header.Get("Content-Type")); {
	case strings.HasPrefix(mt, "image/"):
		data, err := readLimited(resp.Body, icon.MaxBytes)
		if err != nil {
			return nil, "", err
		}
		mimeType, _, err := icon.Validate(data)
		if err != nil {
			return nil, "", err
		}
		return data, mimeType, nil
	case mt == "text/html", mt == "application/xhtml+xml", mt == "":
		page, err := readLimited(resp.Body, f.maxPage)
		if err != nil {
			return nil, "", err
		}
		candidates := iconLinks(page, base)
		fallback := base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
		candidates = append(candidates, fallback)
		if len(candidates) > maxCandidates {
			candidates = candidates[:maxCandidates]
		}
		for _, c := range candidates {
			data, mimeType, err := f.fetchImage(ctx, c)
			if err == nil {
				return data, mimeType, nil
			}
		}
		return nil, "", errors.New("未找到可用 Favicon")
	default:
		return nil, "", fmt.Errorf("不支持的响应类型：%s", mt)
	}
}

func (f *Fetcher) fetchImage(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := parseTarget(raw)
	if err != nil {
		return nil, "", err
	}
	if _, err := f.resolveChecked(ctx, u.Hostname()); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("构造图标请求失败")
	}
	req.Header.Set("User-Agent", "LocalHub-favicon/1.0")
	req.Header.Set("Accept", "image/*, application/octet-stream;q=0.5")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("图标返回状态 %d", resp.StatusCode)
	}
	ct := contentBase(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.HasPrefix(ct, "image/") && ct != "application/octet-stream" {
		return nil, "", fmt.Errorf("图标响应类型不支持：%s", ct)
	}
	data, err := readLimited(resp.Body, icon.MaxBytes)
	if err != nil {
		return nil, "", err
	}
	mimeType, _, err := icon.Validate(data)
	if err != nil {
		return nil, "", err
	}
	return data, mimeType, nil
}

func parseTarget(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("URL 解析失败")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w：只支持 http/https", ErrBlocked)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w：缺少主机名", ErrBlocked)
	}
	return u, nil
}

func (f *Fetcher) resolveChecked(ctx context.Context, host string) ([]net.IP, error) {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		var err error
		ips, err = f.resolve(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("域名解析失败：%w", err)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("域名解析无结果")
	}
	for _, ip := range ips {
		if reason := f.blockedReason(ip); reason != "" {
			return nil, fmt.Errorf("%w：%s", ErrBlocked, reason)
		}
	}
	return ips, nil
}

func (f *Fetcher) safeDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("地址格式非法")
	}
	ips, err := f.resolveChecked(ctx, host)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	// 直接拨解析出的 IP，避免校验与拨号之间再次解析造成 DNS rebinding。
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func (f *Fetcher) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("重定向次数过多")
	}
	if _, err := parseTarget(req.URL.String()); err != nil {
		return err
	}
	if _, err := f.resolveChecked(req.Context(), req.URL.Hostname()); err != nil {
		return err
	}
	return nil
}

// blockedReason 返回不可访问原因，空字符串表示放行。
func (f *Fetcher) blockedReason(ip net.IP) string {
	if ip == nil {
		return "无效地址"
	}
	if v4 := ip.To4(); v4 != nil {
		if f.allowLoopback && v4[0] == 127 {
			return ""
		}
		switch {
		case v4[0] == 0:
			return "保留地址"
		case v4[0] == 10:
			return "私有地址"
		case v4[0] == 127:
			return "回环地址"
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127:
			return "运营商级 NAT 地址"
		case v4[0] == 169 && v4[1] == 254:
			return "链路本地/云元数据地址"
		case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
			return "私有地址"
		case v4[0] == 192 && v4[1] == 168:
			return "私有地址"
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19):
			return "基准测试保留地址"
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 0:
			return "保留地址"
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 2:
			return "文档保留地址"
		case v4[0] == 198 && v4[1] == 51 && v4[2] == 100:
			return "文档保留地址"
		case v4[0] == 203 && v4[1] == 0 && v4[2] == 113:
			return "文档保留地址"
		case v4[0] >= 224:
			return "组播/保留地址"
		}
		return ""
	}
	if ip.IsUnspecified() {
		return "未指定地址"
	}
	if ip.IsLoopback() {
		if f.allowLoopback {
			return ""
		}
		return "回环地址"
	}
	if ip.IsLinkLocalUnicast() {
		return "链路本地地址"
	}
	if ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return "组播地址"
	}
	// IPv6 ULA fc00::/7
	if len(ip) >= 1 && ip[0]&0xfe == 0xfc {
		return "唯一本地地址"
	}
	return ""
}

var (
	reLinkTag = regexp.MustCompile(`(?is)<link\s+[^>]*>`)
	reRel     = regexp.MustCompile(`(?is)rel\s*=\s*(?:"([^"]*)"|'([^']*)'|([^"'>\s]+))`)
	reHref    = regexp.MustCompile(`(?is)href\s*=\s*["']([^"']+)["']`)
)

func iconLinks(page []byte, base *url.URL) []string {
	var out []string
	seen := map[string]bool{}
	for _, tag := range reLinkTag.FindAll(page, 20) {
		rel := ""
		if m := reRel.FindSubmatch(tag); m != nil {
			for _, g := range m[1:] {
				if len(g) > 0 {
					rel = strings.ToLower(string(g))
					break
				}
			}
		}
		if !strings.Contains(rel, "icon") {
			continue
		}
		m := reHref.FindSubmatch(tag)
		if m == nil {
			continue
		}
		ref, err := url.Parse(strings.TrimSpace(string(m[1])))
		if err != nil {
			continue
		}
		abs := base.ResolveReference(ref)
		if abs.Scheme != "http" && abs.Scheme != "https" {
			continue
		}
		s := abs.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func readLimited(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败")
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("响应超过 %d 字节上限", limit)
	}
	return data, nil
}

func contentBase(ct string) string {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	return strings.ToLower(mt)
}
