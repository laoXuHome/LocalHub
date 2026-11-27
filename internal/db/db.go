package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 驱动，无 CGO
)

// DB 封装 P0 阶段的最小数据库连接与迁移状态。
type DB struct {
	SQL     *sql.DB
	Path    string
	Version int
}

// Open 打开数据库。数据库文件必须位于数据目录内（由 config 校验）。
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %q 失败：%w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("连接数据库 %q 失败：%w", path, err)
	}
	return &DB{SQL: sqlDB, Path: path}, nil
}

// Ping 供健康检查使用。
func (d *DB) Ping(ctx context.Context) error {
	if d == nil || d.SQL == nil {
		return fmt.Errorf("数据库未初始化")
	}
	return d.SQL.PingContext(ctx)
}

// Close 关闭连接。
func (d *DB) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}
