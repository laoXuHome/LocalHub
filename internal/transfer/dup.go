package transfer

import (
	"net/url"
	"strings"
)

// NormalizeURL 用于重复判断：仅归一会影响同一性的差异（协议/主机大小写、末尾斜杠、默认端口），
// 保留路径与查询参数。
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimSuffix(raw, "/"))
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "http" && strings.HasSuffix(u.Host, ":80")) ||
		(u.Scheme == "https" && strings.HasSuffix(u.Host, ":443")) {
		if h, err := url.Parse("//" + u.Host); err == nil {
			u.Host = strings.TrimSuffix(h.Host, ":80")
			u.Host = strings.TrimSuffix(u.Host, ":443")
		}
	}
	if u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	} else {
		u.Path = ""
	}
	return u.String()
}

// URLKeys 返回一条记录用于重复判断的非空 URL 键。
func URLKeys(lan, wan string) []string {
	keys := make([]string, 0, 2)
	for _, raw := range []string{lan, wan} {
		if k := NormalizeURL(raw); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}
