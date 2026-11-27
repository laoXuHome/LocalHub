package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

// ErrSessionInvalid 表示令牌不存在、已吊销或已过期。
var ErrSessionInvalid = errors.New("session invalid")

// Session 是服务端会话记录（令牌明文只存在于浏览器 Cookie）。
type Session struct {
	ID        string
	AdminID   int64
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
	IP        string
	UserAgent string
}

// Manager 管理会话的创建、校验与吊销。
type Manager struct {
	DB  *db.DB
	TTL time.Duration
}

// Create 创建会话，返回明文令牌（写入 Cookie）与会话信息。
func (m *Manager) Create(ctx context.Context, adminID int64, ip, ua string, now time.Time) (string, *Session, error) {
	// 时间精度对齐到秒，保证内存对象与库中记录一致（便于校验与对账）。
	now = now.Truncate(time.Second)
	plain, hashed, err := HashToken()
	if err != nil {
		return "", nil, err
	}
	id := hashed[:32]
	expires := now.Add(m.ttl()).Truncate(time.Second)
	_, err = m.DB.SQL.ExecContext(ctx,
		`INSERT INTO sessions (id, admin_id, token_hash, created_at, expires_at, last_seen_at, ip, user_agent, revoked_at)
		 VALUES (?,?,?,?,?,?,?,?, '')`,
		id, adminID, hashed, nowText(now), nowText(expires), nowText(now), ip, truncate(ua, 256))
	if err != nil {
		return "", nil, fmt.Errorf("创建会话失败：%w", err)
	}
	return plain, &Session{ID: id, AdminID: adminID, CreatedAt: now, ExpiresAt: expires, LastSeen: now, IP: ip, UserAgent: truncate(ua, 256)}, nil
}

func (m *Manager) ttl() time.Duration {
	if m.TTL <= 0 {
		return 12 * time.Hour
	}
	return m.TTL
}

// Get 校验令牌；有效时返回会话，已过期/已吊销/不存在一律 ErrSessionInvalid。
func (m *Manager) Get(ctx context.Context, plainToken string, now time.Time) (*Session, error) {
	if plainToken == "" {
		return nil, ErrSessionInvalid
	}
	hash := HashTokenString(plainToken)
	var (
		sess              Session
		created, expires  string
		lastSeen, revoked string
	)
	err := m.DB.SQL.QueryRowContext(ctx, `
		SELECT s.id, s.admin_id, a.username, s.created_at, s.expires_at, s.last_seen_at, s.ip, s.user_agent, s.revoked_at
		FROM sessions s JOIN admin_users a ON a.id = s.admin_id
		WHERE s.token_hash = ?`, hash).
		Scan(&sess.ID, &sess.AdminID, &sess.Username, &created, &expires, &lastSeen, &sess.IP, &sess.UserAgent, &revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionInvalid
		}
		return nil, fmt.Errorf("读取会话失败：%w", err)
	}
	sess.CreatedAt = parseText(created)
	sess.ExpiresAt = parseText(expires)
	sess.LastSeen = parseText(lastSeen)
	if revoked != "" {
		return nil, ErrSessionInvalid
	}
	if now.After(sess.ExpiresAt) {
		return nil, ErrSessionInvalid
	}
	if now.Sub(sess.LastSeen) > time.Minute {
		_, _ = m.DB.SQL.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, nowText(now), sess.ID)
		sess.LastSeen = now
	}
	return &sess, nil
}

// Revoke 吊销单个会话（退出登录）。
func (m *Manager) Revoke(ctx context.Context, plainToken string, now time.Time) error {
	_, err := m.DB.SQL.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE token_hash = ? AND revoked_at = ''`,
		nowText(now), HashTokenString(plainToken))
	if err != nil {
		return fmt.Errorf("吊销会话失败：%w", err)
	}
	return nil
}

// RevokeOthers 吊销该管理员的其他会话（改密后强制其他设备重新登录）。
func (m *Manager) RevokeOthers(ctx context.Context, adminID int64, keepToken string, now time.Time) (int64, error) {
	res, err := m.DB.SQL.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE admin_id = ? AND token_hash <> ? AND revoked_at = ''`,
		nowText(now), adminID, HashTokenString(keepToken))
	if err != nil {
		return 0, fmt.Errorf("吊销其他会话失败：%w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// RevokeAll 吊销该管理员全部会话。
func (m *Manager) RevokeAll(ctx context.Context, adminID int64, now time.Time) (int64, error) {
	res, err := m.DB.SQL.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE admin_id = ? AND revoked_at = ''`, nowText(now), adminID)
	if err != nil {
		return 0, fmt.Errorf("吊销全部会话失败：%w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PurgeExpired 清理过期或已吊销的会话记录。
func (m *Manager) PurgeExpired(ctx context.Context, now time.Time) (int64, error) {
	cutoff := nowText(now.Add(-24 * time.Hour))
	res, err := m.DB.SQL.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE expires_at < ? OR (revoked_at <> '' AND revoked_at < ?)`, nowText(now), cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理会话失败：%w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CountActive 返回该管理员有效会话数量。
func (m *Manager) CountActive(ctx context.Context, adminID int64, now time.Time) (int, error) {
	var n int
	err := m.DB.SQL.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE admin_id = ? AND revoked_at = '' AND expires_at > ?`,
		adminID, nowText(now)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计会话失败：%w", err)
	}
	return n, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
