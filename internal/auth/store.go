package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

// ErrNoAdmin 表示尚未初始化管理员账户。
var ErrNoAdmin = errors.New("admin not initialized")

// Admin 是单管理员账户。
type Admin struct {
	ID                int64
	Username          string
	PasswordHash      string
	CreatedAt         time.Time
	PasswordChangedAt time.Time
}

// Store 封装 admin_users 表的读写。
type Store struct{ DB *db.DB }

func nowText(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseText(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CountAdmins 返回账户数量，用于判定是否需要初始化。
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.DB.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计管理员失败：%w", err)
	}
	return n, nil
}

// CreateAdmin 创建唯一管理员；已存在其他账户时拒绝，保证单管理员约束。
func (s *Store) CreateAdmin(ctx context.Context, username, hash string, now time.Time) error {
	n, err := s.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("已存在管理员，不允许再创建（单管理员约束）")
	}
	_, err = s.DB.SQL.ExecContext(ctx,
		`INSERT INTO admin_users (username, password_hash, created_at, updated_at, password_changed_at) VALUES (?,?,?,?,?)`,
		username, hash, nowText(now), nowText(now), nowText(now))
	if err != nil {
		return fmt.Errorf("创建管理员失败：%w", err)
	}
	return nil
}

// GetByUsername 按用户名读取管理员。
func (s *Store) GetByUsername(ctx context.Context, username string) (*Admin, error) {
	row := s.DB.SQL.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at, password_changed_at FROM admin_users WHERE username = ?`, username)
	a := &Admin{}
	var created, changed string
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &created, &changed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoAdmin
		}
		return nil, fmt.Errorf("读取管理员失败：%w", err)
	}
	a.CreatedAt = parseText(created)
	a.PasswordChangedAt = parseText(changed)
	return a, nil
}

// GetByID 按 id 读取管理员。
func (s *Store) GetByID(ctx context.Context, id int64) (*Admin, error) {
	row := s.DB.SQL.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at, password_changed_at FROM admin_users WHERE id = ?`, id)
	a := &Admin{}
	var created, changed string
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &created, &changed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoAdmin
		}
		return nil, fmt.Errorf("读取管理员失败：%w", err)
	}
	a.CreatedAt = parseText(created)
	a.PasswordChangedAt = parseText(changed)
	return a, nil
}

// UpdatePassword 更新口令哈希。
func (s *Store) UpdatePassword(ctx context.Context, id int64, hash string, now time.Time) error {
	_, err := s.DB.SQL.ExecContext(ctx,
		`UPDATE admin_users SET password_hash = ?, updated_at = ?, password_changed_at = ? WHERE id = ?`,
		hash, nowText(now), nowText(now), id)
	if err != nil {
		return fmt.Errorf("更新口令失败：%w", err)
	}
	return nil
}

// NormalizeUsername 统一用户名格式（去空格、长度校验）。
func NormalizeUsername(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("用户名不能为空")
	}
	if len([]byte(s)) > 64 {
		return "", fmt.Errorf("用户名过长")
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return "", fmt.Errorf("用户名不能包含空白字符")
	}
	return s, nil
}
