package network

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

func openTestDB(t *testing.T, path string) *db.DB {
	t.Helper()
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	return d
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.db")
	d := openTestDB(t, path)
	ctx := context.Background()
	store := &Store{DB: d}

	initial, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("读取初始配置失败：%v", err)
	}
	if len(initial.Targets) != 3 || !initial.Targets[0].Enabled {
		t.Fatalf("初始配置应包含 3 个目标且启用主路由器：%+v", initial)
	}
	if initial.Targets[0].Address == "192.168.1.1" {
		t.Fatal("初始配置不能写死常见用户网段地址")
	}

	want := Input{Targets: []TargetInput{
		{Key: TargetRouter, Address: "10.20.0.1", Enabled: true},
		{Key: TargetBypassRouter, Address: "10.20.0.2", Enabled: true},
		{Key: TargetNAS, Address: "nas.lab", Enabled: false},
	}}
	saved, err := store.Save(ctx, want, time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}
	if saved.Targets[1].Address != "10.20.0.2" || !saved.Targets[1].Enabled {
		t.Fatalf("保存返回值不正确：%+v", saved)
	}

	if err := d.Close(); err != nil {
		t.Fatalf("关闭数据库失败：%v", err)
	}

	// 模拟服务重启：重新打开同一个数据库文件，配置必须仍然存在。
	reopened := openTestDB(t, path)
	defer reopened.Close()
	cfg, err := (&Store{DB: reopened}).Get(context.Background())
	if err != nil {
		t.Fatalf("重启后读取配置失败：%v", err)
	}
	if len(cfg.Targets) != 3 {
		t.Fatalf("重启后目标数量错误：%+v", cfg)
	}
	if cfg.Targets[0].Address != "10.20.0.1" || !cfg.Targets[0].Enabled {
		t.Fatalf("重启后主路由器配置丢失：%+v", cfg.Targets[0])
	}
	if cfg.Targets[1].Address != "10.20.0.2" || !cfg.Targets[1].Enabled {
		t.Fatalf("重启后旁路由配置丢失：%+v", cfg.Targets[1])
	}
	if cfg.Targets[2].Address != "nas.lab" || cfg.Targets[2].Enabled {
		t.Fatalf("重启后 NAS 配置丢失：%+v", cfg.Targets[2])
	}
}

func TestStoreInvalidSaveDoesNotPartiallyWrite(t *testing.T) {
	d := openTestDB(t, filepath.Join(t.TempDir(), "network.db"))
	defer d.Close()
	ctx := context.Background()
	store := &Store{DB: d}
	now := time.Now()

	good := Input{Targets: []TargetInput{
		{Key: TargetRouter, Address: "10.0.0.1", Enabled: true},
		{Key: TargetBypassRouter, Address: "", Enabled: false},
		{Key: TargetNAS, Address: "nas.home", Enabled: true},
	}}
	if _, err := store.Save(ctx, good, now); err != nil {
		t.Fatalf("保存合法配置失败：%v", err)
	}

	allDisabled := Input{Targets: []TargetInput{
		{Key: TargetRouter, Address: "10.9.9.9", Enabled: false},
		{Key: TargetBypassRouter, Address: "10.9.9.8", Enabled: false},
		{Key: TargetNAS, Address: "nas.new", Enabled: false},
	}}
	if _, err := store.Save(ctx, allDisabled, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("全部禁用应返回 ErrInvalid，got %v", err)
	}
	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("读取配置失败：%v", err)
	}
	if cfg.Targets[0].Address != "10.0.0.1" || !cfg.Targets[0].Enabled {
		t.Fatalf("非法保存不应改动已有配置：%+v", cfg)
	}
	if cfg.Targets[2].Address != "nas.home" || !cfg.Targets[2].Enabled {
		t.Fatalf("非法保存不应改动 NAS 配置：%+v", cfg)
	}
}
