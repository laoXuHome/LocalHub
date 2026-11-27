package static

import "embed"

// FS 是前端构建产物。P0 只包含占位页面；镜像构建时会被真实的 Vite 产物覆盖。
//
//go:embed all:dist
var FS embed.FS
