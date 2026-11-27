package bookmark

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/laoXuHome/LocalHub/internal/db"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "bm.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return &Store{DB: d}
}

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

func TestValidationRules(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		ok   bool
	}{
		{"合法最小", Input{Name: strp("NAS"), WANURL: strp("https://nas.example.com")}, true},
		{"名称为空", Input{Name: strp("  "), WANURL: strp("https://a.example.com")}, false},
		{"无任何 URL", Input{Name: strp("x")}, false},
		{"javascript 协议被拒", Input{Name: strp("x"), WANURL: strp("javascript:alert(1)")}, false},
		{"无主机", Input{Name: strp("x"), LANURL: strp("http://")}, false},
		{"名称超长", Input{Name: strp(strings.Repeat("名", 70)), WANURL: strp("https://a.example.com")}, false},
		{"描述超长", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), Description: strp(strings.Repeat("d", 201))}, false},
		{"visibility 非法", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), Visibility: strp("everyone")}, false},
		{"visibility=disabled 不可写入", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), Visibility: strp("disabled")}, false},
		{"open_mode 非法", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), OpenMode: strp("popup")}, false},
		{"icon url 非法", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), IconType: strp("url"), IconValue: strp("ftp://x/y.png")}, false},
		{"icon none 带值", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), IconType: strp("none"), IconValue: strp("x")}, false},
		{"排序负数", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), SortOrder: intp(-1)}, false},
		{"docker 超长", Input{Name: strp("x"), WANURL: strp("https://a.example.com"), DockerContainer: strp(strings.Repeat("c", 129))}, false},
		{"默认值补齐", Input{Name: strp("x"), WANURL: strp("https://a.example.com")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Build(nil, tc.in)
			if tc.ok && err != nil {
				t.Fatalf("应通过，得到错误：%v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("应被拒绝，但通过了：%+v", b)
			}
			if tc.ok {
				if b.IconType != IconNone || b.Visibility != VisibilityPublic || b.OpenMode != OpenModeNewTab || !b.Enabled {
					t.Fatalf("默认值不正确：%+v", b)
				}
			}
		})
	}
}

func intp(i int) *int { return &i }

func TestEffectiveVisibilityAndAccess(t *testing.T) {
	cases := []struct {
		enabled   bool
		vis       string
		guest     bool
		admin     bool
		effective string
	}{
		{true, VisibilityPublic, true, true, VisibilityPublic},
		{true, VisibilityAuthenticated, false, true, VisibilityAuthenticated},
		{true, VisibilityPrivate, false, true, VisibilityPrivate},
		{false, VisibilityPublic, false, false, VisibilityDisabled},
		{false, VisibilityPrivate, false, false, VisibilityDisabled},
	}
	for _, c := range cases {
		b := Bookmark{Enabled: c.enabled, Visibility: c.vis}
		if b.VisibleTo(false) != c.guest || b.VisibleTo(true) != c.admin {
			t.Fatalf("%+v 可见性不符", c)
		}
		if b.EffectiveVisibility() != c.effective {
			t.Fatalf("%+v 有效权限不符：%s", c, b.EffectiveVisibility())
		}
	}
}

func TestStoreCRUDAndOrder(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now()

	b1, err := st.Create(ctx, mustBuild(t, Input{Name: strp("A"), WANURL: strp("https://a.example.com")}), now)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := st.Create(ctx, mustBuild(t, Input{Name: strp("B"), WANURL: strp("https://b.example.com")}), now)
	b3, _ := st.Create(ctx, mustBuild(t, Input{Name: strp("C"), WANURL: strp("https://c.example.com")}), now)
	if b1.SortOrder != 0 || b2.SortOrder != 1 || b3.SortOrder != 2 {
		t.Fatalf("新增应追加到末尾：%d/%d/%d", b1.SortOrder, b2.SortOrder, b3.SortOrder)
	}

	list, err := st.List(ctx, ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil || len(list) != 3 {
		t.Fatalf("列表应为 3 条：%d err=%v", len(list), err)
	}
	if _, err := st.Get(ctx, 999); err != ErrNotFound {
		t.Fatalf("不存在应 ErrNotFound，got %v", err)
	}

	// 重排：C, A, B
	if err := st.Reorder(ctx, []int64{b3.ID, b1.ID, b2.ID}, now); err != nil {
		t.Fatal(err)
	}
	list, _ = st.List(ctx, ListOptions{Admin: true, IncludeDisabled: true})
	if list[0].ID != b3.ID || list[1].ID != b1.ID || list[2].ID != b2.ID {
		t.Fatalf("排序不符合预期：%+v", list)
	}
	// 重排含未知 ID
	if err := st.Reorder(ctx, []int64{b1.ID, 999}, now); err == nil {
		t.Fatalf("未知 ID 应报错")
	}
	// 重排重复 ID
	if err := st.Reorder(ctx, []int64{b1.ID, b1.ID}, now); err == nil {
		t.Fatalf("重复 ID 应报错")
	}

	if err := st.Delete(ctx, b1.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(ctx, b1.ID); err != ErrNotFound {
		t.Fatalf("重复删除应 ErrNotFound，got %v", err)
	}

	n, missing, err := st.BatchDelete(ctx, []int64{b2.ID, 999, b2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(missing) != 1 || missing[0] != 999 {
		t.Fatalf("批量删除结果不符：n=%d missing=%v", n, missing)
	}
}

func TestUpdateChangesOnlyProvidedFields(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now()
	orig := mustBuild(t, Input{
		Name:       strp("Orig"),
		LANURL:     strp("http://192.168.1.10:8080"),
		WANURL:     strp("https://orig.example.com"),
		Visibility: strp(VisibilityPrivate),
	})
	created, err := st.Create(ctx, orig, now)
	if err != nil {
		t.Fatal(err)
	}
	next, err := Build(created, Input{Name: strp("Renamed")})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(ctx, next, now); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Get(ctx, created.ID)
	if got.Name != "Renamed" || got.LANURL != orig.LANURL || got.WANURL != orig.WANURL || got.Visibility != VisibilityPrivate {
		t.Fatalf("编辑应只改提供字段：%+v", got)
	}
}

func mustBuild(t *testing.T, in Input) *Bookmark {
	t.Helper()
	b, err := Build(nil, in)
	if err != nil {
		t.Fatalf("构造书签失败：%v", err)
	}
	return b
}

func TestMigrationIsRepeatableAndKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repeat.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := &Store{DB: d}
	now := time.Now()
	b, err := st.Create(ctx, mustBuild(t, Input{Name: strp("保留"), WANURL: strp("https://keep.example.com")}), now)
	if err != nil {
		t.Fatal(err)
	}
	// 再次执行迁移必须无错且不破坏已有数据。
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("重复迁移失败：%v", err)
	}
	got, err := st.Get(ctx, b.ID)
	if err != nil || got.Name != "保留" {
		t.Fatalf("迁移后数据丢失：%+v err=%v", got, err)
	}
	// 关闭后重新打开，数据仍在（持久化验证）。
	_ = d.Close()
	d2, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if err := d2.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	got2, err := (&Store{DB: d2}).Get(ctx, b.ID)
	if err != nil || got2.WANURL != "https://keep.example.com" {
		t.Fatalf("重开数据库后数据丢失：%+v err=%v", got2, err)
	}
}
