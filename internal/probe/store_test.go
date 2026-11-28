package probe

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
	"github.com/laoXuHome/LocalHub/internal/network"
)

func openProbeDB(t *testing.T, path string) *db.DB {
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

func probeConfig(t *testing.T, routerEnabled bool, routerAddress string) network.Config {
	t.Helper()
	in := network.Input{Targets: []network.TargetInput{
		{Key: network.TargetRouter, Address: routerAddress, Enabled: routerEnabled},
		{Key: network.TargetBypassRouter, Address: "bypass.test", Enabled: !routerEnabled},
		{Key: network.TargetNAS, Address: "", Enabled: false},
	}}
	cfg, err := network.Normalize(in)
	if err != nil {
		t.Fatalf("构造测试配置失败：%v", err)
	}
	return network.Config{Targets: cfg}
}

func TestResultPersistsAndSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.db")
	d := openProbeDB(t, path)
	ctx := context.Background()
	cfg := probeConfig(t, true, "router.test")
	if _, err := (&network.Store{DB: d}).Save(ctx, network.Input{Targets: []network.TargetInput{
		{Key: network.TargetRouter, Address: "router.test", Enabled: true},
		{Key: network.TargetBypassRouter, Address: "bypass.test", Enabled: false},
		{Key: network.TargetNAS, Address: "", Enabled: false},
	}}, time.Now()); err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}
	results := []Result{
		{Key: network.TargetRouter, Label: "主路由器", Address: "router.test", Enabled: true,
			Status: StatusSuccess, HTTPStatus: 200, LatencyMS: 12, CheckedAt: "2026-11-28T02:00:00Z"},
		{Key: network.TargetBypassRouter, Label: "旁路由", Address: "bypass.test", Enabled: false, Status: StatusDisabled},
		{Key: network.TargetNAS, Label: "NAS", Address: "", Enabled: false, Status: StatusDisabled},
	}
	if err := (&ResultStore{DB: d}).Save(ctx, cfg, results); err != nil {
		t.Fatalf("保存结果失败：%v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("关闭数据库失败：%v", err)
	}

	reopened := openProbeDB(t, path)
	defer reopened.Close()
	status, err := (&ResultStore{DB: reopened}).Latest(context.Background())
	if err != nil {
		t.Fatalf("重启后读取结果失败：%v", err)
	}
	if len(status.Targets) != 3 || status.Targets[0].Status != StatusSuccess {
		t.Fatalf("重启后结果丢失：%+v", status)
	}
	if status.Targets[0].LatencyMS != 12 || status.Targets[0].CheckedAt != "2026-11-28T02:00:00Z" {
		t.Fatalf("重启后结果字段不一致：%+v", status.Targets[0])
	}
	if status.Targets[1].Status != StatusDisabled || status.Targets[2].Status != StatusDisabled {
		t.Fatalf("禁用目标不能显示为失败：%+v", status.Targets)
	}
}

func TestLatestInvalidatesChangedAndDisabledTargets(t *testing.T) {
	d := openProbeDB(t, filepath.Join(t.TempDir(), "probe.db"))
	defer d.Close()
	ctx := context.Background()
	targets := []network.TargetInput{
		{Key: network.TargetRouter, Address: "router.old", Enabled: true},
		{Key: network.TargetBypassRouter, Address: "bypass.test", Enabled: false},
		{Key: network.TargetNAS, Address: "", Enabled: false},
	}
	store := &network.Store{DB: d}
	cfg, err := store.Save(ctx, network.Input{Targets: targets}, time.Now())
	if err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}
	rs := &ResultStore{DB: d}
	if err := rs.Save(ctx, cfg, []Result{
		{Key: network.TargetRouter, Label: "主路由器", Address: "router.old", Enabled: true,
			Status: StatusSuccess, HTTPStatus: 200, LatencyMS: 5, CheckedAt: "2026-11-28T02:00:00Z"},
		{Key: network.TargetBypassRouter, Label: "旁路由", Enabled: false, Status: StatusDisabled},
		{Key: network.TargetNAS, Label: "NAS", Enabled: false, Status: StatusDisabled},
	}); err != nil {
		t.Fatalf("保存结果失败：%v", err)
	}

	// 地址修改后，旧成功结果不能被当成新目标当前成功。
	targets[0].Address = "router.new"
	cfg, err = store.Save(ctx, network.Input{Targets: targets}, time.Now())
	if err != nil {
		t.Fatalf("修改地址失败：%v", err)
	}
	status, err := rs.Latest(ctx)
	if err != nil {
		t.Fatalf("读取结果失败：%v", err)
	}
	if status.Targets[0].Status != StatusNotProbed {
		t.Fatalf("地址变化后应未探测：%+v", status.Targets[0])
	}
	_ = cfg

	// 禁用目标必须清理旧结果；重新启用后仍是未探测，而不是旧的成功。
	targets[0].Enabled = false
	targets[1].Enabled = true
	cfg, err = store.Save(ctx, network.Input{Targets: targets}, time.Now())
	if err != nil {
		t.Fatalf("禁用主路由失败：%v", err)
	}
	if err := rs.DropDisabled(ctx, cfg); err != nil {
		t.Fatalf("清理禁用结果失败：%v", err)
	}
	status, err = rs.Latest(ctx)
	if err != nil {
		t.Fatalf("禁用后读取结果失败：%v", err)
	}
	if status.Targets[0].Status != StatusDisabled {
		t.Fatalf("禁用目标状态错误：%+v", status.Targets[0])
	}

	targets[0].Enabled = true
	targets[1].Enabled = false
	cfg, err = store.Save(ctx, network.Input{Targets: targets}, time.Now())
	if err != nil {
		t.Fatalf("重新启用主路由失败：%v", err)
	}
	if err := rs.DropDisabled(ctx, cfg); err != nil {
		t.Fatalf("清理旁路由结果失败：%v", err)
	}
	status, err = rs.Latest(ctx)
	if err != nil {
		t.Fatalf("重新启用后读取结果失败：%v", err)
	}
	if status.Targets[0].Status != StatusNotProbed {
		t.Fatalf("重新启用不能复用禁用前成功结果：%+v", status.Targets[0])
	}
}
