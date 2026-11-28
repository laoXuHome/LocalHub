package approle

import (
	"errors"
	"net/http"
	"time"

	"github.com/laoXuHome/LocalHub/internal/network"
)

// handleAdminNetworkTargets 仅管理员可读写探测目标配置。
// 配置只落入 P4.1 的 network_targets；P4.2 的真实探测结果保存在独立表。
func (s *server) handleAdminNetworkTargets(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	store := &network.Store{DB: s.db}
	switch r.Method {
	case http.MethodGet:
		cfg, err := store.Get(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "network_config_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		var in network.Input
		if !decodeJSON(w, r, &in) {
			return
		}
		cfg, err := store.Save(r.Context(), in, time.Now())
		if err != nil {
			if errors.Is(err, network.ErrInvalid) {
				writeError(w, http.StatusBadRequest, "invalid_network_config", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "network_config_failed", err.Error())
			return
		}
		s.dropDisabledProbeResults(r, cfg)
		writeJSON(w, http.StatusOK, cfg)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "只支持 GET/PUT")
	}
}
