package transfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Detect 根据内容判断格式；format 非 auto 时直接返回该值。
// HTML 与 Chrome/Edge 使用同一 Netscape 书签格式，无法从内容可靠区分来源。
func Detect(data []byte, format string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("文件内容为空")
	}
	if len(data) > MaxImportBytes {
		return "", fmt.Errorf("文件超过 %d MiB 上限", MaxImportBytes>>20)
	}
	if format != "" && format != FormatAuto {
		switch format {
		case FormatHTML, FormatSunPanel, FormatLocalHub:
			return format, nil
		default:
			return "", fmt.Errorf("不支持的导入格式：%s", format)
		}
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("文件内容为空")
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		var probe struct {
			Format  string `json:"format"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(trimmed, &probe); err == nil && probe.Format == JSONFormatName {
			return FormatLocalHub, nil
		}
		return FormatSunPanel, nil
	}
	if bytes.Contains(bytes.ToLower(trimmed), []byte("<dl")) || bytes.Contains(bytes.ToLower(trimmed), []byte("<!doctype netscape")) {
		return FormatHTML, nil
	}
	return "", fmt.Errorf("无法识别的文件格式")
}

// Parse 解析任意受支持格式。
func Parse(data []byte, format string) (string, []Record, []Warning, error) {
	detected, err := Detect(data, format)
	if err != nil {
		return "", nil, nil, err
	}
	switch detected {
	case FormatHTML:
		recs, warns, err := ParseHTML(data)
		return detected, recs, warns, err
	case FormatSunPanel:
		recs, warns, err := ParseSunPanel(data)
		return detected, recs, warns, err
	case FormatLocalHub:
		recs, warns, err := ParseLocalHubJSON(data)
		return detected, recs, warns, err
	default:
		return detected, nil, nil, fmt.Errorf("不支持的格式")
	}
}

func sourceName(format string) string {
	switch format {
	case FormatHTML:
		return "Chrome/Edge 书签 HTML"
	case FormatSunPanel:
		return "Sun-Panel JSON"
	case FormatLocalHub:
		return "LocalHub JSON"
	default:
		return strings.ToUpper(format)
	}
}
