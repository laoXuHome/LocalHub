package approle

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
	"github.com/laoXuHome/LocalHub/internal/transfer"
)

type importRequest struct {
	Format      string            `json:"format"`
	DataBase64  string            `json:"data_base64"`
	Duplicates  string            `json:"duplicates"`
	Selected    []int             `json:"selected"`
	Decisions   map[string]string `json:"decisions"`
	ImportIcons *bool             `json:"import_icons"`
}

func (s *server) decodeImportRequest(w http.ResponseWriter, r *http.Request) (importRequest, bool) {
	var req importRequest
	r.Body = http.MaxBytesReader(w, r.Body, transfer.MaxImportBytes*2)
	if !decodeJSON(w, r, &req) {
		return req, false
	}
	if req.DataBase64 == "" {
		writeError(w, http.StatusBadRequest, "empty_file", "缺少 data_base64")
		return req, false
	}
	if len(req.DataBase64) > base64.StdEncoding.EncodedLen(transfer.MaxImportBytes)+4 {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "导入文件超过 10 MiB 上限")
		return req, false
	}
	return req, true
}

func (s *server) parseImport(req importRequest) (string, []transfer.Record, []transfer.Warning, error) {
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		return "", nil, nil, fmt.Errorf("data_base64 非法")
	}
	if len(data) == 0 {
		return "", nil, nil, fmt.Errorf("文件内容为空")
	}
	if len(data) > transfer.MaxImportBytes {
		return "", nil, nil, fmt.Errorf("导入文件超过 10 MiB 上限")
	}
	return transfer.Parse(data, req.Format)
}

func (s *server) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	req, ok := s.decodeImportRequest(w, r)
	if !ok {
		return
	}
	format, recs, warns, err := s.parseImport(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_import_file", err.Error())
		return
	}
	existing, err := store.List(r.Context(), bookmark.ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取已有书签失败")
		return
	}
	_, preview := transfer.AnalyzeRecords(recs, existing)
	preview.Format = format
	preview.Source = formatSource(format)
	preview.Warnings = append(warns, preview.Warnings...)
	if preview.Warnings == nil {
		preview.Warnings = []transfer.Warning{}
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *server) handleImportExecute(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	req, ok := s.decodeImportRequest(w, r)
	if !ok {
		return
	}
	format, recs, warns, err := s.parseImport(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_import_file", err.Error())
		return
	}
	existing, err := store.List(r.Context(), bookmark.ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取已有书签失败")
		return
	}
	analyzed, _ := transfer.AnalyzeRecords(recs, existing)

	defaultMode := strings.ToLower(strings.TrimSpace(req.Duplicates))
	if defaultMode == "" {
		defaultMode = "skip"
	}
	switch defaultMode {
	case "skip", "overwrite", "keep_both":
	default:
		writeError(w, http.StatusBadRequest, "invalid_duplicates", "duplicates 只能是 skip/overwrite/keep_both")
		return
	}
	for _, v := range req.Decisions {
		switch strings.ToLower(v) {
		case "skip", "overwrite", "keep_both":
		default:
			writeError(w, http.StatusBadRequest, "invalid_decision", "decisions 只能是 skip/overwrite/keep_both")
			return
		}
	}
	importIcons := true
	if req.ImportIcons != nil {
		importIcons = *req.ImportIcons
	}
	selected := map[int]bool{}
	if req.Selected != nil {
		for _, idx := range req.Selected {
			selected[idx] = true
		}
	}

	sum := transfer.Summary{Warnings: warns, Items: []transfer.ImportResult{}}
	for _, a := range analyzed {
		rec := a.Record
		deselected := req.Selected != nil && !selected[rec.Index]
		if a.Err != nil {
			sum.Failed++
			sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: rec.Name, Status: "failed", Reason: a.Err.Error()})
			continue
		}
		if deselected {
			sum.Skipped++
			sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: a.Bookmark.Name, Status: "skipped", Reason: "未选择导入"})
			continue
		}
		mode := defaultMode
		if v, ok := req.Decisions[strconv.Itoa(rec.Index)]; ok {
			mode = strings.ToLower(v)
		}
		if a.Status == "duplicate" {
			switch mode {
			case "skip":
				sum.Skipped++
				sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: a.Bookmark.Name, Status: "skipped", Reason: a.Reason})
				continue
			case "keep_both":
				// 继续按新增处理
			case "overwrite":
				if a.ExistingID > 0 {
					iconOK, err := s.overwriteBookmark(r, store, a, importIcons)
					if err != nil {
						sum.Failed++
						sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: a.Bookmark.Name, Status: "failed", Reason: err.Error()})
						continue
					}
					sum.Overwritten++
					if iconOK {
						sum.Icons++
					}
					if len(rec.IconData) > 0 && !iconOK {
						sum.IconFailed++
					}
					sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: a.Bookmark.Name, Status: "overwritten", BookmarkID: a.ExistingID})
					continue
				}
			}
		}
		id, iconOK, err := s.createImportedBookmark(r, store, a, importIcons)
		if err != nil {
			sum.Failed++
			sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: a.Bookmark.Name, Status: "failed", Reason: err.Error()})
			continue
		}
		sum.Created++
		if iconOK {
			sum.Icons++
		}
		if len(rec.IconData) > 0 && !iconOK {
			sum.IconFailed++
		}
		sum.Items = append(sum.Items, transfer.ImportResult{Index: rec.Index, Name: a.Bookmark.Name, Status: "created", BookmarkID: id})
	}
	sum.Total = len(analyzed)
	if sum.IconFailed > 0 {
		sum.Warnings = append(sum.Warnings, transfer.Warning{Code: "icon_invalid", Message: fmt.Sprintf("%d 条记录的书签已导入，但内嵌图标未通过安全校验，未保存图标", sum.IconFailed)})
	}
	s.log.Info("导入书签完成",
		slog.String("format", format), slog.Int("created", sum.Created), slog.Int("overwritten", sum.Overwritten),
		slog.Int("skipped", sum.Skipped), slog.Int("failed", sum.Failed),
		slog.String("actor", actorName(sessionFrom(r.Context()))))
	writeJSON(w, http.StatusOK, sum)
}

func (s *server) createImportedBookmark(r *http.Request, store *bookmark.Store, a transfer.Analyzed, importIcons bool) (int64, bool, error) {
	b := *a.Bookmark
	created, err := store.Create(r.Context(), &b, time.Now())
	if err != nil {
		return 0, false, err
	}
	if importIcons && len(a.Record.IconData) > 0 && a.IconErr == nil && s.icons != nil {
		file, mimeType, err := s.icons.Save(created.ID, a.Record.IconData)
		if err != nil {
			return created.ID, false, nil
		}
		if err := store.SetIcon(r.Context(), created.ID, file, mimeType, int64(len(a.Record.IconData)), time.Now()); err != nil {
			_ = s.icons.Delete(file)
			return created.ID, false, nil
		}
		return created.ID, true, nil
	}
	return created.ID, false, nil
}

// overwriteBookmark 覆盖重复书签：只更新来源实际提供的字段，未提供的保留原值。
func (s *server) overwriteBookmark(r *http.Request, store *bookmark.Store, a transfer.Analyzed, importIcons bool) (bool, error) {
	old, err := store.Get(r.Context(), a.ExistingID)
	if err != nil {
		return false, err
	}
	in := a.Record.ToInput()
	if len(a.Record.IconData) > 0 {
		in.IconType = nil
		in.IconValue = nil
	}
	next, err := bookmark.Build(old, in)
	if err != nil {
		return false, err
	}
	if err := store.Update(r.Context(), next, time.Now()); err != nil {
		return false, err
	}
	if old.IconFile != "" && next.IconFile == "" {
		_ = store.ClearIconFile(r.Context(), old.ID, time.Now())
		if s.icons != nil {
			_ = s.icons.DeleteForID(old.ID)
		}
	}
	if importIcons && len(a.Record.IconData) > 0 && a.IconErr == nil && s.icons != nil {
		file, mimeType, err := s.icons.Save(old.ID, a.Record.IconData)
		if err != nil {
			return false, nil
		}
		if err := store.SetIcon(r.Context(), old.ID, file, mimeType, int64(len(a.Record.IconData)), time.Now()); err != nil {
			_ = s.icons.Delete(file)
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

func (s *server) handleExport(w http.ResponseWriter, r *http.Request) {
	store := s.bookmarkStore()
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用")
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "html" {
		writeError(w, http.StatusBadRequest, "invalid_format", "format 只能是 json 或 html")
		return
	}
	all, err := store.List(r.Context(), bookmark.ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取书签失败")
		return
	}
	list, err := filterExport(all, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	now := time.Now()
	date := now.Format("20060102")
	if format == "html" {
		data := transfer.BuildHTML(list, now)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"localhub-bookmarks-"+date+".html\"")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	readIcon := func(file string) ([]byte, error) {
		if s.icons == nil {
			return nil, errors.New("图标服务不可用")
		}
		return s.icons.Read(file)
	}
	data, warns := transfer.BuildLocalHubJSON(list, readIcon, now)
	if data == nil {
		writeError(w, http.StatusInternalServerError, "export_failed", "导出 JSON 失败")
		return
	}
	for _, wm := range warns {
		s.log.Warn("导出警告", slog.String("code", wm.Code), slog.String("message", wm.Message))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"localhub-bookmarks-"+date+".json\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func filterExport(all []bookmark.Bookmark, r *http.Request) ([]bookmark.Bookmark, error) {
	q := r.URL.Query()
	scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
	if scope == "" {
		scope = "all"
	}
	var ids map[int64]bool
	if scope == "selected" {
		raw := strings.TrimSpace(q.Get("ids"))
		if raw == "" {
			return nil, fmt.Errorf("scope=selected 时必须提供 ids")
		}
		ids = map[int64]bool{}
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("ids 包含非法 ID：%q", part)
			}
			ids[id] = true
		}
	} else if scope != "all" {
		return nil, fmt.Errorf("scope 只能是 all 或 selected")
	}
	var vis map[string]bool
	if raw := strings.TrimSpace(q.Get("visibility")); raw != "" {
		vis = map[string]bool{}
		for _, part := range strings.Split(raw, ",") {
			v := strings.ToLower(strings.TrimSpace(part))
			switch v {
			case bookmark.VisibilityPublic, bookmark.VisibilityAuthenticated, bookmark.VisibilityPrivate:
				vis[v] = true
			case bookmark.VisibilityDisabled:
				// 允许按“禁用”过滤：由 Enabled=false 表达。
				vis[v] = true
			default:
				return nil, fmt.Errorf("visibility 包含非法值：%q", part)
			}
		}
	}
	out := make([]bookmark.Bookmark, 0, len(all))
	for _, b := range all {
		if ids != nil && !ids[b.ID] {
			continue
		}
		if vis != nil {
			if !vis[b.EffectiveVisibility()] {
				continue
			}
		}
		out = append(out, b)
	}
	return out, nil
}

func formatSource(format string) string {
	switch format {
	case transfer.FormatHTML:
		return "Chrome/Edge 书签 HTML"
	case transfer.FormatSunPanel:
		return "Sun-Panel JSON"
	case transfer.FormatLocalHub:
		return "LocalHub JSON"
	default:
		return format
	}
}
