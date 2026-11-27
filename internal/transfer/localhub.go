package transfer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
	"github.com/laoXuHome/LocalHub/internal/icon"
)

type exportDoc struct {
	Format      string           `json:"format"`
	Version     int              `json:"version"`
	GeneratedAt string           `json:"generated_at"`
	Count       int              `json:"count"`
	Bookmarks   []exportBookmark `json:"bookmarks"`
}

type exportBookmark struct {
	Name            string    `json:"name"`
	LANURL          string    `json:"lan_url"`
	WANURL          string    `json:"wan_url"`
	Description     string    `json:"description"`
	IconType        string    `json:"icon_type"`
	IconValue       string    `json:"icon_value"`
	IconMime        string    `json:"icon_mime,omitempty"`
	IconSize        int64     `json:"icon_size,omitempty"`
	IconDataBase64  string    `json:"icon_data_base64,omitempty"`
	IconMissing     bool      `json:"icon_missing,omitempty"`
	Enabled         bool      `json:"enabled"`
	Visibility      string    `json:"visibility"`
	OpenMode        string    `json:"open_mode"`
	DockerContainer string    `json:"docker_container,omitempty"`
	FolderPath      *[]string `json:"folder_path,omitempty"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

// BuildLocalHubJSON 生成完整迁移 JSON。图标以 base64 内嵌，不包含服务器本地路径。
// readIcon 仅读取文件，不会修改数据库。
func BuildLocalHubJSON(bs []bookmark.Bookmark, readIcon func(file string) ([]byte, error), now time.Time) ([]byte, []Warning) {
	doc := exportDoc{
		Format:      JSONFormatName,
		Version:     JSONFormatVersion,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Count:       len(bs),
		Bookmarks:   make([]exportBookmark, 0, len(bs)),
	}
	warns := []Warning{}
	for _, b := range bs {
		folder := make([]string, len(b.Folder))
		copy(folder, b.Folder)
		item := exportBookmark{
			Name:            b.Name,
			LANURL:          b.LANURL,
			WANURL:          b.WANURL,
			Description:     b.Description,
			IconType:        b.IconType,
			IconValue:       b.IconValue,
			IconMime:        b.IconMime,
			IconSize:        b.IconSize,
			Enabled:         b.Enabled,
			Visibility:      b.Visibility,
			OpenMode:        b.OpenMode,
			DockerContainer: b.DockerContainer,
			FolderPath:      &folder,
			SortOrder:       b.SortOrder,
			CreatedAt:       b.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:       b.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if b.IconFile != "" {
			data, err := readIcon(b.IconFile)
			if err != nil {
				item.IconMissing = true
				warns = append(warns, Warning{Code: "icon_missing", Message: fmt.Sprintf("书签 %q 的本地图标文件不可读，导出中不含图标数据", b.Name)})
			} else {
				item.IconDataBase64 = base64.StdEncoding.EncodeToString(data)
				item.IconSize = int64(len(data))
				if item.IconMime == "" {
					if mimeType, _, verr := icon.Validate(data); verr == nil {
						item.IconMime = mimeType
					}
				}
			}
		}
		doc.Bookmarks = append(doc.Bookmarks, item)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, append(warns, Warning{Code: "encode_failed", Message: "生成 JSON 失败"})
	}
	return append(data, '\n'), warns
}

// ParseLocalHubJSON 解析 LocalHub JSON（版本 1/2；v1 无 folder_path）。
func ParseLocalHubJSON(data []byte) ([]Record, []Warning, error) {
	var doc struct {
		Format    string            `json:"format"`
		Version   int               `json:"version"`
		Bookmarks []json.RawMessage `json:"bookmarks"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		// 允许未知顶层字段（向前兼容读取），退回宽松解析。
		if err2 := json.Unmarshal(data, &doc); err2 != nil {
			return nil, nil, fmt.Errorf("JSON 解析失败：%v", err2)
		}
	}
	if doc.Format != JSONFormatName {
		return nil, nil, fmt.Errorf("不是 LocalHub 导出文件（format=%q）", doc.Format)
	}
	if doc.Version < MinJSONFormatVersion || doc.Version > JSONFormatVersion {
		return nil, nil, fmt.Errorf("不支持的 LocalHub JSON 版本 %d（当前支持 %d-%d）", doc.Version, MinJSONFormatVersion, JSONFormatVersion)
	}
	if len(doc.Bookmarks) == 0 {
		return nil, nil, fmt.Errorf("文件中没有书签")
	}
	if len(doc.Bookmarks) > MaxImportItems {
		return nil, nil, fmt.Errorf("书签数量超过 %d 条上限", MaxImportItems)
	}
	recs := make([]Record, 0, len(doc.Bookmarks))
	for i, raw := range doc.Bookmarks {
		var item exportBookmark
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, nil, fmt.Errorf("第 %d 条书签 JSON 非法：%v", i+1, err)
		}
		rec := Record{
			Index:       i,
			Name:        item.Name,
			LANURL:      item.LANURL,
			WANURL:      item.WANURL,
			Description: item.Description,
			IconType:    item.IconType,
			IconValue:   item.IconValue,
			Visibility:  item.Visibility,
			OpenMode:    item.OpenMode,
			Enabled:     boolPtr(item.Enabled),
		}
		if item.FolderPath != nil {
			rec.Folder = append([]string(nil), *item.FolderPath...)
			rec.FolderProvided = true
		}
		if item.IconDataBase64 == "" && rec.IconType == bookmark.IconFile {
			// 导出时图标文件缺失（icon_missing）：保留书签，不保留本地图标标记。
			rec.IconType = bookmark.IconNone
		}
		if item.IconDataBase64 != "" {
			if len(item.IconDataBase64) > base64.StdEncoding.EncodedLen(icon.MaxBytes)+4 {
				return nil, nil, fmt.Errorf("第 %d 条书签的内嵌图标超过大小上限", i+1)
			}
			payload, err := base64.StdEncoding.DecodeString(item.IconDataBase64)
			if err != nil {
				return nil, nil, fmt.Errorf("第 %d 条书签的内嵌图标 base64 非法", i+1)
			}
			rec.IconData = payload
			rec.IconMime = item.IconMime
			rec.IconType = bookmark.IconFile
		}
		recs = append(recs, rec)
	}
	return recs, []Warning{}, nil
}

func boolPtr(v bool) *bool { return &v }
