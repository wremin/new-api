package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func ch(id int, groups string) *model.Channel {
	return &model.Channel{Id: id, Group: groups}
}

// TestSelectAssetsChannelSingle 单渠道部署：无论有没有分组都必须原样返回。
//
// 这是存量部署的绝大多数形态，改动**不能**影响它们 ——
// 哪怕渠道压根没配分组，也要照常工作。
func TestSelectAssetsChannelSingle(t *testing.T) {
	only := ch(1, "")
	for _, group := range []string{"", "default", "vip"} {
		got, aErr := selectAssetsChannel([]*model.Channel{only}, group)
		if aErr != nil {
			t.Fatalf("group=%q 意外报错: %v", group, aErr.Message)
		}
		if got != only {
			t.Errorf("group=%q 选错了渠道", group)
		}
	}
}

// TestSelectAssetsChannelByGroup 多渠道时按分组区分 —— 本次改动的主目标。
func TestSelectAssetsChannelByGroup(t *testing.T) {
	runyuan := ch(1, "default,vip")
	yike := ch(2, "yike-group")
	all := []*model.Channel{runyuan, yike}

	cases := map[string]*model.Channel{
		"default":    runyuan,
		"vip":        runyuan,
		"yike-group": yike,
	}
	for group, want := range cases {
		got, aErr := selectAssetsChannel(all, group)
		if aErr != nil {
			t.Fatalf("group=%q 意外报错: %v", group, aErr.Message)
		}
		if got != want {
			t.Errorf("group=%q 选中渠道 %d，期望 %d", group, got.Id, want.Id)
		}
	}
}

// TestSelectAssetsChannelGroupNotBound 分组谁都没绑时回落到旧行为。
//
// 关键：**不能**因为分组匹配不上就报一个新的错 ——
// 那会让没配分组的老部署在升级后突然不能传素材。
func TestSelectAssetsChannelGroupNotBound(t *testing.T) {
	// 两个渠道都没配分组 → 回落到旧逻辑 → 仍然是 ambiguous（与改动前一致）
	both := []*model.Channel{ch(1, ""), ch(2, "")}
	_, aErr := selectAssetsChannel(both, "whatever")
	if aErr == nil || aErr.Code != AssetErrChannelAmbiguous {
		t.Errorf("多渠道且分组匹配不上时应当仍报 ambiguous，实际 %v", aErr)
	}

	// 只有一个渠道、分组也对不上 → 照常返回它，不能报错
	one := ch(1, "other-group")
	got, aErr := selectAssetsChannel([]*model.Channel{one}, "no-match")
	if aErr != nil || got != one {
		t.Errorf("单渠道时分组对不上也应返回该渠道，实际 got=%v err=%v", got, aErr)
	}
}

// TestSelectAssetsChannelGroupStillAmbiguous 同一分组绑了多个渠道，仍然是歧义。
// 但报错必须带上分组名，否则管理员不知道该收窄哪个。
func TestSelectAssetsChannelGroupStillAmbiguous(t *testing.T) {
	_, aErr := selectAssetsChannel([]*model.Channel{ch(1, "same"), ch(2, "same")}, "same")
	if aErr == nil || aErr.Code != AssetErrChannelAmbiguous {
		t.Fatalf("同分组多渠道应报 ambiguous，实际 %v", aErr)
	}
	if aErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d", aErr.StatusCode)
	}
	if !contains(aErr.Message, "same") {
		t.Errorf("报错里没带分组名，管理员无从下手：%s", aErr.Message)
	}
}

// TestSelectAssetsChannelNone 没有候选时报"未配置"，不是"有歧义"。
func TestSelectAssetsChannelNone(t *testing.T) {
	_, aErr := selectAssetsChannel(nil, "any")
	if aErr == nil || aErr.Code != AssetErrChannelNotConfigured {
		t.Errorf("无候选应报 not_configured，实际 %v", aErr)
	}
}

// TestSelectAssetsChannelEmptyGroup 分组为空（取不到时）走旧逻辑，绝不瞎猜。
func TestSelectAssetsChannelEmptyGroup(t *testing.T) {
	_, aErr := selectAssetsChannel([]*model.Channel{ch(1, "a"), ch(2, "b")}, "")
	if aErr == nil || aErr.Code != AssetErrChannelAmbiguous {
		t.Errorf("空分组 + 多渠道应报 ambiguous（而不是随便挑一个），实际 %v", aErr)
	}
}

// TestSelectAssetsChannelGroupTrimmed 渠道分组字段带空格也要能匹配。
// GetGroups() 会 TrimSpace，这里确认这个行为被依赖到了。
func TestSelectAssetsChannelGroupTrimmed(t *testing.T) {
	spaced := ch(2, "default, yike-group ,other")
	got, aErr := selectAssetsChannel([]*model.Channel{ch(1, "solo"), spaced}, "yike-group")
	if aErr != nil || got != spaced {
		t.Errorf("带空格的分组应当能匹配，实际 got=%v err=%v", got, aErr)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestSelectAssetsChannelAutoGroup "auto" 是占位符，不能当真实分组名去筛。
//
// 令牌分组为 auto 时，middleware/auth.go 会把字面量 "auto" 写进上下文。
// 若当成真实分组，报错会变成"没有渠道绑定到 auto 分组"，
// 管理员照做就会去加一个名叫 auto 的分组 —— 把人带偏。
func TestSelectAssetsChannelAutoGroup(t *testing.T) {
	// 多渠道 + auto → 应当报旧的 ambiguous，且**不提** auto 分组
	_, aErr := selectAssetsChannel([]*model.Channel{ch(1, "a"), ch(2, "b")}, "auto")
	if aErr == nil || aErr.Code != AssetErrChannelAmbiguous {
		t.Fatalf("auto + 多渠道应报 ambiguous，实际 %v", aErr)
	}
	if contains(aErr.Message, "auto") {
		t.Errorf("报错不应把 auto 当成分组名提出来：%s", aErr.Message)
	}

	// 单渠道 + auto → 正常返回，不受影响
	only := ch(1, "a")
	got, aErr := selectAssetsChannel([]*model.Channel{only}, "auto")
	if aErr != nil || got != only {
		t.Errorf("auto + 单渠道应正常返回，实际 got=%v err=%v", got, aErr)
	}

	// 确认没有渠道会因为叫 auto 就被选中（避免有人真去加这个分组时行为诡异）
	weird := ch(2, "auto")
	got, aErr = selectAssetsChannel([]*model.Channel{ch(1, "a"), weird}, "auto")
	if aErr == nil {
		t.Errorf("即使有渠道绑了 auto，也不该被 auto 选中（应仍是 ambiguous），实际选中 %v", got)
	}
}
