package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
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

func TestFolderMigrationPreservesP22Rows(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if _, err := d.SQL.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("创建迁移表失败：%v", err)
	}
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 P2.2 已验收的 schema v4：只应用 0001-0004。
	for _, m := range migs {
		if m.Version > 4 {
			continue
		}
		if err := d.applyOne(ctx, m); err != nil {
			t.Fatalf("应用历史迁移 %s 失败：%v", m.Name, err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := d.SQL.ExecContext(ctx, `INSERT INTO bookmarks
		(name, wan_url, icon_type, icon_file, icon_mime, icon_size, icon_updated_at, enabled, visibility, open_mode, sort_order, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"旧书签", "https://old.example.com", "file", "1-a.png", "image/png", 70, now, 1, "public", "new_tab", 0, now, now); err != nil {
		t.Fatalf("插入 P2.2 旧数据失败：%v", err)
	}
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("升级到 0005 失败：%v", err)
	}
	if d.Version != len(migs) {
		t.Fatalf("迁移版本应为 %d，得到 %d", len(migs), d.Version)
	}
	var count int
	var folder string
	if err := d.SQL.QueryRowContext(ctx, `SELECT folder_path FROM bookmarks WHERE name='旧书签'`).Scan(&folder); err != nil {
		t.Fatalf("读取旧数据失败：%v", err)
	}
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || folder != "[]" {
		t.Fatalf("旧数据应保留且层级默认空：count=%d folder=%q", count, folder)
	}
}
