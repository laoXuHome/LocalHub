package buildinfo

// 这些变量由 -ldflags -X 注入，构建时必须保持一致。
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// String 返回便于日志输出的版本串。
func String() string {
	return Version + " (" + Commit + ")"
}
