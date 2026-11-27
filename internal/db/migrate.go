package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	Version int
	Name    string
	SQL     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("迁移文件名不符合 NNNN_name.sql：%s", e.Name())
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("迁移文件版本号非法：%s", e.Name())
		}
		body, err := migrationsFS.ReadFile(path.Join("migrations", e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{Version: v, Name: base, SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("迁移版本号重复：%d", out[i].Version)
		}
	}
	return out, nil
}

// Migrate 执行版本化迁移，幂等：已应用的版本不会重复执行。
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.SQL.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建 schema_migrations 失败：%w", err)
	}

	migs, err := loadMigrations()
	if err != nil {
		return err
	}

	applied := map[int]bool{}
	rows, err := d.SQL.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("读取已应用迁移失败：%w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, m := range migs {
		if applied[m.Version] {
			continue
		}
		if err := d.applyOne(ctx, m); err != nil {
			return err
		}
	}
	d.Version = len(migs)
	return nil
}

func (d *DB) applyOne(ctx context.Context, m migration) error {
	tx, err := d.SQL.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("迁移 %s 开启事务失败：%w", m.Name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("迁移 %s 执行失败：%w", m.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?,?,?)`,
		m.Version, m.Name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("迁移 %s 记录失败：%w", m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("迁移 %s 提交失败：%w", m.Name, err)
	}
	return nil
}
