package bookmark

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LayoutSize 是卡片尺寸的受控枚举，和前端 CardSize 保持一致。
type LayoutSize string

const (
	LayoutLarge  LayoutSize = "large"
	LayoutMedium LayoutSize = "medium"
	LayoutSmall  LayoutSize = "small"
)

// LayoutItem 是一条设备维度布局；数组顺序即最终展示顺序。
type LayoutItem struct {
	ID       int64      `json:"id"`
	Position int        `json:"position"`
	Size     LayoutSize `json:"size"`
}

// Layout 保存两个独立设备的布局。
type Layout struct {
	Desktop []LayoutItem `json:"desktop"`
	Mobile  []LayoutItem `json:"mobile"`
}

const maxLayoutItems = 200

// ValidLayoutSize 校验尺寸枚举。
func ValidLayoutSize(size LayoutSize) bool {
	return size == LayoutLarge || size == LayoutMedium || size == LayoutSmall
}

// ValidateLayout 在写库前校验重复 ID、尺寸和数量；不存在性由 ReplaceLayouts 再校验。
func ValidateLayout(layout Layout) error {
	for device, items := range map[string][]LayoutItem{"desktop": layout.Desktop, "mobile": layout.Mobile} {
		if len(items) > maxLayoutItems {
			return fmt.Errorf("%s 布局最多 %d 项", device, maxLayoutItems)
		}
		seen := make(map[int64]struct{}, len(items))
		for _, item := range items {
			if item.ID <= 0 {
				return fmt.Errorf("%s 布局包含非法书签 ID", device)
			}
			if !ValidLayoutSize(item.Size) {
				return fmt.Errorf("%s 布局包含非法尺寸 %q", device, item.Size)
			}
			if _, ok := seen[item.ID]; ok {
				return fmt.Errorf("%s 布局包含重复书签 ID %d", device, item.ID)
			}
			seen[item.ID] = struct{}{}
		}
	}
	return nil
}

func normalizeLayout(layout Layout) Layout {
	if layout.Desktop == nil {
		layout.Desktop = []LayoutItem{}
	}
	if layout.Mobile == nil {
		layout.Mobile = []LayoutItem{}
	}
	return layout
}

// GetLayout 返回两个设备的布局；无数据时返回空数组，前端自行回退到书籍默认顺序。
func (s *Store) GetLayout(ctx context.Context) (Layout, error) {
	out := Layout{Desktop: []LayoutItem{}, Mobile: []LayoutItem{}}
	rows, err := s.DB.SQL.QueryContext(ctx, `
		SELECT device, bookmark_id, position, size
		FROM bookmark_layouts
		ORDER BY device ASC, position ASC, bookmark_id ASC`)
	if err != nil {
		return out, fmt.Errorf("查询首页布局失败：%w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var device string
		var item LayoutItem
		if err := rows.Scan(&device, &item.ID, &item.Position, &item.Size); err != nil {
			return out, fmt.Errorf("读取首页布局失败：%w", err)
		}
		switch device {
		case "desktop":
			out.Desktop = append(out.Desktop, item)
		case "mobile":
			out.Mobile = append(out.Mobile, item)
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("读取首页布局失败：%w", err)
	}
	return out, nil
}

// ReplaceLayouts 原子替换两个设备的布局；既有的 bookmarks.sort_order 不受影响。
func (s *Store) ReplaceLayouts(ctx context.Context, layout Layout, now time.Time) error {
	if err := ValidateLayout(layout); err != nil {
		return err
	}
	layout = normalizeLayout(layout)
	if err := s.ensureBookmarksExist(ctx, append(append([]LayoutItem{}, layout.Desktop...), layout.Mobile...)); err != nil {
		return err
	}

	tx, err := s.DB.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启布局事务失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_layouts`); err != nil {
		return fmt.Errorf("清理旧布局失败：%w", err)
	}
	insert := func(device string, items []LayoutItem) error {
		for index, item := range items {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO bookmark_layouts (device, bookmark_id, position, size, updated_at)
				VALUES (?, ?, ?, ?, ?)`, device, item.ID, index, string(item.Size), ts(now)); err != nil {
				return fmt.Errorf("写入 %s 布局失败：%w", device, err)
			}
		}
		return nil
	}
	if err := insert("desktop", layout.Desktop); err != nil {
		return err
	}
	if err := insert("mobile", layout.Mobile); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交布局失败：%w", err)
	}
	return nil
}

func (s *Store) ensureBookmarksExist(ctx context.Context, items []LayoutItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	for _, item := range items {
		ids = append(ids, "?")
		args = append(args, item.ID)
	}
	rows, err := s.DB.SQL.QueryContext(ctx, `SELECT id FROM bookmarks WHERE id IN (`+strings.Join(ids, ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("校验布局书签失败：%w", err)
	}
	defer rows.Close()
	found := make(map[int64]struct{}, len(items))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("读取布局书签失败：%w", err)
		}
		found[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取布局书签失败：%w", err)
	}
	for _, item := range items {
		if _, ok := found[item.ID]; !ok {
			return fmt.Errorf("布局中的书签 %d 不存在", item.ID)
		}
	}
	return nil
}

// DeleteForBookmark 清理单个书签的布局行，删除书签时可同步调用。
func (s *Store) DeleteForBookmark(ctx context.Context, id int64) error {
	_, err := s.DB.SQL.ExecContext(ctx, `DELETE FROM bookmark_layouts WHERE bookmark_id = ?`, id)
	if err != nil {
		return fmt.Errorf("清理书签布局失败：%w", err)
	}
	return nil
}
