-- 0002_auth：P1.1 账户与权限基础。
-- 只建立单管理员、会话与登录失败记录，不创建书签等业务表。

CREATE TABLE IF NOT EXISTS admin_users (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    username           TEXT NOT NULL UNIQUE,
    password_hash      TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    password_changed_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    admin_id     INTEGER NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ip           TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    revoked_at   TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (admin_id) REFERENCES admin_users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_admin ON sessions(admin_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- 登录失败记录：用于按 IP + 用户名统计窗口内失败次数，实现限速与锁定。
CREATE TABLE IF NOT EXISTS login_attempts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    ip           TEXT NOT NULL,
    username     TEXT NOT NULL,
    success      INTEGER NOT NULL,
    attempted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_login_attempts_key ON login_attempts(ip, username, attempted_at);
