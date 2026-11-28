package network

import (
	"errors"
	"testing"
)

func TestValidateAddress(t *testing.T) {
	valid := []string{
		"192.168.1.1",
		"10.0.0.2",
		"2001:db8::1",
		"fe80::1",
		"router.local",
		"nas.home.example",
		"router",
		"Router.Local.",
	}
	for _, value := range valid {
		if err := ValidateAddress(value); err != nil {
			t.Errorf("合法地址 %q 被拒绝：%v", value, err)
		}
	}

	invalid := []string{
		"",
		" 192.168.1.1",
		"http://192.168.1.1",
		"https://router.local/",
		"192.168.1.1:80",
		"999.999.999.999",
		"192.168.1",
		"192.168.1.1/path",
		"user@router.local",
		"router.local?x=1",
		"坏地址",
		"router_local",
		"-router.local",
		"router.local-",
		"256.0.0.1",
		"12345",
	}
	for _, value := range invalid {
		if err := ValidateAddress(value); !errors.Is(err, ErrInvalid) {
			t.Errorf("非法地址 %q 应返回 ErrInvalid，got %v", value, err)
		}
	}
}

func TestDefaultTargetsSatisfyInvariant(t *testing.T) {
	targets := DefaultTargets()
	if len(targets) != 3 {
		t.Fatalf("默认应有 3 个目标，got %d", len(targets))
	}
	enabled := 0
	for _, target := range targets {
		if target.Enabled {
			enabled++
			if err := ValidateAddress(target.Address); err != nil {
				t.Fatalf("默认启用目标地址不合法：%v", err)
			}
		}
	}
	if enabled == 0 {
		t.Fatal("默认配置至少应启用一个目标")
	}
}

func TestNormalize(t *testing.T) {
	valid := Input{Targets: []TargetInput{
		{Key: TargetRouter, Address: "10.0.0.1", Enabled: false},
		{Key: TargetBypassRouter, Address: "10.0.0.2", Enabled: true},
		{Key: TargetNAS, Address: "nas.home", Enabled: false},
	}}
	targets, err := Normalize(valid)
	if err != nil {
		t.Fatalf("合法配置被拒绝：%v", err)
	}
	if len(targets) != 3 || targets[0].Key != TargetRouter || targets[1].Key != TargetBypassRouter || targets[2].Key != TargetNAS {
		t.Fatalf("目标顺序/数量不正确：%+v", targets)
	}
	if !targets[1].Enabled || targets[1].Address != "10.0.0.2" {
		t.Fatalf("旁路由配置不正确：%+v", targets[1])
	}

	allDisabled := Input{Targets: []TargetInput{
		{Key: TargetRouter, Address: "10.0.0.1", Enabled: false},
		{Key: TargetBypassRouter, Address: "", Enabled: false},
		{Key: TargetNAS, Address: "", Enabled: false},
	}}
	if _, err := Normalize(allDisabled); !errors.Is(err, ErrInvalid) {
		t.Fatalf("全部禁用应拒绝，got %v", err)
	}

	cases := []struct {
		name string
		in   Input
	}{
		{"缺少目标", Input{Targets: []TargetInput{{Key: TargetRouter, Address: "10.0.0.1", Enabled: true}}}},
		{"重复目标", Input{Targets: []TargetInput{
			{Key: TargetRouter, Address: "10.0.0.1", Enabled: true},
			{Key: TargetRouter, Address: "10.0.0.2", Enabled: true},
			{Key: TargetNAS, Address: "nas.home", Enabled: false},
		}}},
		{"未知目标", Input{Targets: []TargetInput{
			{Key: TargetRouter, Address: "10.0.0.1", Enabled: true},
			{Key: "switch", Address: "10.0.0.2", Enabled: false},
			{Key: TargetNAS, Address: "nas.home", Enabled: false},
		}}},
		{"启用目标地址非法", Input{Targets: []TargetInput{
			{Key: TargetRouter, Address: "http://10.0.0.1", Enabled: true},
			{Key: TargetBypassRouter, Address: "", Enabled: false},
			{Key: TargetNAS, Address: "", Enabled: false},
		}}},
		{"禁用目标非空地址非法", Input{Targets: []TargetInput{
			{Key: TargetRouter, Address: "10.0.0.1", Enabled: true},
			{Key: TargetBypassRouter, Address: "999.1.1.1", Enabled: false},
			{Key: TargetNAS, Address: "", Enabled: false},
		}}},
	}
	for _, tc := range cases {
		if _, err := Normalize(tc.in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s 应返回 ErrInvalid，got %v", tc.name, err)
		}
	}
}
