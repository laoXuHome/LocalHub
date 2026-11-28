package probe

import (
	"time"

	"github.com/laoXuHome/LocalHub/internal/network"
)

// Status 是单个探测目标的最新状态。not_probed 与 disabled 不入库，由查询层推导。
type Status string

const (
	StatusSuccess          Status = "success"
	StatusHTTPError        Status = "http_error"
	StatusTimeout          Status = "timeout"
	StatusConnectionFailed Status = "connection_failed"
	StatusProbeError       Status = "probe_error"
	StatusNotProbed        Status = "not_probed"
	StatusDisabled         Status = "disabled"
)

func (s Status) persisted() bool {
	switch s {
	case StatusSuccess, StatusHTTPError, StatusTimeout, StatusConnectionFailed, StatusProbeError:
		return true
	default:
		return false
	}
}

// Result 是面向管理员的完整探测结果，包含目标地址与失败详情。
// 地址与内部错误只允许管理员读取。
type Result struct {
	Key         network.TargetKey `json:"key"`
	Label       string            `json:"label"`
	Address     string            `json:"address,omitempty"`
	Enabled     bool              `json:"enabled"`
	Status      Status            `json:"status"`
	HTTPStatus  int               `json:"http_status"`
	LatencyMS   int64             `json:"latency_ms"`
	CheckedAt   string            `json:"checked_at,omitempty"`
	FailureInfo string            `json:"failure_reason,omitempty"`
}

// PublicResult 是访客可见的探测状态，刻意不包含地址与内部错误详情。
type PublicResult struct {
	Key        network.TargetKey `json:"key"`
	Label      string            `json:"label"`
	Enabled    bool              `json:"enabled"`
	Status     Status            `json:"status"`
	HTTPStatus int               `json:"http_status"`
	LatencyMS  int64             `json:"latency_ms"`
	CheckedAt  string            `json:"checked_at,omitempty"`
}

// StatusResponse 是三个目标的最新状态集合。
type StatusResponse struct {
	Targets   []Result `json:"targets"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// PublicStatusResponse 是给访客的状态集合。
type PublicStatusResponse struct {
	Targets   []PublicResult `json:"targets"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

// PublicStatus 去除地址与失败详情，避免访客读取内网拓扑和内部错误。
func PublicStatus(in StatusResponse) PublicStatusResponse {
	out := PublicStatusResponse{UpdatedAt: in.UpdatedAt, Targets: make([]PublicResult, 0, len(in.Targets))}
	for _, r := range in.Targets {
		out.Targets = append(out.Targets, PublicResult{
			Key:        r.Key,
			Label:      r.Label,
			Enabled:    r.Enabled,
			Status:     r.Status,
			HTTPStatus: r.HTTPStatus,
			LatencyMS:  r.LatencyMS,
			CheckedAt:  r.CheckedAt,
		})
	}
	return out
}

// Run 是一次手动探测的执行记录。P4.2 每个进程只保留最近若干次，最新结果另存数据库。
type Run struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at,omitempty"`
	Results    []Result `json:"results"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
