package logging

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// New 建立统一结构化日志（JSON，输出到 stdout，容器友好）。
func New(level, role string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String("time", t.Format(time.RFC3339Nano))
				}
			}
			return a
		},
	})
	return slog.New(h).With(slog.String("role", role), slog.String("service", "localhub"))
}
