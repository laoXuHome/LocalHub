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

type dockerContainersResponse struct {
	Status                 dockerclient.Status      `json:"status"`
	CheckedAt              string                   `json:"checked_at"`
	Message                string                   `json:"message"`
	ReadOnly               bool                     `json:"read_only"`
	RefreshIntervalSeconds int                      `json:"refresh_interval_seconds"`
	Containers             []dockerclient.Container `json:"containers"`
	Counts                 dockerclient.Counts      `json:"counts"`
	Skipped                int                      `json:"skipped,omitempty"`
	Truncated              bool                     `json:"truncated,omitempty"`
}

// handleAdminDockerContainers 返回脱敏后的容器发现结果。
// 连接失败时返回空容器和失败状态，不把容器误报为 stopped。
func (s *server) handleAdminDockerContainers(w http.ResponseWriter, r *http.Request) {
	refresh := 30
	if s.cfg.DockerRefreshInterval >= 10*time.Second && s.cfg.DockerRefreshInterval <= 300*time.Second {
		refresh = int(s.cfg.DockerRefreshInterval.Seconds())
	}
	if s.docker == nil {
		writeJSON(w, http.StatusOK, dockerContainersResponse{
			Status:                 dockerclient.StatusUnknown,
			CheckedAt:              time.Now().UTC().Format(time.RFC3339),
			Message:                "Docker 状态未知",
			ReadOnly:               true,
			RefreshIntervalSeconds: refresh,
			Containers:             []dockerclient.Container{},
		})
		return
	}
	timeout := s.cfg.DockerTimeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout+time.Second)
	defer cancel()
	list := s.docker.ListContainers(ctx)
	writeJSON(w, http.StatusOK, dockerContainersResponse{
		Status:                 list.Status,
		CheckedAt:              list.CheckedAt,
		Message:                list.Message,
		ReadOnly:               true,
		RefreshIntervalSeconds: refresh,
		Containers:             list.Containers,
		Counts:                 list.Counts,
		Skipped:                list.Skipped,
		Truncated:              list.Truncated,
	})
}
