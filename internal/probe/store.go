package probe

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/laoXuHome/LocalHub/internal/db"
	"github.com/laoXuHome/LocalHub/internal/network"
)

// ResultStore 持久化每个目标最近一次真实探测结果，与 network_targets 配置分表。
type ResultStore struct{ DB *db.DB }

type storedResult struct {
	Address    string
	Status     Status
	HTTPStatus int
	LatencyMS  int64
	CheckedAt  string
	Reason     string
}

// Save 在一个事务中写入本次真实探测结果，并删除禁用目标的旧结果。
// 只允许写入启用目标；禁用目标不发起请求，也不保留可误读的成功记录。
func (s *ResultStore) Save(ctx context.Context, cfg network.Config, results []Result) error {
	tx, err := s.DB.SQL.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("开启探测结果事务失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, r := range results {
		if !r.Enabled {
			if _, err := tx.ExecContext(ctx, `DELETE FROM network_probe_results WHERE target_key = ?`, string(r.Key)); err != nil {
				return fmt.Errorf("删除禁用目标结果失败：%w", err)
			}
			continue
		}
		if !r.Status.persisted() {
			return fmt.Errorf("拒绝持久化非法探测状态：%s", r.Status)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO network_probe_results
				(target_key, address, status, http_status, latency_ms, checked_at, reason)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(target_key) DO UPDATE SET
				address = excluded.address,
				status = excluded.status,
				http_status = excluded.http_status,
				latency_ms = excluded.latency_ms,
				checked_at = excluded.checked_at,
				reason = excluded.reason`,
			string(r.Key), r.Address, string(r.Status), r.HTTPStatus, r.LatencyMS,
			r.CheckedAt, r.FailureInfo); err != nil {
			return fmt.Errorf("保存探测结果失败：%w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交探测结果失败：%w", err)
	}
	return nil
}

// DropDisabled 在管理员保存配置后删除已禁用目标的旧结果。
// 若之后重新启用，会重新回到“未探测”，不会把禁用前的成功结果当成当前状态。
func (s *ResultStore) DropDisabled(ctx context.Context, cfg network.Config) error {
	stmt, err := s.DB.SQL.PrepareContext(ctx, `DELETE FROM network_probe_results WHERE target_key = ?`)
	if err != nil {
		return fmt.Errorf("准备清理探测结果失败：%w", err)
	}
	defer stmt.Close()
	for _, t := range cfg.Targets {
		if t.Enabled {
			continue
		}
		if _, err := stmt.ExecContext(ctx, string(t.Key)); err != nil {
			return fmt.Errorf("清理禁用目标结果失败：%w", err)
		}
	}
	return nil
}

// Latest 读取三个目标的最新结果并与当前配置合并。
// 地址改过、从未探测、已禁用的目标不会被历史结果误报为当前成功。
func (s *ResultStore) Latest(ctx context.Context) (StatusResponse, error) {
	cfg, err := (&network.Store{DB: s.DB}).Get(ctx)
	if err != nil {
		return StatusResponse{}, err
	}

	rows, err := s.DB.SQL.QueryContext(ctx, `
		SELECT target_key, address, status, http_status, latency_ms, checked_at, reason
		FROM network_probe_results`)
	if err != nil {
		return StatusResponse{}, fmt.Errorf("读取探测结果失败：%w", err)
	}
	defer rows.Close()

	stored := make(map[network.TargetKey]storedResult, 3)
	for rows.Next() {
		var key, address, status, checkedAt, reason string
		var httpStatus int
		var latencyMS int64
		if err := rows.Scan(&key, &address, &status, &httpStatus, &latencyMS, &checkedAt, &reason); err != nil {
			return StatusResponse{}, fmt.Errorf("解析探测结果失败：%w", err)
		}
		stored[network.TargetKey(key)] = storedResult{
			Address: address, Status: Status(status), HTTPStatus: httpStatus,
			LatencyMS: latencyMS, CheckedAt: checkedAt, Reason: reason,
		}
	}
	if err := rows.Err(); err != nil {
		return StatusResponse{}, fmt.Errorf("读取探测结果失败：%w", err)
	}

	out := StatusResponse{Targets: make([]Result, 0, len(cfg.Targets))}
	for _, target := range cfg.Targets {
		result := Result{
			Key: target.Key, Label: target.Label, Address: target.Address, Enabled: target.Enabled,
			Status: StatusNotProbed,
		}
		if !target.Enabled {
			result.Status = StatusDisabled
			out.Targets = append(out.Targets, result)
			continue
		}
		row, ok := stored[target.Key]
		// 地址变更后旧结果不再属于当前目标；必须重新探测。
		if ok && row.Address == target.Address {
			result.Status = row.Status
			result.HTTPStatus = row.HTTPStatus
			result.LatencyMS = row.LatencyMS
			result.CheckedAt = row.CheckedAt
			result.FailureInfo = row.Reason
			if row.CheckedAt > out.UpdatedAt {
				out.UpdatedAt = row.CheckedAt
			}
		}
		out.Targets = append(out.Targets, result)
	}
	return out, nil
}
