package bookmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

// ErrNotFound 表示书签不存在。
var ErrNotFound = errors.New("bookmark not found")

// Store 封装书签表读写。
type Store struct{ DB *db.DB }

// ListOptions 控制列表范围。
type ListOptions struct {
	// Admin 为 true 时包含 logged_in / private 权限的书签。
	Admin bool
	// IncludeDisabled 为 true 时包含 enabled=false 的书签（仅管理列表使用）。
	IncludeDisabled bool
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTS(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

const selectCols = `id, name, lan_url, wan_url, description, icon_type, icon_value, icon_file, icon_mime, icon_size, icon_updated_at, enabled, visibility, open_mode, docker_container, sort_order, created_at, updated_at`

func scan(sc interface{ Scan(...any) error }) (*Bookmark, error) {
	b := &Bookmark{}
	var enabled int
	var created, updated, iconUpdated string
	if err := sc.Scan(&b.ID, &b.Name, &b.LANURL, &b.WANURL, &b.Description, &b.IconType, &b.IconValue,
		&b.IconFile, &b.IconMime, &b.IconSize, &iconUpdated,
		&enabled, &b.Visibility, &b.OpenMode, &b.DockerContainer, &b.SortOrder, &created, &updated); err != nil {
		return nil, err
	}
	b.IconUpdatedAt = parseTS(iconUpdated)
	b.Enabled = enabled == 1
	b.CreatedAt = parseTS(created)
	b.UpdatedAt = parseTS(updated)
	return b, nil
}

// List 返回调用者可见的书签，按 sort_order、id 升序。
func (s *Store) List(ctx context.Context, o ListOptions) ([]Bookmark, error) {
	where := []string{}
	if !o.IncludeDisabled {
		where = append(where, "enabled = 1")
	}
	if !o.Admin {
		where = append(where, "visibility = ?")
	}
	q := "SELECT " + selectCols + " FROM bookmarks"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY sort_order ASC, id ASC"

	var (
		rows *sql.Rows
		err  error
	)
	if o.Admin {
		rows, err = s.DB.SQL.QueryContext(ctx, q)
	} else {
		rows, err = s.DB.SQL.QueryContext(ctx, q, VisibilityPublic)
	}
	if err != nil {
		return nil, fmt.Errorf("查询书签失败：%w", err)
	}
	defer rows.Close()

	out := make([]Bookmark, 0)
	for rows.Next() {
		b, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("读取书签失败：%w", err)
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// Get 按 id 读取书签；不存在返回 ErrNotFound。
func (s *Store) Get(ctx context.Context, id int64) (*Bookmark, error) {
	row := s.DB.SQL.QueryRowContext(ctx, "SELECT "+selectCols+" FROM bookmarks WHERE id = ?", id)
	b, err := scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("读取书签失败：%w", err)
	}
	return b, nil
}

// Create 新增书签；未指定 sort_order 时追加到末尾。
func (s *Store) Create(ctx context.Context, b *Bookmark, now time.Time) (*Bookmark, error) {
	var maxOrder sql.NullInt64
	if err := s.DB.SQL.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM bookmarks`).Scan(&maxOrder); err != nil {
		return nil, fmt.Errorf("读取排序失败：%w", err)
	}
	if b.SortOrder == 0 {
		if maxOrder.Valid {
			b.SortOrder = int(maxOrder.Int64) + 1
		} else {
			b.SortOrder = 0
		}
	}
	res, err := s.DB.SQL.ExecContext(ctx, `
		INSERT INTO bookmarks (name, lan_url, wan_url, description, icon_type, icon_value, enabled, visibility,
			open_mode, docker_container, sort_order, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.Name, b.LANURL, b.WANURL, b.Description, b.IconType, b.IconValue, boolInt(b.Enabled), b.Visibility,
		b.OpenMode, b.DockerContainer, b.SortOrder, ts(now), ts(now))
	if err != nil {
		return nil, fmt.Errorf("新增书签失败：%w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取新增书签 ID 失败：%w", err)
	}
	b.ID = id
	b.CreatedAt = now
	b.UpdatedAt = now
	return b, nil
}

// Update 覆盖更新书签（字段已在 Build/Validate 中校验）。
func (s *Store) Update(ctx context.Context, b *Bookmark, now time.Time) error {
	res, err := s.DB.SQL.ExecContext(ctx, `
		UPDATE bookmarks SET name = ?, lan_url = ?, wan_url = ?, description = ?, icon_type = ?, icon_value = ?,
			icon_file = ?, icon_mime = ?, icon_size = ?, icon_updated_at = ?,
			enabled = ?, visibility = ?, open_mode = ?, docker_container = ?, sort_order = ?, updated_at = ?
		WHERE id = ?`,
		b.Name, b.LANURL, b.WANURL, b.Description, b.IconType, b.IconValue,
		b.IconFile, b.IconMime, b.IconSize, ts(b.IconUpdatedAt),
		boolInt(b.Enabled), b.Visibility, b.OpenMode, b.DockerContainer, b.SortOrder, ts(now), b.ID)
	if err != nil {
		return fmt.Errorf("更新书签失败：%w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	b.UpdatedAt = now
	return nil
}

// Delete 删除单个书签。
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.DB.SQL.ExecContext(ctx, `DELETE FROM bookmarks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除书签失败：%w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// BatchDelete 批量删除，返回实际删除数量与不存在的 ID。
func (s *Store) BatchDelete(ctx context.Context, ids []int64) (int, []int64, error) {
	if len(ids) == 0 {
		return 0, nil, invalid("至少提供一个书签 ID")
	}
	if len(ids) > MaxBatchSize {
		return 0, nil, invalid("单次最多删除 %d 条", MaxBatchSize)
	}
	uniq := make(map[int64]struct{}, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return 0, nil, invalid("书签 ID 必须为正整数")
		}
		if _, dup := uniq[id]; dup {
			continue
		}
		uniq[id] = struct{}{}
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")

	// 先记录实际存在的 ID，避免删除后再查询时把已成功删除的 ID 误报为不存在。
	rows, err := s.DB.SQL.QueryContext(ctx, `SELECT id FROM bookmarks WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("查询书签失败：%w", err)
	}
	existing := make(map[int64]struct{}, len(args))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("读取书签 ID 失败：%w", err)
		}
		existing[id] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return 0, nil, fmt.Errorf("关闭游标失败：%w", err)
	}

	res, err := s.DB.SQL.ExecContext(ctx, `DELETE FROM bookmarks WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("批量删除失败：%w", err)
	}
	n, _ := res.RowsAffected()

	var missing []int64
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			missing = append(missing, id)
		}
	}
	return int(n), uniqIDs(missing), nil
}

// Reorder 按给定顺序重排书签（0 基序号），顺序中不允许出现不存在的 ID。
func (s *Store) Reorder(ctx context.Context, ids []int64, now time.Time) error {
	if len(ids) == 0 {
		return invalid("至少提供一个书签 ID")
	}
	if len(ids) > MaxBatchSize {
		return invalid("单次最多调整 %d 条", MaxBatchSize)
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return invalid("书签 ID 必须为正整数")
		}
		if seen[id] {
			return invalid("书签 ID 重复：%d", id)
		}
		seen[id] = true
	}
	tx, err := s.DB.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE bookmarks SET sort_order = ?, updated_at = ? WHERE id = ?`, i, ts(now), id)
		if err != nil {
			return fmt.Errorf("调整排序失败：%w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return invalid("书签不存在：%d", id)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交排序失败：%w", err)
	}
	return nil
}

// Count 返回书签总数（用于测试与后续统计）。
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	if err := s.DB.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计书签失败：%w", err)
	}
	return n, nil
}

// SortVisible 把两个列表按 sort_order 排序（供测试与上层合并使用）。
func SortVisible(list []Bookmark) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].SortOrder != list[j].SortOrder {
			return list[i].SortOrder < list[j].SortOrder
		}
		return list[i].ID < list[j].ID
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// uniqIDs 去除重复 ID 并保持原顺序。
func uniqIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// SetIcon 关联本地图标文件并切换到 icon_type=file。
func (s *Store) SetIcon(ctx context.Context, id int64, file, mimeType string, size int64, now time.Time) error {
	res, err := s.DB.SQL.ExecContext(ctx, `
		UPDATE bookmarks SET icon_type = ?, icon_value = '', icon_file = ?, icon_mime = ?, icon_size = ?,
			icon_updated_at = ?, updated_at = ?
		WHERE id = ?`, IconFile, file, mimeType, size, ts(now), ts(now), id)
	if err != nil {
		return fmt.Errorf("保存图标信息失败：%w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearIcon 删除图标关联并回到无图标状态。
func (s *Store) ClearIcon(ctx context.Context, id int64, now time.Time) error {
	res, err := s.DB.SQL.ExecContext(ctx, `
		UPDATE bookmarks SET icon_type = ?, icon_value = '', icon_file = '', icon_mime = '', icon_size = 0,
			icon_updated_at = '', updated_at = ?
		WHERE id = ?`, IconNone, ts(now), id)
	if err != nil {
		return fmt.Errorf("清除图标信息失败：%w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearIconFile 只清除本地图标文件元数据，保留 icon_type/icon_value（用于切换图标类型）。
func (s *Store) ClearIconFile(ctx context.Context, id int64, now time.Time) error {
	res, err := s.DB.SQL.ExecContext(ctx, `
		UPDATE bookmarks SET icon_file = '', icon_mime = '', icon_size = 0, icon_updated_at = '', updated_at = ?
		WHERE id = ?`, ts(now), id)
	if err != nil {
		return fmt.Errorf("清理图标文件信息失败：%w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListIconFiles 返回给定 ID 集合中仍关联的图标文件名，用于删除书签后清理文件。
func (s *Store) ListIconFiles(ctx context.Context, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	uniq := uniqIDs(ids)
	args := make([]any, 0, len(uniq))
	for _, id := range uniq {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	rows, err := s.DB.SQL.QueryContext(ctx, `SELECT id, icon_file FROM bookmarks WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("查询图标文件失败：%w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var file string
		if err := rows.Scan(&id, &file); err != nil {
			return nil, fmt.Errorf("读取图标文件失败：%w", err)
		}
		if file != "" {
			out[id] = file
		}
	}
	return out, rows.Err()
}
