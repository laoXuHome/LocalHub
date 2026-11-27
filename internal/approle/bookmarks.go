package approle

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/laoXuHome/LocalHub/internal/auth"
	"github.com/laoXuHome/LocalHub/internal/bookmark"
)

func (s *server) bookmarkStore() *bookmark.Store {
	if s.db == nil {
		return nil
	}
	return &bookmark.Store{DB: s.db}
}

func bookmarkJSON(b bookmark.Bookmark) map[string]any {
	return map[string]any{
		"id":                   b.ID,
		"name":                 b.Name,
		"lan_url":              b.LANURL,
		"wan_url":              b.WANURL,
		"description":          b.Description,
		"icon_type":            b.IconType,
		"icon_value":           b.IconValue,
		"enabled":              b.Enabled,
		"visibility":           b.Visibility,
		"effective_visibility": b.EffectiveVisibility(),
		"open_mode":            b.OpenMode,
		"docker_container":     b.DockerContainer,
		"sort_order":           b.SortOrder,
		"created_at":           b.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":           b.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bookmark.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "书签不存在")
	case errors.Is(err, bookmark.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_bookmark", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "书签操作失败")
	}
}

// handlePublicBookmarks：访客/管理员读取书签列表，按权限过滤。
func (s *server) handlePublicBookmarks(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	_, admin := s.currentSession(r)
	list, err := store.List(r.Context(), bookmark.ListOptions{Admin: admin})
	if err != nil {
		s.log.Error("读取书签列表失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "读取书签失败")
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, b := range list {
		items = append(items, bookmarkJSON(b))
	}
	role := "guest"
	if admin {
		role = "admin"
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "role": role})
}

// handlePublicBookmark：读取单个书签，不可见时统一返回 404（不泄露存在性）。
func (s *server) handlePublicBookmark(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	b, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_, admin := s.currentSession(r)
	if !b.VisibleTo(admin) {
		writeError(w, http.StatusNotFound, "not_found", "书签不存在")
		return
	}
	writeJSON(w, http.StatusOK, bookmarkJSON(*b))
}

// handleAdminListBookmarks：管理员可见全部（含禁用）。
func (s *server) handleAdminListBookmarks(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	list, err := store.List(r.Context(), bookmark.ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		s.log.Error("读取书签列表失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "读取书签失败")
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, b := range list {
		items = append(items, bookmarkJSON(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleAdminGetBookmark：管理员读取单个书签（含禁用）。
func (s *server) handleAdminGetBookmark(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	b, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookmarkJSON(*b))
}

// handleAdminCreateBookmark：新增书签。
func (s *server) handleAdminCreateBookmark(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	var in bookmark.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	b, err := bookmark.Build(nil, in)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	created, err := store.Create(r.Context(), b, time.Now())
	if err != nil {
		s.log.Error("新增书签失败", slog.String("error", err.Error()))
		writeStoreErr(w, err)
		return
	}
	sess := sessionFrom(r.Context())
	s.log.Info("新增书签", slog.Int64("bookmark_id", created.ID), slog.String("name", created.Name), slog.String("actor", actorName(sess)))
	writeJSON(w, http.StatusCreated, bookmarkJSON(*created))
}

// handleAdminUpdateBookmark：编辑书签（未提供字段保持不变）。
func (s *server) handleAdminUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	base, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	var in bookmark.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	next, err := bookmark.Build(base, in)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if err := store.Update(r.Context(), next, time.Now()); err != nil {
		s.log.Error("编辑书签失败", slog.String("error", err.Error()), slog.Int64("bookmark_id", id))
		writeStoreErr(w, err)
		return
	}
	s.log.Info("编辑书签", slog.Int64("bookmark_id", id), slog.String("actor", actorName(sessionFrom(r.Context()))))
	writeJSON(w, http.StatusOK, bookmarkJSON(*next))
}

// handleAdminDeleteBookmark：删除单个书签，目标必须存在。
func (s *server) handleAdminDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	if err := store.Delete(r.Context(), id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.log.Info("删除书签", slog.Int64("bookmark_id", id), slog.String("actor", actorName(sessionFrom(r.Context()))))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": 1})
}

// handleAdminBatchDelete：批量删除。
func (s *server) handleAdminBatchDelete(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, missing, err := store.BatchDelete(r.Context(), req.IDs)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if missing == nil {
		missing = []int64{}
	}
	s.log.Info("批量删除书签", slog.Int("deleted", n), slog.Int("missing", len(missing)), slog.String("actor", actorName(sessionFrom(r.Context()))))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n, "missing": missing})
}

// handleAdminReorder：按给定 ID 顺序重排。
func (s *server) handleAdminReorder(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := store.Reorder(r.Context(), req.IDs, time.Now()); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.log.Info("调整书签排序", slog.Int("count", len(req.IDs)), slog.String("actor", actorName(sessionFrom(r.Context()))))
	list, err := store.List(r.Context(), bookmark.ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取书签失败")
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, b := range list {
		items = append(items, bookmarkJSON(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "total": len(items)})
}

func actorName(sess *auth.Session) string {
	if sess == nil {
		return "unknown"
	}
	return sess.Username
}
