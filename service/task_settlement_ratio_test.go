package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func bc(ratios map[string]float64) *model.TaskBillingContext {
	return &model.TaskBillingContext{OtherRatios: ratios}
}

// TestTokenSettlementMultiplierSkipsSeconds 本文件的核心断言。
//
// 生产实测：一个 15 秒任务被收 ¥44.84，按上游官价只应收 ¥14.95 —— 正好 3 倍。
// 原因是 token 数已经随时长增长（5秒≈108,900，15秒≈324,900），
// 而 seconds 倍率又是 ceil(15/5)=3，两者相乘把时长算了两遍。
func TestTokenSettlementMultiplierSkipsSeconds(t *testing.T) {
	if got := tokenSettlementMultiplier(bc(map[string]float64{"seconds": 3})); got != 1.0 {
		t.Errorf("seconds 必须被跳过（token 数已含时长），实际倍率 %g", got)
	}
	if got := tokenSettlementMultiplier(bc(map[string]float64{"seconds": 6})); got != 1.0 {
		t.Errorf("30 秒任务同样要跳过，实际倍率 %g", got)
	}
}

// TestTokenSettlementMultiplierKeepsVideoInput video_input 必须保留。
//
// 它是上游给的折扣，不是内容量纲，token 数里不体现 —— 跳过它会导致少收。
// 这条是经确认后刻意保留的行为，不是遗漏。
func TestTokenSettlementMultiplierKeepsVideoInput(t *testing.T) {
	got := tokenSettlementMultiplier(bc(map[string]float64{"video_input": 0.6}))
	if math.Abs(got-0.6) > 1e-9 {
		t.Errorf("video_input 应当保留，实际倍率 %g", got)
	}

	// 与 seconds 同时存在时，只跳过 seconds
	got = tokenSettlementMultiplier(bc(map[string]float64{"seconds": 3, "video_input": 0.6}))
	if math.Abs(got-0.6) > 1e-9 {
		t.Errorf("应当只跳过 seconds、保留 video_input，实际倍率 %g", got)
	}
}

// TestTokenSettlementMultiplierSkipsSize size 同样要跳过。
//
// 由实测反解出的公式 tokens = 宽×高×24fps×秒数/1024 + 900 里含 宽×高，
// 说明 token 数已经随分辨率缩放，再乘 size 就是重复计算。
//
// 另一个细节：1080P 对 720P 的真实 token 比是 2.25（像素比），
// 而 size 倍率配的是 1.5 —— 两个数本来就对不上。
// 去掉之后计费直接与 token 成正比，也就与上游成本成正比。
func TestTokenSettlementMultiplierSkipsSize(t *testing.T) {
	if got := tokenSettlementMultiplier(bc(map[string]float64{"size": 1.5})); got != 1.0 {
		t.Errorf("size 必须被跳过（token 数已含分辨率），实际倍率 %g", got)
	}
	// seconds + size 同时存在时两个都跳过，只剩 1.0
	if got := tokenSettlementMultiplier(bc(map[string]float64{"seconds": 3, "size": 1.5})); got != 1.0 {
		t.Errorf("seconds 与 size 都应跳过，实际倍率 %g", got)
	}
}

// TestTokenFormulaMatchesMeasurements 把反解出的公式本身钉住。
//
// 这不是在测生产代码，而是在记录**排除表的依据**：
// 将来有人想把 seconds 或 size 加回去时，先得解释这两个实测点。
func TestTokenFormulaMatchesMeasurements(t *testing.T) {
	tokens := func(w, h, seconds int) int {
		return w*h*24*seconds/1024 + 900
	}
	if got := tokens(1280, 720, 5); got != 108900 {
		t.Errorf("5 秒 720P 应为 108900（实测值），公式算出 %d", got)
	}
	if got := tokens(1280, 720, 15); got != 324900 {
		t.Errorf("15 秒 720P 应为 324900（实测值），公式算出 %d", got)
	}
	// 尚未实测，但公式给出的预测 —— 跑一个 1080P 5 秒任务即可证伪
	if got := tokens(1920, 1080, 5); got != 243900 {
		t.Errorf("1080P 5 秒的预测值应为 243900，公式算出 %d", got)
	}
}

// TestTokenSettlementMultiplierEdges 边界：nil、空、非法值。
func TestTokenSettlementMultiplierEdges(t *testing.T) {
	if got := tokenSettlementMultiplier(nil); got != 1.0 {
		t.Errorf("nil 应返回 1.0，实际 %g", got)
	}
	if got := tokenSettlementMultiplier(bc(nil)); got != 1.0 {
		t.Errorf("空 ratios 应返回 1.0，实际 %g", got)
	}
	// 0 和负数是脏数据，不能让它把金额清零或变负
	if got := tokenSettlementMultiplier(bc(map[string]float64{"x": 0})); got != 1.0 {
		t.Errorf("倍率 0 应被忽略，实际 %g", got)
	}
	if got := tokenSettlementMultiplier(bc(map[string]float64{"x": -2})); got != 1.0 {
		t.Errorf("负倍率应被忽略，实际 %g", got)
	}
}

// TestTokenSettlementMatchesUpstreamListPrice 端到端核对：
// 去掉时长重复计算后，结算金额应当正好等于上游官价。
//
// 数字全部来自 2026-08-28 的真实账单：
//
//	上游官价  324,900 × ¥46/1M = ¥14.9454
//	我们原本收 ¥44.836191（= 官价 × 3）
func TestTokenSettlementMatchesUpstreamListPrice(t *testing.T) {
	const (
		tokens     = 324900
		modelRatio = 46.0 / (2.0 * 7.3) // doubao-seedance-2.0，¥46 / 1M tokens
		usd2rmb    = 7.3
		quotaUnit  = 500000.0
	)

	multiplier := tokenSettlementMultiplier(bc(map[string]float64{"seconds": 3, "size": 1.0}))
	quota := int(float64(tokens) * modelRatio * 1.0 * multiplier)
	cny := float64(quota) / quotaUnit * usd2rmb

	const upstreamListPrice = 14.9454
	if math.Abs(cny-upstreamListPrice) > 0.01 {
		t.Errorf("结算 ¥%.4f，应当贴近上游官价 ¥%.4f", cny, upstreamListPrice)
	}

	// 顺带确认修复前确实是 3 倍
	oldMultiplier := 3.0 * 1.0
	oldCNY := float64(int(float64(tokens)*modelRatio*1.0*oldMultiplier)) / quotaUnit * usd2rmb
	if math.Abs(oldCNY-44.836) > 0.01 {
		t.Errorf("修复前应为 ¥44.836，实际算出 ¥%.4f —— 说明这条测试的假设与生产对不上", oldCNY)
	}
}
