package approle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
	"github.com/laoXuHome/LocalHub/internal/icon"
)

// faviconFetcher 抽象抓取器，便于测试注入；生产使用 favicon.Fetcher。
type faviconFetcher interface {
	Fetch(ctx context.Context, raw string) ([]byte, string, error)
}

// handleBookmarkIcon：读取书签本地图标，权限与书签本身一致。
func (s *server) handleBookmarkIcon(w http.ResponseWriter, r *http.Request) {
	store, icons := s.bookmarkStore(), s.icons
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	if store == nil || icons == nil {
		writeError(w, http.StatusServiceUnavailable, "icon_unavailable", "图标服务不可用")
		return
	}
	b, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_, admin := s.currentSession(r)
	if !b.VisibleTo(admin) || b.IconFile == "" {
		writeError(w, http.StatusNotFound, "not_found", "图标不存在")
		return
	}
	data, err := icons.Read(b.IconFile)
	if err != nil {
		// 文件缺失/损坏只影响该图标，返回 404，不影响其他书签与进程。
		s.log.Warn("读取图标文件失败", slog.Int64("bookmark_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusNotFound, "not_found", "图标不存在")
		return
	}
	etag := `"` + b.IconFile + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ct := b.IconMime
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	if b.Visibility == bookmark.VisibilityPublic && b.Enabled {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleBookmarkIconUpload：上传并校验图标，替换书签当前图标。
func (s *server) handleBookmarkIconUpload(w http.ResponseWriter, r *http.Request) {
	store, icons := s.bookmarkStore(), s.icons
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	if store == nil || icons == nil {
		writeError(w, http.StatusServiceUnavailable, "icon_unavailable", "图标服务不可用")
		return
	}
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	if !strings.HasPrefix(ct, "image/") {
		writeError(w, http.StatusBadRequest, "invalid_icon", "Content-Type 必须是图片类型")
		return
	}
	b, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}

	body := http.MaxBytesReader(w, r.Body, icon.MaxBytes)
	data, err := io.ReadAll(body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "icon_too_large", "图标超过 256 KiB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_icon", "读取图标数据失败")
		return
	}

	file, mimeType, err := icons.Save(id, data)
	if err != nil {
		if errors.Is(err, icon.ErrInvalid) {
			writeError(w, http.StatusBadRequest, "invalid_icon", err.Error())
			return
		}
		s.log.Error("保存图标失败", slog.Int64("bookmark_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "icon_store_failed", "图标保存失败")
		return
	}

	old := b.IconFile
	if err := store.SetIcon(r.Context(), id, file, mimeType, int64(len(data)), time.Now()); err != nil {
		_ = icons.Delete(file)
		writeStoreErr(w, err)
		return
	}
	if old != "" && old != file {
		_ = icons.Delete(old)
	}

	if nb, err := store.Get(r.Context(), id); err == nil {
		writeJSON(w, http.StatusOK, bookmarkJSON(*nb))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "icon_url": bookmarkIconURL(id)})
}

// handleBookmarkIconDelete：删除书签图标并清理本地文件。
func (s *server) handleBookmarkIconDelete(w http.ResponseWriter, r *http.Request) {
	store, icons := s.bookmarkStore(), s.icons
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	if store == nil || icons == nil {
		writeError(w, http.StatusServiceUnavailable, "icon_unavailable", "图标服务不可用")
		return
	}
	b, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if err := store.ClearIcon(r.Context(), id, time.Now()); err != nil {
		writeStoreErr(w, err)
		return
	}
	if b.IconFile != "" {
		_ = icons.Delete(b.IconFile)
	}
	_ = icons.DeleteForID(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bookmark_id": id})
}

// handleBookmarkFavicon：从书签 URL 重新抓取图标，失败返回 502 且不改动现有图标。
func (s *server) handleBookmarkFavicon(w http.ResponseWriter, r *http.Request) {
	store, icons, fetcher := s.bookmarkStore(), s.icons, s.favicon
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "书签 ID 非法")
		return
	}
	if store == nil || icons == nil || fetcher == nil {
		writeError(w, http.StatusServiceUnavailable, "icon_unavailable", "图标服务不可用")
		return
	}
	b, err := store.Get(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	target := b.WANURL
	if target == "" {
		target = b.LANURL
	}
	if target == "" {
		writeError(w, http.StatusBadRequest, "no_url", "书签没有可用于抓取的 URL")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	data, mimeType, err := fetcher.Fetch(ctx, target)
	if err != nil {
		s.log.Warn("抓取图标失败", slog.Int64("bookmark_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "favicon_fetch_failed", "无法获取网站图标")
		return
	}
	file, mimeType, err := icons.Save(id, data)
	if err != nil {
		writeError(w, http.StatusBadGateway, "favicon_save_failed", "图标无法保存")
		return
	}
	old := b.IconFile
	if err := store.SetIcon(r.Context(), id, file, mimeType, int64(len(data)), time.Now()); err != nil {
		_ = icons.Delete(file)
		writeStoreErr(w, err)
		return
	}
	if old != "" && old != file {
		_ = icons.Delete(old)
	}
	if nb, err := store.Get(r.Context(), id); err == nil {
		writeJSON(w, http.StatusOK, bookmarkJSON(*nb))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "icon_url": bookmarkIconURL(id)})
}

// maybeRefreshFavicon 在后台抓取图标，不阻塞书签创建/编辑请求。
func (s *server) maybeRefreshFavicon(id int64) {
	if !s.autoFavicon || s.icons == nil || s.favicon == nil || s.db == nil {
		return
	}
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("自动抓取图标异常", slog.Int64("bookmark_id", id), slog.Any("panic", rec))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		store := s.bookmarkStore()
		b, err := store.Get(ctx, id)
		if err != nil || b.IconType != bookmark.IconFavicon {
			return
		}
		target := b.WANURL
		if target == "" {
			target = b.LANURL
		}
		data, mimeType, err := s.favicon.Fetch(ctx, target)
		if err != nil {
			s.log.Warn("自动抓取图标失败", slog.Int64("bookmark_id", id), slog.String("error", err.Error()))
			return
		}
		file, mimeType, err := s.icons.Save(id, data)
		if err != nil {
			s.log.Warn("自动保存图标失败", slog.Int64("bookmark_id", id), slog.String("error", err.Error()))
			return
		}
		old := b.IconFile
		if err := store.SetIcon(ctx, id, file, mimeType, int64(len(data)), time.Now()); err != nil {
			_ = s.icons.Delete(file)
			s.log.Warn("自动关联图标失败", slog.Int64("bookmark_id", id), slog.String("error", err.Error()))
			return
		}
		if old != "" && old != file {
			_ = s.icons.Delete(old)
		}
	}()
}

func bookmarkIconURL(id int64) string { return "/api/bookmarks/" + strconv.FormatInt(id, 10) + "/icon" }
