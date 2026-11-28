package approle

import (
	"context"
	"net/http"
	"time"

	"github.com/laoXuHome/LocalHub/internal/dockerclient"
)

// handleAdminDockerStatus 返回脱敏后的 Docker 只读连接状态。
// 不返回连接地址、Socket 路径、响应体或内部错误堆栈。
func (s *server) handleAdminDockerStatus(w http.ResponseWriter, r *http.Request) {
	if s.docker == nil {
		writeJSON(w, http.StatusOK, dockerclient.CheckResult{
			Status:    dockerclient.StatusUnknown,
			CheckedAt: time.Now().UTC().Format(time.RFC3339),
			Message:   "Docker 状态未知",
			ReadOnly:  true,
		})
		return
	}
	timeout := s.cfg.DockerTimeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 3 * time.Second
	}
	// 在客户端超时之外留少量余量，确保上游卡住时接口能及时返回。
	ctx, cancel := context.WithTimeout(r.Context(), timeout+time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.docker.Check(ctx))
}
