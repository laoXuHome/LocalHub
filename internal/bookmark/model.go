package bookmark

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// 权限类型：公开 / 登录后可用 / 仅个人可见；第四种“禁用”由 Enabled=false 表达。
const (
	VisibilityPublic        = "public"
	VisibilityAuthenticated = "authenticated"
	VisibilityPrivate       = "private"

	VisibilityDisabled = "disabled" // 仅用于对外输出“有效权限”，不作为可写入值
)

// 打开方式。
const (
	OpenModeNewTab  = "new_tab"
	OpenModeSelf    = "self"
	OpenModeCurrent = "current_tab"
)

// 图标类型（P2.1 只存信息，不做抓取）。
const (
	IconNone    = "none"
	IconFavicon = "favicon"
	IconURL     = "url"
	IconEmoji   = "emoji"
	IconText    = "text"
	// IconFile 表示图标已保存为本地文件（P2.2），只能由图标上传/抓取接口设置。
	IconFile = "file"
)

// 字段长度上限。
const (
	MaxNameLen        = 64
	MaxURLLen         = 2048
	MaxDescriptionLen = 200
	MaxIconValueLen   = 512
	MaxDockerLen      = 128
	MaxBatchSize      = 200
)

// Bookmark 是书签记录。
type Bookmark struct {
	ID          int64
	Name        string
	LANURL      string
	WANURL      string
	Description string
	IconType    string
	IconValue   string
	// IconFile 是 <dataDir>/icons 下的内部文件名，不含用户输入。
	IconFile        string
	IconMime        string
	IconSize        int64
	IconUpdatedAt   time.Time
	Enabled         bool
	Visibility      string
	OpenMode        string
	DockerContainer string
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// EffectiveVisibility 返回对外生效的权限：禁用优先。
func (b Bookmark) EffectiveVisibility() string {
	if !b.Enabled {
		return VisibilityDisabled
	}
	return b.Visibility
}

// VisibleTo 判断某类调用者是否可读该书签。
// guest：只能读公开；admin：可读公开、登录后可用、仅个人可见；禁用对所有人不可见（管理列表除外）。
func (b Bookmark) VisibleTo(admin bool) bool {
	switch b.EffectiveVisibility() {
	case VisibilityPublic:
		return true
	case VisibilityAuthenticated, VisibilityPrivate:
		return admin
	default:
		return false
	}
}

// Input 是新增/编辑的请求体（PATCH 语义：指针字段表示“本次是否提供”）。
type Input struct {
	Name            *string `json:"name"`
	LANURL          *string `json:"lan_url"`
	WANURL          *string `json:"wan_url"`
	Description     *string `json:"description"`
	IconType        *string `json:"icon_type"`
	IconValue       *string `json:"icon_value"`
	Enabled         *bool   `json:"enabled"`
	Visibility      *string `json:"visibility"`
	OpenMode        *string `json:"open_mode"`
	DockerContainer *string `json:"docker_container"`
	SortOrder       *int    `json:"sort_order"`
}

// ErrInvalid 表示输入不合法。
var ErrInvalid = errors.New("invalid bookmark")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w：%s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Build 把输入合并到基础记录上并校验。base 为 nil 表示新增。
func Build(base *Bookmark, in Input) (*Bookmark, error) {
	b := &Bookmark{
		IconType:   IconNone,
		Visibility: VisibilityPublic,
		OpenMode:   OpenModeNewTab,
		Enabled:    true,
	}
	if base != nil {
		*b = *base
	}

	if in.Name != nil {
		b.Name = strings.TrimSpace(*in.Name)
	}
	if in.LANURL != nil {
		b.LANURL = strings.TrimSpace(*in.LANURL)
	}
	if in.WANURL != nil {
		b.WANURL = strings.TrimSpace(*in.WANURL)
	}
	if in.Description != nil {
		b.Description = strings.TrimSpace(*in.Description)
	}
	if in.IconType != nil {
		v := strings.ToLower(strings.TrimSpace(*in.IconType))
		if v == IconFile {
			return nil, invalid("本地图标请通过图标上传或抓取接口设置")
		}
		if b.IconType == IconFile && v != IconFile {
			// 从本地图标切换为其他图标类型时，清理本地文件元数据，旧文件由处理器删除。
			b.IconFile, b.IconMime, b.IconSize, b.IconUpdatedAt = "", "", 0, time.Time{}
		}
		b.IconType = v
	}
	if in.IconValue != nil {
		b.IconValue = strings.TrimSpace(*in.IconValue)
	}
	if in.Enabled != nil {
		b.Enabled = *in.Enabled
	}
	if in.Visibility != nil {
		b.Visibility = strings.ToLower(strings.TrimSpace(*in.Visibility))
	}
	if in.OpenMode != nil {
		b.OpenMode = strings.ToLower(strings.TrimSpace(*in.OpenMode))
	}
	if in.DockerContainer != nil {
		b.DockerContainer = strings.TrimSpace(*in.DockerContainer)
	}
	if in.SortOrder != nil {
		b.SortOrder = *in.SortOrder
	}

	if err := Validate(b); err != nil {
		return nil, err
	}
	return b, nil
}

// Validate 校验字段合法性（长度、枚举、URL 协议）。
func Validate(b *Bookmark) error {
	if b.Name == "" {
		return invalid("名称不能为空")
	}
	if len([]byte(b.Name)) > MaxNameLen {
		return invalid("名称最长 %d 字节", MaxNameLen)
	}
	if len([]byte(b.LANURL)) > MaxURLLen || len([]byte(b.WANURL)) > MaxURLLen {
		return invalid("URL 最长 %d 字节", MaxURLLen)
	}
	if b.LANURL == "" && b.WANURL == "" {
		return invalid("至少需要填写 LAN URL 或 WAN URL")
	}
	for _, u := range []string{b.LANURL, b.WANURL} {
		if u == "" {
			continue
		}
		if err := checkHTTPURL(u); err != nil {
			return err
		}
	}
	if len([]byte(b.Description)) > MaxDescriptionLen {
		return invalid("描述最长 %d 字节", MaxDescriptionLen)
	}
	switch b.IconType {
	case IconNone, IconFavicon, IconEmoji, IconText:
		if b.IconType == IconNone && b.IconValue != "" {
			return invalid("icon_type=none 时不应携带 icon_value")
		}
	case IconFile:
		if b.IconFile == "" {
			return invalid("icon_type=file 但缺少图标文件")
		}
		if b.IconValue != "" {
			return invalid("icon_type=file 时不应携带 icon_value")
		}
	case IconURL:
		if b.IconValue == "" {
			return invalid("icon_type=url 时必须提供 icon_value")
		}
		if err := checkHTTPURL(b.IconValue); err != nil {
			return err
		}
	default:
		return invalid("icon_type=%q 非法，可选 none/favicon/url/emoji/text", b.IconType)
	}
	if len([]byte(b.IconValue)) > MaxIconValueLen {
		return invalid("icon_value 最长 %d 字节", MaxIconValueLen)
	}
	switch b.Visibility {
	case VisibilityPublic, VisibilityAuthenticated, VisibilityPrivate:
	default:
		return invalid("visibility=%q 非法，可选 public/authenticated/private（禁用请用 enabled=false）", b.Visibility)
	}
	switch b.OpenMode {
	case OpenModeNewTab, OpenModeSelf:
	case OpenModeCurrent:
		b.OpenMode = OpenModeSelf
	default:
		return invalid("open_mode=%q 非法，可选 new_tab/self", b.OpenMode)
	}
	if len([]byte(b.DockerContainer)) > MaxDockerLen {
		return invalid("docker_container 最长 %d 字节", MaxDockerLen)
	}
	if strings.ContainsAny(b.DockerContainer, "\r\n") {
		return invalid("docker_container 不能包含换行")
	}
	if b.SortOrder < 0 {
		return invalid("sort_order 不能为负数")
	}
	return nil
}

func checkHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return invalid("URL 无法解析：%s", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return invalid("URL 协议只允许 http/https：%s", raw)
	}
	if u.Host == "" {
		return invalid("URL 缺少主机名：%s", raw)
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return invalid("URL 不能包含空白字符")
	}
	return nil
}
