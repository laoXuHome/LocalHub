package probe

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/network"
)

type fakeProber struct {
	mu      sync.Mutex
	calls   []network.Target
	block   <-chan struct{}
	results map[network.TargetKey]Result
}

func (f *fakeProber) Probe(ctx context.Context, target network.Target) Result {
	f.mu.Lock()
	f.calls = append(f.calls, target)
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return Result{Key: target.Key, Label: target.Label, Address: target.Address, Enabled: true,
				Status: StatusTimeout, CheckedAt: formatTime(time.Now())}
		}
	}
	if result, ok := f.results[target.Key]; ok {
		result.Label = target.Label
		result.Address = target.Address
		result.Enabled = target.Enabled
		return result
	}
	return Result{Key: target.Key, Label: target.Label, Address: target.Address, Enabled: true,
		Status: StatusSuccess, HTTPStatus: 200, LatencyMS: 10, CheckedAt: formatTime(time.Now())}
}

func (f *fakeProber) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestManagerDeduplicatesAndPersistsRun(t *testing.T) {
	d := openProbeDB(t, filepath.Join(t.TempDir(), "manager.db"))
	defer d.Close()
	ctx := context.Background()
	if _, err := (&network.Store{DB: d}).Save(ctx, network.Input{Targets: []network.TargetInput{
		{Key: network.TargetRouter, Address: "router.test", Enabled: true},
		{Key: network.TargetBypassRouter, Address: "bypass.test", Enabled: true},
		{Key: network.TargetNAS, Address: "nas.test", Enabled: true},
	}}, time.Now()); err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}

	block := make(chan struct{})
	fake := &fakeProber{
		block: block,
		results: map[network.TargetKey]Result{
			network.TargetRouter:       {Status: StatusSuccess, HTTPStatus: 200, LatencyMS: 8, CheckedAt: "2026-11-28T02:10:00Z"},
			network.TargetBypassRouter: {Status: StatusTimeout, FailureInfo: "模拟超时", CheckedAt: "2026-11-28T02:10:00Z"},
			network.TargetNAS:          {Status: StatusHTTPError, HTTPStatus: 500, FailureInfo: "模拟 HTTP 500", CheckedAt: "2026-11-28T02:10:00Z"},
		},
	}
	m := NewManager(d)
	m.Prober = fake
	run, err := m.Start(ctx)
	if err != nil {
		t.Fatalf("启动探测失败：%v", err)
	}
	if run.Status != "running" || run.ID == "" {
		t.Fatalf("启动响应错误：%+v", run)
	}

	second, err := m.Start(ctx)
	if !errors.Is(err, ErrInProgress) || second.ID != run.ID {
		t.Fatalf("重复触发必须被拒绝并返回同一执行记录：run=%+v err=%v", second, err)
	}

	close(block)
	var finished Run
	for i := 0; i < 40; i++ {
		got, ok := m.Run(run.ID)
		if ok && got.Status == "completed" {
			finished = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if finished.Status != "completed" || len(finished.Results) != 3 {
		t.Fatalf("探测未完成或结果数量错误：%+v", finished)
	}
	if finished.Results[0].Status != StatusSuccess || finished.Results[1].Status != StatusTimeout || finished.Results[2].Status != StatusHTTPError {
		t.Fatalf("单次结果状态错误：%+v", finished.Results)
	}
	if fake.callCount() != 3 {
		t.Fatalf("三个启用目标应各请求一次，got %d", fake.callCount())
	}

	latest, err := (&ResultStore{DB: d}).Latest(ctx)
	if err != nil {
		t.Fatalf("读取持久化结果失败：%v", err)
	}
	if latest.Targets[0].Status != StatusSuccess || latest.Targets[2].HTTPStatus != 500 {
		t.Fatalf("最新结果未持久化：%+v", latest.Targets)
	}
}

func TestManagerSkipsDisabledTargets(t *testing.T) {
	d := openProbeDB(t, filepath.Join(t.TempDir(), "manager.db"))
	defer d.Close()
	ctx := context.Background()
	if _, err := (&network.Store{DB: d}).Save(ctx, network.Input{Targets: []network.TargetInput{
		{Key: network.TargetRouter, Address: "router.test", Enabled: true},
		{Key: network.TargetBypassRouter, Address: "bypass.test", Enabled: false},
		{Key: network.TargetNAS, Address: "nas.test", Enabled: false},
	}}, time.Now()); err != nil {
		t.Fatalf("保存配置失败：%v", err)
	}
	fake := &fakeProber{results: map[network.TargetKey]Result{}}
	m := NewManager(d)
	m.Prober = fake
	run, err := m.Start(ctx)
	if err != nil {
		t.Fatalf("启动探测失败：%v", err)
	}
	var finished Run
	for i := 0; i < 40; i++ {
		got, ok := m.Run(run.ID)
		if ok && got.Status == "completed" {
			finished = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if finished.Status != "completed" || len(finished.Results) != 3 {
		t.Fatalf("探测未完成：%+v", finished)
	}
	if fake.callCount() != 1 {
		t.Fatalf("只有启用目标可以发起请求，got %d", fake.callCount())
	}
	if finished.Results[1].Status != StatusDisabled || finished.Results[2].Status != StatusDisabled {
		t.Fatalf("禁用目标状态错误：%+v", finished.Results)
	}
}
