package transfer

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
)

const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

var testPNG = func() []byte {
	dec, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		panic(err)
	}
	return dec
}()

func TestNormalizeURL(t *testing.T) {
	// 11 个表驱动子用例
	cases := []struct{ name, raw, want string }{
		{"空值", "  ", ""},
		{"大小写与末尾斜杠", "HTTPS://Example.COM/Path/", "https://example.com/Path"},
		{"根路径归一", "http://example.com/", "http://example.com"},
		{"默认端口去除", "https://example.com:443/a", "https://example.com/a"},
		{"非默认端口保留", "https://example.com:8443/a", "https://example.com:8443/a"},
		{"查询参数保留", "https://example.com/a?x=1", "https://example.com/a?x=1"},
		{"片段保留", "https://example.com/a#top", "https://example.com/a#top"},
		{"非 URL 文本小写", "FileServer/", "fileserver"},
		{"http 默认端口", "http://example.com:80/", "http://example.com"},
		{"LAN 地址", "http://192.168.1.10:8080/", "http://192.168.1.10:8080"},
		{"仅主机", "https://example.com", "https://example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeURL(tc.raw); got != tc.want {
				t.Fatalf("NormalizeURL(%q)=%q，want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestURLKeys(t *testing.T) {
	if got := URLKeys("", ""); len(got) != 0 {
		t.Fatalf("空 URL 不应产生键：%v", got)
	}
	keys := URLKeys("http://192.168.1.10:8080/", "https://example.com/")
	if len(keys) != 2 || keys[0] != "http://192.168.1.10:8080" || keys[1] != "https://example.com" {
		t.Fatalf("URLKeys 结果错误：%v", keys)
	}
}

func TestDetectFormat(t *testing.T) {
	cases := []struct{ name, data, format, want string }{
		{"HTML doctype", "<!DOCTYPE NETSCAPE-Bookmark-file-1><DL><DT><A HREF=\"https://a\">A</A></DL>", FormatAuto, FormatHTML},
		{"HTML dl 片段", "<dl><dt><a href=\"https://a\">A</a></dl>", FormatAuto, FormatHTML},
		{"LocalHub JSON", `{"format":"localhub-bookmarks","version":1,"bookmarks":[]}`, FormatAuto, FormatLocalHub},
		{"Sun-Panel JSON", `{"items":[{"name":"A","url":"https://a"}]}`, FormatAuto, FormatSunPanel},
		{"显式 HTML", "<DL></DL>", FormatHTML, FormatHTML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Detect([]byte(tc.data), tc.format)
			if err != nil {
				t.Fatalf("Detect 失败：%v", err)
			}
			if got != tc.want {
				t.Fatalf("Detect=%q，want %q", got, tc.want)
			}
		})
	}
	if _, err := Detect(nil, FormatAuto); err == nil {
		t.Fatalf("空文件应被拒绝")
	}
	if _, err := Detect([]byte(strings.Repeat("x", MaxImportBytes+1)), FormatAuto); err == nil {
		t.Fatalf("超大文件应被拒绝")
	}
}

const chromeHTML = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3 ADD_DATE="1700000000">工作</H3>
    <DL><p>
        <DT><H3 ADD_DATE="1700000003">项目A</H3>
        <DL><p>
            <DT><A HREF="https://example.com/" ADD_DATE="1700000001">Example &amp; One</A>
        </DL><p>
        <DT><A HREF="https://second.example.com/path" ADD_DATE="1700000002">Second</A>
    </DL><p>
</DL><p>`

const edgeHTML = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3 ADD_DATE="1700000010">Edge 工作</H3>
    <DL><p>
        <DT><H3 ADD_DATE="1700000013">Edge 项目B</H3>
        <DL><p>
            <DT><A HREF="https://edge.example.com/" ADD_DATE="1700000011">Edge One</A>
        </DL><p>
        <DT><A HREF="https://edge-second.example.com/path" ADD_DATE="1700000012">Edge Second</A>
    </DL><p>
</DL><p>`

func TestParseChromeAndEdgeHTML(t *testing.T) {
	cases := []struct {
		name       string
		data       string
		firstURL   string
		firstPath  []string
		secondPath []string
	}{
		{"Chrome", chromeHTML, "https://example.com/", []string{"工作", "项目A"}, []string{"工作"}},
		{"Edge", edgeHTML, "https://edge.example.com/", []string{"Edge 工作", "Edge 项目B"}, []string{"Edge 工作"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recs, warns, err := ParseHTML([]byte(tc.data))
			if err != nil {
				t.Fatalf("解析失败：%v", err)
			}
			if len(recs) != 2 {
				t.Fatalf("应解析出 2 条，got %d", len(recs))
			}
			if recs[0].WANURL != tc.firstURL {
				t.Fatalf("首条解析错误：%+v", recs[0])
			}
			if !equalPath(recs[0].Folder, tc.firstPath) || !equalPath(recs[1].Folder, tc.secondPath) {
				t.Fatalf("多级文件夹层级未识别：%+v", recs)
			}
			for _, w := range warns {
				if w.Code == "folder_not_persisted" {
					t.Fatalf("层级已持久化，不应再提示不写库：%+v", warns)
				}
			}
		})
	}
}

func TestParseHTMLLocalHubAttrs(t *testing.T) {
	html := `<DL><DT><A HREF="https://wan.example.com/app" LOCALHUB_LAN_URL="http://192.168.1.10:8080" LOCALHUB_VISIBILITY="private" LOCALHUB_ENABLED="false" LOCALHUB_OPEN_MODE="self" LOCALHUB_DESCRIPTION="描述" LOCALHUB_ICON_TYPE="emoji" LOCALHUB_ICON_VALUE="🚀">应用</A></DL>`
	recs, _, err := ParseHTML([]byte(html))
	if err != nil || len(recs) != 1 {
		t.Fatalf("解析失败：recs=%v err=%v", recs, err)
	}
	r := recs[0]
	if r.LANURL != "http://192.168.1.10:8080" || r.Visibility != "private" || r.OpenMode != "self" || r.Description != "描述" || r.IconType != "emoji" || r.IconValue != "🚀" {
		t.Fatalf("LocalHub 属性映射错误：%+v", r)
	}
	if r.Enabled == nil || *r.Enabled {
		t.Fatalf("LOCALHUB_ENABLED=false 应映射为禁用：%+v", r.Enabled)
	}
}

func TestBuildHTMLRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	list := []bookmark.Bookmark{
		{ID: 1, Name: "公开站", Folder: []string{"工作", "项目A"}, WANURL: "https://pub.example.com/", LANURL: "http://192.168.1.2:9000", Enabled: true, Visibility: bookmark.VisibilityPublic, OpenMode: bookmark.OpenModeNewTab, Description: "公开", IconType: bookmark.IconEmoji, IconValue: "🌐"},
		{ID: 2, Name: "私有站", Folder: []string{"工作"}, WANURL: "https://priv.example.com/", Enabled: false, Visibility: bookmark.VisibilityPrivate, OpenMode: bookmark.OpenModeSelf},
		{ID: 3, Name: "根站", WANURL: "https://root.example.com/", Enabled: true, Visibility: bookmark.VisibilityPublic, OpenMode: bookmark.OpenModeNewTab},
	}
	data := BuildHTML(list, now)
	text := string(data)
	for _, want := range []string{"<!DOCTYPE NETSCAPE-Bookmark-file-1>", "<H3", "工作", "项目A", "LOCALHUB_VISIBILITY=\"public\"", "LOCALHUB_ENABLED=\"0\"", "LOCALHUB_ICON_TYPE=\"emoji\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("HTML 缺少 %q：\n%s", want, text)
		}
	}
	if strings.Contains(text, ">LocalHub</H3>") {
		t.Fatalf("不应再插入额外的 LocalHub 根文件夹：\n%s", text)
	}
	recs, _, err := ParseHTML(data)
	if err != nil || len(recs) != 3 {
		t.Fatalf("导出的 HTML 无法回读：recs=%v err=%v", recs, err)
	}
	if !equalPath(recs[0].Folder, []string{"工作", "项目A"}) || !equalPath(recs[1].Folder, []string{"工作"}) || !equalPath(recs[2].Folder, nil) {
		t.Fatalf("HTML 层级回读错误：%+v", recs)
	}
	if recs[0].WANURL != "https://pub.example.com/" || recs[1].Enabled == nil || *recs[1].Enabled {
		t.Fatalf("回读数据不一致：%+v", recs)
	}
}

func TestParseSunPanelNested(t *testing.T) {
	raw := `{"data":[{"groupName":"工具","apps":[
		{"name":"面板","url":"https://panel.example.com","lanUrl":"http://10.0.0.2:3000","icon":"🚀","target":"_blank","description":"面板"},
		{"title":"文档","href":"https://doc.example.com","logo":"https://doc.example.com/logo.png","openType":"_self"}
	]}]}`
	recs, warns, err := ParseSunPanel([]byte(raw))
	if err != nil || len(recs) != 2 {
		t.Fatalf("解析失败：recs=%v err=%v", recs, err)
	}
	if !equalPath(recs[0].Folder, []string{"工具"}) || recs[0].OpenMode != "new_tab" || recs[0].IconType != "emoji" || recs[0].LANURL != "http://10.0.0.2:3000" {
		t.Fatalf("首条映射错误：%+v", recs[0])
	}
	if recs[1].IconType != "url" || recs[1].OpenMode != "self" {
		t.Fatalf("次条映射错误：%+v", recs[1])
	}
	if len(warns) == 0 || warns[0].Code != "sunpanel_partial" {
		t.Fatalf("应给出部分字段不支持的警告：%+v", warns)
	}
}

func TestSunPanelRejectsInvalid(t *testing.T) {
	if _, _, err := ParseSunPanel([]byte("not-json")); err == nil {
		t.Fatalf("非法 JSON 应被拒绝")
	}
	if _, _, err := ParseSunPanel([]byte(`{"items":[]}`)); err == nil {
		t.Fatalf("无书签应被拒绝")
	}
}

func TestLocalHubJSONRoundTripWithIcon(t *testing.T) {
	now := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	list := []bookmark.Bookmark{{
		ID: 7, Name: "带图标", WANURL: "https://icon.example.com/", LANURL: "http://192.168.1.7:8080",
		Folder: []string{"迁移", "图标"}, Description: "d", IconType: bookmark.IconFile, IconFile: "7-abcd1234.png", IconMime: "image/png", IconSize: int64(len(testPNG)),
		Enabled: true, Visibility: bookmark.VisibilityAuthenticated, OpenMode: bookmark.OpenModeSelf, SortOrder: 3,
		CreatedAt: now, UpdatedAt: now,
	}}
	data, warns := BuildLocalHubJSON(list, func(file string) ([]byte, error) { return testPNG, nil }, now)
	if len(warns) != 0 {
		t.Fatalf("不应有导出警告：%+v", warns)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("导出 JSON 非法：%v", err)
	}
	if doc["format"] != JSONFormatName || doc["version"] != float64(JSONFormatVersion) {
		t.Fatalf("缺少格式版本信息：%v", doc)
	}
	if strings.Contains(string(data), "7-abcd1234.png") {
		t.Fatalf("导出不应包含服务器本地图标路径")
	}
	recs, _, err := ParseLocalHubJSON(data)
	if err != nil || len(recs) != 1 {
		t.Fatalf("回读失败：recs=%v err=%v", recs, err)
	}
	if recs[0].IconType != bookmark.IconFile || len(recs[0].IconData) == 0 || recs[0].Visibility != bookmark.VisibilityAuthenticated || !equalPath(recs[0].Folder, []string{"迁移", "图标"}) {
		t.Fatalf("图标或权限未迁移：%+v", recs[0])
	}
}

func TestLocalHubJSONMissingIcon(t *testing.T) {
	list := []bookmark.Bookmark{{ID: 1, Name: "缺图标", WANURL: "https://a.example.com", Enabled: true, Visibility: bookmark.VisibilityPublic, OpenMode: bookmark.OpenModeNewTab, IconType: bookmark.IconFile, IconFile: "1-x.png"}}
	data, warns := BuildLocalHubJSON(list, func(string) ([]byte, error) { return nil, errNotFound }, time.Now())
	if len(warns) != 1 || warns[0].Code != "icon_missing" {
		t.Fatalf("应给出 icon_missing 警告：%+v", warns)
	}
	var doc struct {
		Bookmarks []struct {
			IconMissing bool `json:"icon_missing"`
		} `json:"bookmarks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || len(doc.Bookmarks) != 1 || !doc.Bookmarks[0].IconMissing {
		t.Fatalf("应标记 icon_missing：%s", data)
	}
	if _, _, err := ParseLocalHubJSON(data); err != nil {
		t.Fatalf("缺图标 JSON 仍应可解析：%v", err)
	}
}

func TestLocalHubJSONRejectsBadVersionAndOversize(t *testing.T) {
	bad := `{"format":"localhub-bookmarks","version":99,"bookmarks":[]}`
	if _, _, err := ParseLocalHubJSON([]byte(bad)); err == nil {
		t.Fatalf("不支持的版本应被拒绝")
	}
	big := `{"format":"localhub-bookmarks","version":1,"bookmarks":[{"name":"x","url":"https://x.example.com","icon_data_base64":"` + strings.Repeat("A", 400000) + `"}]}`
	if _, _, err := ParseLocalHubJSON([]byte(big)); err == nil {
		t.Fatalf("超大内嵌图标应被拒绝")
	}
}

func TestAnalyzeRecords(t *testing.T) {
	existing := []bookmark.Bookmark{{ID: 5, Name: "旧", WANURL: "https://dup.example.com/", Enabled: true, Visibility: bookmark.VisibilityPublic}}
	recs := []Record{
		{Index: 0, Name: "新站", WANURL: "https://new.example.com"},
		{Index: 1, Name: "重复", WANURL: "https://dup.example.com"},
		{Index: 2, Name: "", WANURL: ""},
		{Index: 3, Name: "批内重复", WANURL: "https://new.example.com"},
	}
	_, preview := AnalyzeRecords(recs, existing)
	if preview.Total != 4 || preview.New != 1 || preview.Duplicates != 2 || preview.Invalid != 1 {
		t.Fatalf("统计错误：%+v", preview)
	}
	if preview.Items[1].ExistingID != 5 || preview.Items[1].Duplicate != "url" {
		t.Fatalf("已有书签重复识别错误：%+v", preview.Items[1])
	}
	if preview.Items[3].Duplicate != "batch" {
		t.Fatalf("批内重复识别错误：%+v", preview.Items[3])
	}
}

type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }

var errNotFound error = notFoundError{}

func equalPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseSunPanelGroupFolder(t *testing.T) {
	raw := `{"data":[{"name":"工作","apps":[
		{"name":"组A","children":[{"name":"子站","url":"https://nested.example.com"}]},
		{"name":"面板","url":"https://panel2.example.com"}
	]}]}`
	recs, _, err := ParseSunPanel([]byte(raw))
	if err != nil || len(recs) != 2 {
		t.Fatalf("解析失败：recs=%v err=%v", recs, err)
	}
	if !equalPath(recs[0].Folder, []string{"工作", "组A"}) || !equalPath(recs[1].Folder, []string{"工作"}) {
		t.Fatalf("Sun-Panel 分组层级未保留：%+v", recs)
	}
}

func TestLocalHubJSONV1StillImports(t *testing.T) {
	raw := `{"format":"localhub-bookmarks","version":1,"bookmarks":[{"name":"旧版","wan_url":"https://v1.example.com","enabled":true,"visibility":"public","open_mode":"new_tab"}]}`
	recs, _, err := ParseLocalHubJSON([]byte(raw))
	if err != nil || len(recs) != 1 {
		t.Fatalf("v1 兼容失败：recs=%v err=%v", recs, err)
	}
	if recs[0].FolderProvided {
		t.Fatalf("v1 没有 folder_path，不应声称提供层级：%+v", recs[0])
	}
	b, err := bookmark.Build(nil, recs[0].ToInput())
	if err != nil || len(b.Folder) != 0 {
		t.Fatalf("v1 记录应按根层级导入：b=%+v err=%v", b, err)
	}
}

func TestLocalHubJSONRootFolderRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	list := []bookmark.Bookmark{{ID: 1, Name: "根站", WANURL: "https://root.example.com", Enabled: true, Visibility: bookmark.VisibilityPublic, OpenMode: bookmark.OpenModeNewTab, CreatedAt: now, UpdatedAt: now}}
	data, warns := BuildLocalHubJSON(list, func(string) ([]byte, error) { return nil, errNotFound }, now)
	if len(warns) != 0 {
		t.Fatalf("根书签不应产生图标警告：%+v", warns)
	}
	var doc struct {
		Bookmarks []struct {
			FolderPath *[]string `json:"folder_path"`
		} `json:"bookmarks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || len(doc.Bookmarks) != 1 || doc.Bookmarks[0].FolderPath == nil || len(*doc.Bookmarks[0].FolderPath) != 0 {
		t.Fatalf("根层级应显式导出空数组：%s err=%v", data, err)
	}
	recs, _, err := ParseLocalHubJSON(data)
	if err != nil || len(recs) != 1 || !recs[0].FolderProvided || len(recs[0].Folder) != 0 {
		t.Fatalf("根层级应可往返：%+v err=%v", recs, err)
	}
}

func TestParseSunPanelUnnamedGroupWarns(t *testing.T) {
	raw := `{"data":[{"children":[{"name":"子站","url":"https://unnamed.example.com"}]}]}`
	recs, warns, err := ParseSunPanel([]byte(raw))
	if err != nil || len(recs) != 1 {
		t.Fatalf("解析失败：recs=%v err=%v", recs, err)
	}
	if len(recs[0].Folder) != 0 || !recs[0].FolderProvided {
		t.Fatalf("无名分组子项应落在上一级并标记层级已提供：%+v", recs[0])
	}
	found := false
	for _, w := range warns {
		if w.Code == "sunpanel_folder_unnamed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("无名分组必须告警：%+v", warns)
	}
}

func TestParseMinifiedNestedHTML(t *testing.T) {
	raw := `<!DOCTYPE NETSCAPE-Bookmark-file-1><DL><p><DT><H3>工作</H3><DL><p><DT><H3>项目</H3><DL><p><DT><A HREF="https://min.example.com/">Min</A></DL><p></DL><p></DL><p>`
	recs, _, err := ParseHTML([]byte(raw))
	if err != nil || len(recs) != 1 {
		t.Fatalf("压缩 HTML 解析失败：recs=%v err=%v", recs, err)
	}
	if !equalPath(recs[0].Folder, []string{"工作", "项目"}) {
		t.Fatalf("压缩 HTML 层级错误：%+v", recs[0])
	}
}
