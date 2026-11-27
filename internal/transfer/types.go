// Package transfer 实现书签导入导出：格式识别、解析、预览、导出。
// 不包含任何前端或分类/标签体系。
package transfer

import "github.com/laoXuHome/LocalHub/internal/bookmark"

// 支持的导入格式。
const (
	FormatAuto     = "auto"
	FormatHTML     = "html"
	FormatSunPanel = "sunpanel"
	FormatLocalHub = "localhub"
)

// 单条导入限制。
const (
	MaxImportBytes = 10 << 20
	MaxImportItems = 5000
	// JSONFormatName / JSONFormatVersion 是 LocalHub JSON 的版本标识。
	JSONFormatName    = "localhub-bookmarks"
	JSONFormatVersion = 1
)

// Warning 是整体级别的导入警告（字段不支持、层级不保留等）。
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Record 是一条解析后的候选书签，来自任意来源。
type Record struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	LANURL      string `json:"lan_url"`
	WANURL      string `json:"wan_url"`
	Description string `json:"description"`
	IconType    string `json:"icon_type"`
	IconValue   string `json:"icon_value"`
	// IconData 是内嵌图标字节（LocalHub JSON 迁移用）；不参与 JSON 预览输出。
	IconData []byte `json:"-"`
	IconMime string `json:"-"`
	Enabled  *bool  `json:"-"`
	// Visibility/OpenMode 为空表示来源未提供，导入时使用默认值/保留原值。
	Visibility string `json:"visibility"`
	OpenMode   string `json:"open_mode"`
	Folder     string `json:"folder"`
}

// Item 是预览中的单条结果。
type Item struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	LANURL     string `json:"lan_url"`
	WANURL     string `json:"wan_url"`
	Folder     string `json:"folder"`
	Status     string `json:"status"` // new / duplicate / invalid
	Reason     string `json:"reason,omitempty"`
	ExistingID int64  `json:"existing_id,omitempty"`
	Duplicate  string `json:"duplicate_kind,omitempty"` // url / batch
}

// Preview 是预览响应。
type Preview struct {
	Format     string    `json:"format"`
	Source     string    `json:"source"`
	Total      int       `json:"total"`
	Valid      int       `json:"valid"`
	Invalid    int       `json:"invalid"`
	Duplicates int       `json:"duplicates"`
	New        int       `json:"new"`
	Warnings   []Warning `json:"warnings"`
	Items      []Item    `json:"items"`
}

// ImportResult 是执行结果中的单条记录。
type ImportResult struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Status     string `json:"status"` // created / overwritten / skipped / failed
	Reason     string `json:"reason,omitempty"`
	BookmarkID int64  `json:"bookmark_id,omitempty"`
}

// Summary 是执行汇总。
type Summary struct {
	Created     int            `json:"created"`
	Overwritten int            `json:"overwritten"`
	Skipped     int            `json:"skipped"`
	Failed      int            `json:"failed"`
	Total       int            `json:"total"`
	Icons       int            `json:"icons_imported"`
	IconFailed  int            `json:"icons_failed"`
	Items       []ImportResult `json:"items"`
	Warnings    []Warning      `json:"warnings"`
}

// 用于把解析结果统一成 bookmork 可校验的输入。
func (r Record) ToInput() bookmark.Input {
	s := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	in := bookmark.Input{
		Name:        s(r.Name),
		LANURL:      s(r.LANURL),
		WANURL:      s(r.WANURL),
		Description: s(r.Description),
		IconType:    s(r.IconType),
		IconValue:   s(r.IconValue),
		Visibility:  s(r.Visibility),
		OpenMode:    s(r.OpenMode),
	}
	if r.Enabled != nil {
		in.Enabled = r.Enabled
	}
	return in
}
