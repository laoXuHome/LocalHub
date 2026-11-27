package approle

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
)

// handlePublicBookmarkLayout：读取当前调用者可见书签的桌面/移动端布局。
// 布局本身不包含 URL 等敏感字段；仍按书签可见性过滤，避免泄露私有/未启用书签 ID。
func (s *server) handlePublicBookmarkLayout(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	_, admin := s.currentSession(r)
	list, err := store.List(r.Context(), bookmark.ListOptions{Admin: admin})
	if err != nil {
		s.log.Error("读取布局可见书签失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "读取首页布局失败")
		return
	}
	allowed := make(map[int64]struct{}, len(list))
	for _, b := range list {
		allowed[b.ID] = struct{}{}
	}
	layout, err := store.GetLayout(r.Context())
	if err != nil {
		s.log.Error("读取首页布局失败", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "读取首页布局失败")
		return
	}
	layout.Desktop = filterLayout(layout.Desktop, allowed)
	layout.Mobile = filterLayout(layout.Mobile, allowed)
	writeJSON(w, http.StatusOK, layout)
}

func filterLayout(items []bookmark.LayoutItem, allowed map[int64]struct{}) []bookmark.LayoutItem {
	out := make([]bookmark.LayoutItem, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.ID]; ok {
			out = append(out, item)
		}
	}
	return out
}

// handleAdminSaveBookmarkLayout：管理员手动保存整套桌面/移动端布局。
// 只写 bookmark_layouts，不触碰 bookmarks.sort_order。
func (s *server) handleAdminSaveBookmarkLayout(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	var layout bookmark.Layout
	if !decodeJSON(w, r, &layout) {
		return
	}
	if layout.Desktop == nil || layout.Mobile == nil {
		writeError(w, http.StatusBadRequest, "invalid_layout", "desktop 与 mobile 布局都必须提供")
		return
	}
	if err := bookmark.ValidateLayout(layout); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_layout", err.Error())
		return
	}
	if err := store.ReplaceLayouts(r.Context(), layout, time.Now()); err != nil {
		s.log.Error("保存首页布局失败", slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "invalid_layout", err.Error())
		return
	}
	now, err := store.GetLayout(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "布局已保存但读取失败")
		return
	}
	s.log.Info("保存首页布局",
		slog.Int("desktop", len(layout.Desktop)),
		slog.Int("mobile", len(layout.Mobile)),
		slog.String("actor", actorName(sessionFrom(r.Context()))))
	writeJSON(w, http.StatusOK, now)
}
