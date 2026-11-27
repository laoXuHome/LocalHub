package transfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
)

var sunPanelWarnings = []Warning{
	{Code: "sunpanel_partial", Message: "Sun-Panel 仅映射名称、URL、图标、描述、打开方式与文件夹层级；权限、排序不写入数据库"},
}

// ParseSunPanel 解析 Sun-Panel 风格的 JSON（items/data/list/apps/sites 数组，支持 children 嵌套）。
func ParseSunPanel(data []byte) ([]Record, []Warning, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, nil, fmt.Errorf("JSON 解析失败：%v", err)
	}
	var recs []Record
	err := walkSunPanel(root, nil, &recs, 0)
	if err != nil {
		return nil, nil, err
	}
	if len(recs) == 0 {
		return nil, nil, fmt.Errorf("未识别到 Sun-Panel 书签（需包含 name/url 字段，或 items/data 数组）")
	}
	if len(recs) > MaxImportItems {
		return nil, nil, fmt.Errorf("书签数量超过 %d 条上限", MaxImportItems)
	}
	// 重新编号，保证索引稳定。
	for i := range recs {
		recs[i].Index = i
	}
	return recs, sunPanelWarnings, nil
}

func walkSunPanel(v any, folder []string, recs *[]Record, depth int) error {
	if depth > 12 {
		return fmt.Errorf("JSON 嵌套层级过深")
	}
	switch node := v.(type) {
	case []any:
		for _, child := range node {
			if err := walkSunPanel(child, folder, recs, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		// 容器键：items/data/list/apps/sites/bookmarks/groups/children
		name := firstString(node, "name", "title", "label")
		url := firstString(node, "url", "href", "link", "wanUrl", "wan_url")
		lan := firstString(node, "lanUrl", "lan_url", "internalUrl", "internal_url")
		folderName := firstString(node, "groupName", "group_name", "folderName", "folder_name")
		hasContainer := false
		for _, key := range []string{"items", "data", "list", "apps", "sites", "bookmarks", "groups", "children"} {
			if _, ok := node[key]; ok {
				hasContainer = true
				break
			}
		}
		// 仅有名称、无 URL 且包含子节点的对象视为文件夹分组。
		if folderName == "" && hasContainer && url == "" && lan == "" {
			folderName = name
		}
		for _, key := range []string{"items", "data", "list", "apps", "sites", "bookmarks", "groups", "children"} {
			if child, ok := node[key]; ok {
				if len(folder) >= bookmark.MaxFolderDepth {
					return fmt.Errorf("Sun-Panel 文件夹层级超过 %d 级", bookmark.MaxFolderDepth)
				}
				sub := appendFolder(folder, folderName)
				if err := walkSunPanel(child, sub, recs, depth+1); err != nil {
					return err
				}
			}
		}
		if url == "" && lan == "" {
			// 仅有名称的节点视为文件夹/分组，不产生记录。
			return nil
		}
		r := Record{
			Index:          len(*recs),
			Name:           name,
			LANURL:         lan,
			WANURL:         url,
			Description:    firstString(node, "description", "desc", "remark"),
			Folder:         append([]string(nil), folder...),
			FolderProvided: true,
		}
		switch strings.ToLower(firstString(node, "target", "openType", "open_type")) {
		case "_self", "self", "current", "current_tab":
			r.OpenMode = "self"
		case "_blank", "new_tab", "blank":
			r.OpenMode = "new_tab"
		}
		if iconVal := firstString(node, "icon", "logo", "iconUrl", "icon_url"); iconVal != "" {
			switch {
			case strings.HasPrefix(strings.ToLower(iconVal), "http://"), strings.HasPrefix(strings.ToLower(iconVal), "https://"):
				r.IconType, r.IconValue = "url", iconVal
			case len([]rune(iconVal)) <= 4 && !strings.Contains(iconVal, "/"):
				r.IconType, r.IconValue = "emoji", iconVal
			default:
				r.IconType, r.IconValue = "text", iconVal
			}
		}
		*recs = append(*recs, r)
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func appendFolder(base []string, name string) []string {
	name = strings.TrimSpace(name)
	out := append([]string(nil), base...)
	if name == "" {
		return out
	}
	return append(out, name)
}
