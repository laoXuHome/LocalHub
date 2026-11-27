package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("首次迁移失败：%v", err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("重复迁移失败：%v", err)
	}

	var count int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("重复启动不应重复记录迁移，期望 1 条，得到 %d", count)
	}

	var table string
	if err := d.SQL.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='app_meta'`).Scan(&table); err != nil {
		t.Errorf("app_meta 表未创建：%v", err)
	}
}

func TestPing(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "ping.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Ping(context.Background()); err != nil {
		t.Errorf("Ping 应成功：%v", err)
	}
}
