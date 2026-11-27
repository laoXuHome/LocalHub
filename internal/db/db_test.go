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

	all, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(all) {
		t.Errorf("每个迁移版本应恰好记录一次，期望 %d 条，得到 %d", len(all), count)
	}
	if d.Version != len(all) {
		t.Errorf("版本号应为 %d，得到 %d", len(all), d.Version)
	}

	for _, name := range []string{"app_meta", "admin_users", "sessions", "login_attempts"} {
		var table string
		if err := d.SQL.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&table); err != nil {
			t.Errorf("%s 表未创建：%v", name, err)
		}
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
