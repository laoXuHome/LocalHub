package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

// Limiter 基于 login_attempts 表实现“窗口内失败次数 + 锁定时长”。
type Limiter struct {
	DB          *db.DB
	MaxFailures int
	Window      time.Duration
	Lockout     time.Duration
}

func (l *Limiter) max() int {
	if l.MaxFailures <= 0 {
		return 5
	}
	return l.MaxFailures
}

func (l *Limiter) window() time.Duration {
	if l.Window <= 0 {
		return 15 * time.Minute
	}
	return l.Window
}

func (l *Limiter) lockout() time.Duration {
	if l.Lockout <= 0 {
		return 15 * time.Minute
	}
	return l.Lockout
}

// Locked 返回是否处于锁定期以及剩余时长。
func (l *Limiter) Locked(ctx context.Context, ip, username string, now time.Time) (bool, time.Duration, error) {
	since := nowText(now.Add(-l.window()))
	var failures int
	var lastText string
	err := l.DB.SQL.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MAX(attempted_at), '')
		FROM login_attempts
		WHERE ip = ? AND username = ? AND success = 0 AND attempted_at >= ?`,
		ip, username, since).Scan(&failures, &lastText)
	if err != nil {
		return false, 0, fmt.Errorf("读取登录失败记录失败：%w", err)
	}
	if failures < l.max() || lastText == "" {
		return false, 0, nil
	}
	last := parseText(lastText)
	until := last.Add(l.lockout())
	if now.Before(until) {
		return true, until.Sub(now), nil
	}
	// 锁定期已结束：清除旧记录，允许重新尝试。
	_, err = l.DB.SQL.ExecContext(ctx, `DELETE FROM login_attempts WHERE ip = ? AND username = ?`, ip, username)
	if err != nil {
		return false, 0, fmt.Errorf("清理登录失败记录失败：%w", err)
	}
	return false, 0, nil
}

// Record 记录一次登录尝试，并顺带清理窗口外的旧记录。
func (l *Limiter) Record(ctx context.Context, ip, username string, success bool, now time.Time) error {
	v := 0
	if success {
		v = 1
	}
	_, err := l.DB.SQL.ExecContext(ctx,
		`INSERT INTO login_attempts (ip, username, success, attempted_at) VALUES (?,?,?,?)`,
		ip, username, v, nowText(now))
	if err != nil {
		return fmt.Errorf("记录登录尝试失败：%w", err)
	}
	_, _ = l.DB.SQL.ExecContext(ctx,
		`DELETE FROM login_attempts WHERE attempted_at < ?`, nowText(now.Add(-l.window())))
	return nil
}

// Clear 清除该键的失败记录（登录成功后调用）。
func (l *Limiter) Clear(ctx context.Context, ip, username string) error {
	_, err := l.DB.SQL.ExecContext(ctx,
		`DELETE FROM login_attempts WHERE ip = ? AND username = ? AND success = 0`, ip, username)
	if err != nil {
		return fmt.Errorf("清除登录失败记录失败：%w", err)
	}
	return nil
}
