package approle

import (
	"errors"
	"net/http"
	"strings"

	"github.com/laoXuHome/LocalHub/internal/network"
	"github.com/laoXuHome/LocalHub/internal/probe"
)

// handlePublicNetworkStatus 只返回访客可见的状态：目标名称、状态、耗时、时间、HTTP 码。
// 不返回地址、内部错误文本或探测配置。
func (s *server) handlePublicNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "network_status_unavailable", "网络状态暂时不可用")
		return
	}
	status, err := (&probe.ResultStore{DB: s.db}).Latest(r.Context())
	if err != nil {
		s.log.Error("读取公开网络状态失败", "error", err.Error())
		writeError(w, http.StatusServiceUnavailable, "network_status_unavailable", "网络状态暂时不可用")
		return
	}
	writeJSON(w, http.StatusOK, probe.PublicStatus(status))
}

// handleAdminNetworkResults 返回管理员可见的完整结果，包含目标地址与失败原因。
func (s *server) handleAdminNetworkResults(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	status, err := (&probe.ResultStore{DB: s.db}).Latest(r.Context())
	if err != nil {
		s.log.Error("读取管理员网络探测结果失败", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "network_results_failed", "读取探测结果失败")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleAdminNetworkProbe 手动触发一次探测；目标完全来自已保存配置，不接受 URL 参数。
func (s *server) handleAdminNetworkProbe(w http.ResponseWriter, r *http.Request) {
	if s.probe == nil {
		writeError(w, http.StatusServiceUnavailable, "probe_unavailable", "探测服务不可用")
		return
	}
	run, err := s.probe.Start(r.Context())
	if errors.Is(err, probe.ErrInProgress) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "probe_in_progress", "message": "已有探测正在进行，请等待完成",
			"run_id": run.ID, "status": run.Status,
		})
		return
	}
	if err != nil {
		s.log.Error("启动网络探测失败", "error", err.Error())
		writeError(w, http.StatusServiceUnavailable, "probe_start_failed", "无法启动探测")
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

// handleAdminNetworkProbeRun 返回单次探测执行结果。
func (s *server) handleAdminNetworkProbeRun(w http.ResponseWriter, r *http.Request) {
	if s.probe == nil {
		writeError(w, http.StatusServiceUnavailable, "probe_unavailable", "探测服务不可用")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeError(w, http.StatusBadRequest, "invalid_probe_run", "探测记录标识不合法")
		return
	}
	run, ok := s.probe.Run(id)
	if !ok {
		writeError(w, http.StatusNotFound, "probe_run_not_found", "探测记录不存在")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// dropDisabledProbeResults 在管理员保存配置后清理禁用目标的旧结果。
// 清理失败不改变已经成功的配置保存结果。
func (s *server) dropDisabledProbeResults(r *http.Request, cfg network.Config) {
	if s.db == nil {
		return
	}
	if err := (&probe.ResultStore{DB: s.db}).DropDisabled(r.Context(), cfg); err != nil {
		s.log.Warn("清理禁用目标探测结果失败", "error", err.Error())
	}
}
