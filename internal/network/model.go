package network

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// TargetKey 是三个固定探测目标的稳定标识，不允许前端新增或删除目标。
type TargetKey string

const (
	TargetRouter       TargetKey = "router"
	TargetBypassRouter TargetKey = "bypass_router"
	TargetNAS          TargetKey = "nas"
)

// MaxAddressLen 限制目标地址长度；IPv6 与主机名都远低于该上限。
const MaxAddressLen = 253

// Target 是一个可独立启用与编辑的探测目标。
type Target struct {
	Key     TargetKey `json:"key"`
	Label   string    `json:"label"`
	Address string    `json:"address"`
	Enabled bool      `json:"enabled"`
}

// Config 是当前网络探测目标配置。
type Config struct {
	Targets   []Target `json:"targets"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// TargetInput 是保存请求中的一个目标；标签由服务端固定，不接受客户端覆盖。
type TargetInput struct {
	Key     TargetKey `json:"key"`
	Address string    `json:"address"`
	Enabled bool      `json:"enabled"`
}

// Input 是全量替换请求；必须同时提交三个固定目标。
type Input struct {
	Targets []TargetInput `json:"targets"`
}

// ErrInvalid 表示探测目标配置不合法。
var ErrInvalid = errors.New("网络配置不合法")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w：%s", ErrInvalid, fmt.Sprintf(format, args...))
}

var targetLabels = map[TargetKey]string{
	TargetRouter:       "主路由器",
	TargetBypassRouter: "旁路由",
	TargetNAS:          "NAS",
}

var targetOrder = []TargetKey{TargetRouter, TargetBypassRouter, TargetNAS}

// DefaultTargets 是首次迁移的占位配置：不写死用户实际网络地址。
// 主路由器先以占位主机名启用，另外两个目标留空并禁用，管理员首次进入后再改成实际地址。
func DefaultTargets() []Target {
	return []Target{
		{Key: TargetRouter, Label: targetLabels[TargetRouter], Address: "router.local", Enabled: true},
		{Key: TargetBypassRouter, Label: targetLabels[TargetBypassRouter], Address: "", Enabled: false},
		{Key: TargetNAS, Label: targetLabels[TargetNAS], Address: "", Enabled: false},
	}
}

// ValidateAddress 只接受 IPv4、IPv6 或主机名，不接受协议、端口、路径、查询参数。
func ValidateAddress(value string) error {
	if value == "" {
		return invalid("地址不能为空")
	}
	if len(value) > MaxAddressLen {
		return invalid("地址不能超过 %d 个字符", MaxAddressLen)
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return invalid("地址不能包含空白字符")
	}
	if strings.Contains(value, "://") {
		return invalid("请填写 IP 或主机名，不要包含 http:// 或 https://")
	}
	if strings.ContainsAny(value, "/?#@") {
		return invalid("请填写 IP 或主机名，不要包含路径、查询参数或用户信息")
	}
	if ip := net.ParseIP(value); ip != nil {
		return nil
	}
	if strings.Contains(value, ":") {
		return invalid("IPv6 地址不合法，或地址中包含了不允许的端口")
	}

	host := strings.TrimSuffix(value, ".")
	if host == "" {
		return invalid("主机名不能为空")
	}
	labels := strings.Split(host, ".")
	allNumeric := true
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return invalid("主机名标签不能为空且不能超过 63 个字符")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return invalid("主机名标签不能以连字符开头或结尾")
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				allNumeric = false
				continue
			}
			if (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return invalid("主机名只能包含字母、数字、连字符和点")
		}
	}
	if allNumeric {
		return invalid("IPv4 地址不合法，或地址不能是纯数字")
	}
	return nil
}

// Normalize 校验并生成服务端权威的目标列表。只允许三个固定 key，每个恰好一次。
func Normalize(in Input) ([]Target, error) {
	if len(in.Targets) != len(targetOrder) {
		return nil, invalid("必须提交主路由器、旁路由、NAS 三个目标")
	}
	byKey := make(map[TargetKey]TargetInput, len(in.Targets))
	for _, item := range in.Targets {
		if _, ok := targetLabels[item.Key]; !ok {
			return nil, invalid("未知的探测目标：%s", item.Key)
		}
		if _, dup := byKey[item.Key]; dup {
			return nil, invalid("探测目标重复：%s", item.Key)
		}
		address := strings.TrimSpace(item.Address)
		byKey[item.Key] = TargetInput{Key: item.Key, Address: address, Enabled: item.Enabled}
	}
	enabled := 0
	out := make([]Target, 0, len(targetOrder))
	for _, key := range targetOrder {
		item, ok := byKey[key]
		if !ok {
			return nil, invalid("缺少探测目标：%s", key)
		}
		if item.Enabled {
			enabled++
			if err := ValidateAddress(item.Address); err != nil {
				return nil, fmt.Errorf("%w（%s）", err, targetLabels[key])
			}
		} else if item.Address != "" {
			if err := ValidateAddress(item.Address); err != nil {
				return nil, fmt.Errorf("%w（%s）", err, targetLabels[key])
			}
		}
		out = append(out, Target{
			Key:     key,
			Label:   targetLabels[key],
			Address: item.Address,
			Enabled: item.Enabled,
		})
	}
	if enabled == 0 {
		return nil, invalid("至少需要启用一个探测目标")
	}
	return out, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
