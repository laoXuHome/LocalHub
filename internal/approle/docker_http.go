package approle

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
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

type bookmarkRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type bookmarkContainerLink struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Status    string        `json:"status"`
	Bookmarks []bookmarkRef `json:"bookmarks"`
}

type bookmarkMissingLink struct {
	BookmarkID   int64  `json:"bookmark_id"`
	BookmarkName string `json:"bookmark_name"`
	ContainerID  string `json:"container_id"`
}

type bookmarkLinksResponse struct {
	Status     dockerclient.Status     `json:"status"`
	CheckedAt  string                  `json:"checked_at"`
	Message    string                  `json:"message"`
	ReadOnly   bool                    `json:"read_only"`
	Containers []bookmarkContainerLink `json:"containers"`
	Missing    []bookmarkMissingLink   `json:"missing"`
	Unlinked   int                     `json:"unlinked"`
}

// handleAdminDockerBookmarkLinks 返回容器与书签的只读关联视图。
// 只有 Docker 连接成功时才会判定 missing；Docker 不可用时 missing 必须为空。
func (s *server) handleAdminDockerBookmarkLinks(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	list, err := store.List(r.Context(), bookmark.ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		s.log.Error("读取书签关联失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "读取书签关联失败")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	resp := bookmarkLinksResponse{
		Status: dockerclient.StatusUnknown, CheckedAt: now, Message: "Docker 状态未知",
		ReadOnly: true, Containers: []bookmarkContainerLink{}, Missing: []bookmarkMissingLink{},
	}
	if s.docker == nil {
		for _, b := range list {
			if b.DockerContainer == "" {
				resp.Unlinked++
			}
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	timeout := s.cfg.DockerTimeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout+time.Second)
	defer cancel()
	cl := s.docker.ListContainers(ctx)
	resp.Status, resp.CheckedAt, resp.Message = cl.Status, cl.CheckedAt, cl.Message

	byID := make(map[string]int, len(cl.Containers))
	for _, c := range cl.Containers {
		link := bookmarkContainerLink{ID: c.ID, Name: c.Name, Status: string(c.Status), Bookmarks: []bookmarkRef{}}
		resp.Containers = append(resp.Containers, link)
		byID[c.ID] = len(resp.Containers) - 1
	}
	for _, b := range list {
		if b.DockerContainer == "" {
			resp.Unlinked++
			continue
		}
		if cl.Status != dockerclient.StatusConnected {
			continue // Docker 不可用时不能判定容器不存在，也不能误删关联。
		}
		if idx, ok := byID[b.DockerContainer]; ok {
			resp.Containers[idx].Bookmarks = append(resp.Containers[idx].Bookmarks, bookmarkRef{ID: b.ID, Name: b.Name})
		} else {
			resp.Missing = append(resp.Missing, bookmarkMissingLink{BookmarkID: b.ID, BookmarkName: b.Name, ContainerID: b.DockerContainer})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
