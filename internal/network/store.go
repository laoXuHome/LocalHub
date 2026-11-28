package network

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

// Store 封装 network_targets 表的读写。
type Store struct{ DB *db.DB }

const selectTargets = `SELECT target_key, address, enabled, updated_at FROM network_targets`

// Get 返回三个目标的当前配置；迁移保证三行存在。
func (s *Store) Get(ctx context.Context) (Config, error) {
	rows, err := s.DB.SQL.QueryContext(ctx, selectTargets+`
		WHERE target_key IN ('router', 'bypass_router', 'nas')
		ORDER BY CASE target_key
			WHEN 'router' THEN 0
			WHEN 'bypass_router' THEN 1
			ELSE 2 END`)
	if err != nil {
		return Config{}, fmt.Errorf("读取网络探测配置失败：%w", err)
	}
	defer rows.Close()

	items := make(map[TargetKey]Target, 3)
	latest := ""
	for rows.Next() {
		var key, address, updated string
		var enabled int
		if err := rows.Scan(&key, &address, &enabled, &updated); err != nil {
			return Config{}, fmt.Errorf("解析网络探测配置失败：%w", err)
		}
		k := TargetKey(key)
		if _, ok := targetLabels[k]; !ok {
			continue
		}
		items[k] = Target{Key: k, Label: targetLabels[k], Address: address, Enabled: enabled == 1}
		if updated > latest {
			latest = updated
		}
	}
	if err := rows.Err(); err != nil {
		return Config{}, fmt.Errorf("读取网络探测配置失败：%w", err)
	}
	if len(items) != len(targetOrder) {
		return Config{}, fmt.Errorf("网络探测配置不完整：期望 3 个目标，实际 %d 个", len(items))
	}

	out := make([]Target, 0, len(targetOrder))
	for _, key := range targetOrder {
		out = append(out, items[key])
	}
	return Config{Targets: out, UpdatedAt: latest}, nil
}

// Save 在单个事务内全量替换三个目标；校验在事务外完成，避免部分写入。
func (s *Store) Save(ctx context.Context, in Input, now time.Time) (Config, error) {
	targets, err := Normalize(in)
	if err != nil {
		return Config{}, err
	}

	tx, err := s.DB.SQL.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return Config{}, fmt.Errorf("开启网络配置事务失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stamp := formatTime(now)
	for _, t := range targets {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO network_targets (target_key, address, enabled, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(target_key) DO UPDATE SET
				address = excluded.address,
				enabled = excluded.enabled,
				updated_at = excluded.updated_at`,
			string(t.Key), t.Address, boolInt(t.Enabled), stamp); err != nil {
			return Config{}, fmt.Errorf("保存网络探测配置失败：%w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Config{}, fmt.Errorf("提交网络探测配置失败：%w", err)
	}

	// 从数据库重新读取，确保返回值反映真实持久化结果。
	cfg, err := s.Get(ctx)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// GetByKey 是测试与后续探测引擎使用的单目标读取入口。
func (s *Store) GetByKey(ctx context.Context, key TargetKey) (Target, error) {
	if _, ok := targetLabels[key]; !ok {
		return Target{}, ErrInvalid
	}
	var address, updated string
	var enabled int
	err := s.DB.SQL.QueryRowContext(ctx, `SELECT address, enabled, updated_at FROM network_targets WHERE target_key = ?`, string(key)).
		Scan(&address, &enabled, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, fmt.Errorf("探测目标不存在：%s", key)
	}
	if err != nil {
		return Target{}, fmt.Errorf("读取探测目标失败：%w", err)
	}
	return Target{Key: key, Label: targetLabels[key], Address: address, Enabled: enabled == 1}, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
