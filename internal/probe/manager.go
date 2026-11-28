package probe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
	"github.com/laoXuHome/LocalHub/internal/network"
)

const (
	// MaxConcurrentProbes 限制一次探测的并发数；目标固定为三个。
	MaxConcurrentProbes = 3
	// RunTimeout 是单次手动探测的整体上限，包含三个并行目标。
	RunTimeout = 8 * time.Second
	// maxRunHistory 是进程内保留的单次执行记录数量。
	maxRunHistory = 20
)

var (
	// ErrInProgress 表示已有探测在执行，禁止堆积新任务。
	ErrInProgress = errors.New("已有探测正在进行")
)

// Manager 负责手动触发探测、限制并发、记录单次执行结果。
// 最新结果由 ResultStore 持久化；执行记录只在进程内保留。
type Manager struct {
	DB     *db.DB
	Prober Prober

	mu      sync.Mutex
	runs    map[string]Run
	order   []string
	running string
}

func NewManager(database *db.DB) *Manager {
	return &Manager{
		DB: database, Prober: NewHTTPProber(),
		runs: make(map[string]Run),
	}
}

// Start 读取当前 P4.1 配置并异步执行一次探测。
// 不接受 URL 输入；目标只能来自 network_targets 的三个固定 key。
func (m *Manager) Start(ctx context.Context) (Run, error) {
	if m.DB == nil || m.Prober == nil {
		return Run{}, fmt.Errorf("探测服务不可用")
	}
	cfg, err := (&network.Store{DB: m.DB}).Get(ctx)
	if err != nil {
		return Run{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running != "" {
		if run, ok := m.runs[m.running]; ok {
			return run, ErrInProgress
		}
		m.running = ""
	}
	id := newRunID()
	run := Run{
		ID: id, Status: "running", StartedAt: formatTime(time.Now()),
		Results: make([]Result, 0, len(cfg.Targets)),
	}
	m.runs[id] = run
	m.order = append(m.order, id)
	m.running = id
	m.trimHistory()

	go m.execute(cfg, id)
	return run, nil
}

// Run 返回一次手动探测的执行结果；不存在返回 false。
func (m *Manager) Run(id string) (Run, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	if !ok {
		return Run{}, false
	}
	return cloneRun(run), true
}

func (m *Manager) execute(cfg network.Config, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), RunTimeout)
	defer cancel()

	results := make([]Result, len(cfg.Targets))
	sem := make(chan struct{}, MaxConcurrentProbes)
	var wg sync.WaitGroup
	for i, target := range cfg.Targets {
		if !target.Enabled {
			results[i] = Result{
				Key: target.Key, Label: target.Label, Address: target.Address,
				Enabled: false, Status: StatusDisabled,
			}
			continue
		}
		wg.Add(1)
		go func(index int, item network.Target) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[index] = Result{
					Key: item.Key, Label: item.Label, Address: item.Address, Enabled: true,
					Status: StatusTimeout, FailureInfo: "探测超时", CheckedAt: formatTime(time.Now()),
				}
				return
			}
			res := m.Prober.Probe(ctx, item)
			// 探测结果必须绑定到当前配置目标，不能被替换或错配。
			res.Key = item.Key
			res.Label = item.Label
			res.Address = item.Address
			res.Enabled = item.Enabled
			results[index] = res
		}(i, target)
	}
	wg.Wait()

	// 结果写入使用独立超时；整体探测超时不应导致已取得的结果无法落库。
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	_ = (&ResultStore{DB: m.DB}).Save(saveCtx, cfg, results)

	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.runs[id]
	run.Status = "completed"
	run.FinishedAt = formatTime(time.Now())
	run.Results = results
	m.runs[id] = run
	if m.running == id {
		m.running = ""
	}
}

func (m *Manager) trimHistory() {
	if len(m.order) <= maxRunHistory {
		return
	}
	for len(m.order) > maxRunHistory {
		id := m.order[0]
		if id == m.running {
			break
		}
		delete(m.runs, id)
		m.order = m.order[1:]
	}
}

func cloneRun(run Run) Run {
	out := run
	out.Results = append([]Result(nil), run.Results...)
	return out
}

func newRunID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "probe-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("probe-%d", time.Now().UnixNano())
}
