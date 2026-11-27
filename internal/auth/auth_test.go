package auth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		pw   string
		want bool
	}{
		{"short", false},
		{"12345678901", false}, // 11 字节
		{"123456789012", true}, // 12 字节
		{"   ", false},
		{strings.Repeat("a", 73), false},
		{strings.Repeat("a", 72), true},
	}
	for _, tc := range cases {
		err := ValidatePassword(tc.pw)
		if (err == nil) != tc.want {
			t.Fatalf("ValidatePassword(%d 字节) 结果不符：err=%v want=%v", len([]byte(tc.pw)), err, tc.want)
		}
	}
}

func TestHashAndCompare(t *testing.T) {
	plain := "Adm1nPassw0rd!2026"
	h1, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	h2, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	if h1 == h2 {
		t.Fatalf("相同口令应产生不同哈希（随机盐）")
	}
	if strings.Contains(h1, plain) {
		t.Fatalf("哈希中出现明文")
	}
	if !ComparePassword(h1, plain) || !ComparePassword(h2, plain) {
		t.Fatalf("正确口令应校验通过")
	}
	if ComparePassword(h1, "wrong-password-000") {
		t.Fatalf("错误口令不应通过")
	}
	if ComparePassword("not-a-hash", plain) {
		t.Fatalf("非法哈希不应通过，且不应 panic")
	}
}

func TestTokenHashIsStableAndUnique(t *testing.T) {
	a1, h1, err := HashToken()
	if err != nil {
		t.Fatalf("生成令牌失败：%v", err)
	}
	a2, h2, _ := HashToken()
	if a1 == a2 || h1 == h2 {
		t.Fatalf("令牌应唯一")
	}
	if HashTokenString(a1) != h1 {
		t.Fatalf("令牌哈希计算不一致")
	}
	if strings.Contains(h1, a1) {
		t.Fatalf("库中不应保存令牌明文")
	}
}

func TestSingleAdminConstraint(t *testing.T) {
	d := testDB(t)
	st := &Store{DB: d}
	ctx := context.Background()
	hash, _ := HashPassword("Adm1nPassw0rd!2026")
	if err := st.CreateAdmin(ctx, "admin", hash, time.Now()); err != nil {
		t.Fatalf("首次创建管理员失败：%v", err)
	}
	if err := st.CreateAdmin(ctx, "admin2", hash, time.Now()); err == nil {
		t.Fatalf("第二个管理员必须被拒绝（单管理员约束）")
	}
	if n, err := st.CountAdmins(ctx); err != nil || n != 1 {
		t.Fatalf("管理员数量应为 1，got %d err=%v", n, err)
	}
	if _, err := st.GetByUsername(ctx, "nobody"); err != ErrNoAdmin {
		t.Fatalf("不存在的管理员应返回 ErrNoAdmin，got %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	d := testDB(t)
	st := &Store{DB: d}
	ctx := context.Background()
	hash, _ := HashPassword("Adm1nPassw0rd!2026")
	if err := st.CreateAdmin(ctx, "admin", hash, time.Now()); err != nil {
		t.Fatalf("创建管理员失败：%v", err)
	}
	admin, err := st.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("读取管理员失败：%v", err)
	}
	m := &Manager{DB: d, TTL: time.Hour}
	now := time.Now()

	token, sess, err := m.Create(ctx, admin.ID, "127.0.0.1", "test-agent", now)
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	got, err := m.Get(ctx, token, now)
	if err != nil {
		t.Fatalf("有效会话应校验通过：%v", err)
	}
	if got.Username != "admin" || got.AdminID != admin.ID {
		t.Fatalf("会话信息不符：%+v", got)
	}
	if !got.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Fatalf("过期时间不一致")
	}

	// 伪造令牌无效
	if _, err := m.Get(ctx, token+"x", now); err != ErrSessionInvalid {
		t.Fatalf("伪造令牌应无效，got %v", err)
	}
	// 过期后无效
	if _, err := m.Get(ctx, token, now.Add(2*time.Hour)); err != ErrSessionInvalid {
		t.Fatalf("过期会话应无效，got %v", err)
	}
	// 吊销后无效
	if err := m.Revoke(ctx, token, now); err != nil {
		t.Fatalf("吊销失败：%v", err)
	}
	if _, err := m.Get(ctx, token, now); err != ErrSessionInvalid {
		t.Fatalf("吊销后会话应无效，got %v", err)
	}
}

func TestRevokeOthersKeepsCurrent(t *testing.T) {
	d := testDB(t)
	st := &Store{DB: d}
	ctx := context.Background()
	hash, _ := HashPassword("Adm1nPassw0rd!2026")
	_ = st.CreateAdmin(ctx, "admin", hash, time.Now())
	admin, _ := st.GetByUsername(ctx, "admin")

	m := &Manager{DB: d, TTL: time.Hour}
	now := time.Now()
	keep, _, err := m.Create(ctx, admin.ID, "a", "ua", now)
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	other1, _, _ := m.Create(ctx, admin.ID, "b", "ua", now)
	other2, _, _ := m.Create(ctx, admin.ID, "c", "ua", now)

	n, err := m.RevokeOthers(ctx, admin.ID, keep, now)
	if err != nil || n != 2 {
		t.Fatalf("应吊销 2 个其他会话，got %d err=%v", n, err)
	}
	if _, err := m.Get(ctx, keep, now); err != nil {
		t.Fatalf("当前会话应保留：%v", err)
	}
	for _, tok := range []string{other1, other2} {
		if _, err := m.Get(ctx, tok, now); err == nil {
			t.Fatalf("其他会话应被吊销")
		}
	}
}

func TestLimiterLockAndRecover(t *testing.T) {
	d := testDB(t)
	l := &Limiter{DB: d, MaxFailures: 3, Window: time.Minute, Lockout: 2 * time.Minute}
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 3; i++ {
		if err := l.Record(ctx, "1.2.3.4", "admin", false, now); err != nil {
			t.Fatalf("记录失败：%v", err)
		}
	}
	locked, retry, err := l.Locked(ctx, "1.2.3.4", "admin", now)
	if err != nil || !locked || retry <= 0 {
		t.Fatalf("达到阈值应锁定，got locked=%v retry=%v err=%v", locked, retry, err)
	}
	// 其他 IP 不受影响
	if locked, _, _ := l.Locked(ctx, "5.6.7.8", "admin", now); locked {
		t.Fatalf("其他 IP 不应被锁定")
	}
	// 锁定期结束自动解除
	after := now.Add(3 * time.Minute)
	if locked, _, err := l.Locked(ctx, "1.2.3.4", "admin", after); err != nil || locked {
		t.Fatalf("锁定期结束后应解除，got locked=%v err=%v", locked, err)
	}
	// 成功后清除失败记录
	if err := l.Record(ctx, "1.2.3.4", "admin", false, after); err != nil {
		t.Fatal(err)
	}
	if err := l.Clear(ctx, "1.2.3.4", "admin"); err != nil {
		t.Fatal(err)
	}
	if locked, _, _ := l.Locked(ctx, "1.2.3.4", "admin", after); locked {
		t.Fatalf("清除后不应锁定")
	}
}
