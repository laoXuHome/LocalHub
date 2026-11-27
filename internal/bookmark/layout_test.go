package bookmark

import (
	"context"
	"testing"
	"time"
)

func TestLayoutReplaceAndRead(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()

	mk := func(name string) Bookmark {
		t.Helper()
		b, err := Build(nil, Input{Name: strp(name), WANURL: strp("https://" + name + ".example.com")})
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.Create(ctx, b, now)
		if err != nil {
			t.Fatal(err)
		}
		return *out
	}
	a, b, c := mk("a"), mk("b"), mk("c")

	layout := Layout{
		Desktop: []LayoutItem{{ID: c.ID, Size: LayoutLarge}, {ID: a.ID, Size: LayoutSmall}, {ID: b.ID, Size: LayoutMedium}},
		Mobile:  []LayoutItem{{ID: b.ID, Size: LayoutMedium}, {ID: c.ID, Size: LayoutSmall}},
	}
	if err := s.ReplaceLayouts(ctx, layout, now); err != nil {
		t.Fatalf("保存布局失败：%v", err)
	}
	got, err := s.GetLayout(ctx)
	if err != nil {
		t.Fatalf("读取布局失败：%v", err)
	}
	if len(got.Desktop) != 3 || got.Desktop[0].ID != c.ID || got.Desktop[0].Size != LayoutLarge || got.Desktop[0].Position != 0 {
		t.Fatalf("桌面布局不正确：%+v", got.Desktop)
	}
	if len(got.Mobile) != 2 || got.Mobile[0].ID != b.ID || got.Mobile[1].ID != c.ID {
		t.Fatalf("移动布局不正确：%+v", got.Mobile)
	}

	// 设备独立：重新保存桌面不影响移动端。
	if err := s.ReplaceLayouts(ctx, Layout{
		Desktop: []LayoutItem{{ID: a.ID, Size: LayoutMedium}},
		Mobile:  got.Mobile,
	}, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetLayout(ctx)
	if len(got.Desktop) != 1 || got.Desktop[0].ID != a.ID || len(got.Mobile) != 2 {
		t.Fatalf("设备布局未独立保存：%+v", got)
	}

	// 既有 P2 sort_order 不被布局写入改动。
	list, err := s.List(ctx, ListOptions{Admin: true, IncludeDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range list {
		if item.SortOrder != i {
			t.Fatalf("布局保存不应修改 sort_order：%+v", list)
		}
	}
}

func TestLayoutValidation(t *testing.T) {
	for _, layout := range []Layout{
		{Desktop: []LayoutItem{{ID: 1, Size: "huge"}}, Mobile: []LayoutItem{}},
		{Desktop: []LayoutItem{{ID: 1, Size: LayoutLarge}, {ID: 1, Size: LayoutSmall}}, Mobile: []LayoutItem{}},
		{Desktop: []LayoutItem{{ID: 0, Size: LayoutLarge}}, Mobile: []LayoutItem{}},
	} {
		if err := ValidateLayout(layout); err == nil {
			t.Fatalf("非法布局应被拒绝：%+v", layout)
		}
	}
	s := testStore(t)
	ctx := context.Background()
	if err := s.ReplaceLayouts(ctx, Layout{Desktop: []LayoutItem{{ID: 999, Size: LayoutLarge}}, Mobile: []LayoutItem{}}, time.Now()); err == nil {
		t.Fatalf("不存在的书签 ID 应被拒绝")
	}
}

func TestLayoutDeleteForBookmark(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	b, _ := Build(nil, Input{Name: strp("deleted"), WANURL: strp("https://d.example.com")})
	created, err := s.Create(ctx, b, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceLayouts(ctx, Layout{Desktop: []LayoutItem{{ID: created.ID, Size: LayoutSmall}}, Mobile: []LayoutItem{}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteForBookmark(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetLayout(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Desktop) != 0 {
		t.Fatalf("删除书签后布局应清理：%+v", got)
	}
}
