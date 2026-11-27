package transfer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
)

var (
	reH3      = regexp.MustCompile(`(?is)<dt[^>]*>\s*<h3[^>]*>(.*?)</h3>`)
	reAnchor  = regexp.MustCompile(`(?is)<dt[^>]*>\s*<a\s+([^>]*)>(.*?)</a>`)
	reAttr    = regexp.MustCompile(`(?is)([a-z0-9_:-]+)\s*=\s*"([^"]*)"`)
	reDLClose = regexp.MustCompile(`(?is)</dl\s*>`)
	reDLOpen  = regexp.MustCompile(`(?is)<dl[^>]*>`)
)

var htmlWarnings = []Warning{
	{Code: "html_no_icon", Message: "浏览器导出的 HTML 通常不含图标；仅 Firefox 的 ICON=data: 或 LocalHub 导出的 LOCALHUB_ICON_* 可携带图标"},
	{Code: "html_permission_default", Message: "HTML 未携带 LOCALHUB_VISIBILITY 时，权限按默认值 public 处理"},
}

// ParseHTML 解析 Chrome/Edge/浏览器 Netscape 书签 HTML。
// 同时识别 LocalHub 导出的扩展属性（LOCALHUB_*）以支持往返。
func ParseHTML(data []byte) ([]Record, []Warning, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var recs []Record
	stack := []string{}
	unnamed := 0
	idx := 0
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// 先处理同一行内的多个标签：以 </DL> 与 <H3> 的出现顺序维护层级。
		if reDLClose.MatchString(strings.ToLower(line)) && len(stack) > 0 && reH3.MatchString(line) == false {
			// 关闭的是文件夹本身时，后续链接属于父层；根 DL 关闭时栈为空不处理。
			stack = stack[:len(stack)-1]
		}
		if m := reH3.FindStringSubmatch(line); m != nil {
			name := strings.TrimSpace(html.UnescapeString(stripTags(m[1])))
			if name == "" {
				name = "未命名文件夹"
				unnamed++
			}
			if len(stack) >= bookmark.MaxFolderDepth {
				return nil, nil, fmt.Errorf("文件夹层级超过 %d 级", bookmark.MaxFolderDepth)
			}
			stack = append(stack, name)
			continue
		}
		m := reAnchor.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		attrs := parseAttrs(m[1])
		href := strings.TrimSpace(html.UnescapeString(attrs["href"]))
		name := strings.TrimSpace(html.UnescapeString(stripTags(m[2])))
		rec := Record{
			Index:          idx,
			Name:           name,
			WANURL:         href,
			Description:    strings.TrimSpace(html.UnescapeString(attrs["localhub_description"])),
			Folder:         append([]string(nil), stack...),
			FolderProvided: true,
		}
		if rec.Name == "" {
			rec.Name = hostName(href)
		}
		rec.LANURL = strings.TrimSpace(html.UnescapeString(attrs["localhub_lan_url"]))
		if v := strings.TrimSpace(html.UnescapeString(attrs["localhub_visibility"])); v != "" {
			rec.Visibility = strings.ToLower(v)
		}
		if v := strings.TrimSpace(html.UnescapeString(attrs["localhub_open_mode"])); v != "" {
			rec.OpenMode = strings.ToLower(v)
		}
		if v := strings.TrimSpace(attrs["localhub_enabled"]); v != "" {
			switch strings.ToLower(v) {
			case "1", "true", "yes":
				b := true
				rec.Enabled = &b
			case "0", "false", "no":
				b := false
				rec.Enabled = &b
			}
		}
		rec.IconType = strings.ToLower(strings.TrimSpace(attrs["localhub_icon_type"]))
		rec.IconValue = strings.TrimSpace(html.UnescapeString(attrs["localhub_icon_value"]))
		if rec.IconType == "" {
			if iconAttr := strings.TrimSpace(attrs["icon"]); strings.HasPrefix(strings.ToLower(iconAttr), "data:image/") {
				if payload, mimeType, ok := parseDataURI(iconAttr); ok {
					rec.IconData, rec.IconMime, rec.IconType = payload, mimeType, "file"
				}
			}
		}
		recs = append(recs, rec)
		idx++
	}
	if len(recs) == 0 {
		return nil, nil, fmt.Errorf("未在 HTML 中找到书签（需为 Netscape <DT><A HREF> 格式）")
	}
	if len(recs) > MaxImportItems {
		return nil, nil, fmt.Errorf("书签数量超过 %d 条上限", MaxImportItems)
	}
	warns := append([]Warning(nil), htmlWarnings...)
	if unnamed > 0 {
		warns = append(warns, Warning{Code: "folder_unnamed", Message: fmt.Sprintf("%d 个文件夹名为空，已按“未命名文件夹”保存", unnamed)})
	}
	return recs, warns, nil
}

func parseAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, m := range reAttr.FindAllStringSubmatch(raw, -1) {
		out[strings.ToLower(m[1])] = m[2]
	}
	return out
}

func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func hostName(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

func parseDataURI(v string) ([]byte, string, bool) {
	if !strings.HasPrefix(strings.ToLower(v), "data:image/") {
		return nil, "", false
	}
	comma := strings.Index(v, ",")
	if comma < 0 {
		return nil, "", false
	}
	meta := v[5:comma]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return nil, "", false
	}
	mimeType := strings.Split(meta, ";")[0]
	payload, err := base64.StdEncoding.DecodeString(v[comma+1:])
	if err != nil {
		return nil, "", false
	}
	return payload, strings.ToLower(mimeType), true
}

// BuildHTML 生成浏览器可导入的 Netscape 书签 HTML。
// 按书签已持久化的 Folder 路径生成嵌套 <H3>/<DL>；不引入分类或标签体系。
func BuildHTML(bs []bookmark.Bookmark, now time.Time) []byte {
	ts := now.Unix()
	root := &htmlFolder{index: map[string]*htmlFolder{}}
	for _, bm := range bs {
		node := root
		for _, name := range bm.Folder {
			child := node.index[name]
			if child == nil {
				child = &htmlFolder{name: name, index: map[string]*htmlFolder{}}
				node.index[name] = child
				node.children = append(node.children, child)
			}
			node = child
		}
		node.items = append(node.items, bm)
	}
	var b bytes.Buffer
	b.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
	b.WriteString("<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
	b.WriteString("<TITLE>Bookmarks</TITLE>\n<H1>Bookmarks</H1>\n<DL><p>\n")

	writeBookmark := func(bm bookmark.Bookmark, depth int) {
		indent := strings.Repeat("    ", depth)
		href := bm.WANURL
		if href == "" {
			href = bm.LANURL
		}
		attrs := []string{
			fmt.Sprintf("HREF=\"%s\"", html.EscapeString(href)),
			fmt.Sprintf("ADD_DATE=\"%d\"", ts),
			fmt.Sprintf("LOCALHUB_VISIBILITY=\"%s\"", html.EscapeString(bm.Visibility)),
			fmt.Sprintf("LOCALHUB_ENABLED=\"%s\"", boolAttr(bm.Enabled)),
			fmt.Sprintf("LOCALHUB_OPEN_MODE=\"%s\"", html.EscapeString(bm.OpenMode)),
		}
		if bm.LANURL != "" {
			attrs = append(attrs, fmt.Sprintf("LOCALHUB_LAN_URL=\"%s\"", html.EscapeString(bm.LANURL)))
		}
		if bm.Description != "" {
			attrs = append(attrs, fmt.Sprintf("LOCALHUB_DESCRIPTION=\"%s\"", html.EscapeString(bm.Description)))
		}
		if bm.IconType == bookmark.IconEmoji || bm.IconType == bookmark.IconText || bm.IconType == bookmark.IconURL {
			attrs = append(attrs,
				fmt.Sprintf("LOCALHUB_ICON_TYPE=\"%s\"", html.EscapeString(bm.IconType)),
				fmt.Sprintf("LOCALHUB_ICON_VALUE=\"%s\"", html.EscapeString(bm.IconValue)),
			)
		}
		fmt.Fprintf(&b, "%s<DT><A %s>%s</A>\n", indent, strings.Join(attrs, " "), html.EscapeString(bm.Name))
	}
	var walk func(node *htmlFolder, depth int)
	walk = func(node *htmlFolder, depth int) {
		indent := strings.Repeat("    ", depth)
		for _, child := range node.children {
			fmt.Fprintf(&b, "%s<DT><H3 ADD_DATE=\"%d\" LAST_MODIFIED=\"%d\">%s</H3>\n", indent, ts, ts, html.EscapeString(child.name))
			fmt.Fprintf(&b, "%s<DL><p>\n", indent)
			walk(child, depth+1)
			fmt.Fprintf(&b, "%s</DL><p>\n", indent)
		}
		for _, bm := range node.items {
			writeBookmark(bm, depth+1)
		}
	}
	walk(root, 0)
	b.WriteString("</DL><p>\n")
	return b.Bytes()
}

type htmlFolder struct {
	name     string
	children []*htmlFolder
	index    map[string]*htmlFolder
	items    []bookmark.Bookmark
}

func boolAttr(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
