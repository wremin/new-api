package yike

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// AdjustBillingOnComplete 用上游 GetYikeJobCredit 返回的实际消耗做差额结算。
//
// 为什么值得多打一次上游请求：预扣是按「时长 × 分辨率」估的，而上游按真实
// 消耗计费。任务被上游截断、参数被上游修正、或者失败后只收部分费用，
// 估算与实际就会分叉 —— 差额结算是唯一能把这部分抹平的地方。
//
// 三道闸门，任何一道不满足都返回 0（保持预扣），绝不猜：
//  1. 渠道必须配置 yike_credit_usd_rate。credit 的计价单位上游资料里没有给出，
//     也没有响应样例可推断。没有这个换算率就按实际额度结算，等于拿客户的余额
//     赌一个未经验证的假设 —— 宁可保持预扣，让管理员在联调拿到真实响应后再配。
//  2. 上游查询必须成功且能解出正数 credit。
//  3. 结算额度必须为正。
//
// 注意签名只给了 (task, taskResult)，渠道信息要自己查。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || taskResult == nil {
		return 0
	}
	// 失败任务走的是退款路径，不在这里结算
	if taskResult.Status != model.TaskStatusSuccess {
		return 0
	}

	ctx := context.Background()

	ch, err := model.CacheGetChannel(task.ChannelId)
	if err != nil || ch == nil {
		logger.LogWarn(ctx, fmt.Sprintf("yike: 任务 %s 取渠道 %d 失败，保持预扣额度: %v",
			task.TaskID, task.ChannelId, err))
		return 0
	}

	rate := ch.GetOtherSettings().YikeCreditUsdRate
	if rate <= 0 {
		// 未配置换算率是**预期内**的默认状态，不是错误，因此只记 debug 级别，
		// 否则每个任务完成都会刷一条告警。
		return 0
	}

	credit, ok := fetchJobCredit(ctx, ch, task.GetUpstreamTaskID())
	if !ok || credit <= 0 {
		return 0
	}

	// credit → 美元 → 系统额度。QuotaPerUnit 是 1 美元对应的额度数。
	quota := int(credit * rate * common.QuotaPerUnit)
	if quota <= 0 {
		return 0
	}
	logger.LogInfo(ctx, fmt.Sprintf("yike: 任务 %s 上游实际消耗 %g credit，按 %g USD/credit 结算为 %d 额度",
		task.TaskID, credit, rate, quota))
	return quota
}

// fetchJobCredit 查询单个任务的上游实际计费。
//
// 返回 ok=false 表示"拿不到"，调用方应保持预扣而不是判定为 0 消耗 ——
// 把查询失败当成免费会让上游的每一次抖动都变成一次全额退款。
func fetchJobCredit(ctx context.Context, ch *model.Channel, jobID string) (float64, bool) {
	if jobID == "" {
		return 0, false
	}
	resp, err := doAction(ch.GetBaseURL(), ch.Key, ch.GetSetting().Proxy,
		ActionGetJobCredit, map[string]any{"JobId": jobID})
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("yike: 查询任务计费失败 %s: %v", jobID, err))
		return 0, false
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("yike: 读取计费响应失败 %s: %v", jobID, err))
		return 0, false
	}
	if resp.StatusCode != http.StatusOK {
		logger.LogWarn(ctx, fmt.Sprintf("yike: 计费查询返回 %d: %s", resp.StatusCode, ExtractUpstreamError(body)))
		return 0, false
	}
	return parseJobCredit(body)
}

// parseJobCredit 从 GetYikeJobCredit 响应里挖出消耗值。
//
// 上游资料只说明了这个 Action 的用途，没有给响应样例，字段名与嵌套层级都待联调核对。
// 因此这里不写死路径，而是在响应里按一组候选键名递归找第一个数值 ——
// 上游把值放在顶层还是包在 Credit / Data 里都能接住。
// 一个都找不到就返回 ok=false，由调用方保持预扣。
func parseJobCredit(body []byte) (float64, bool) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return 0, false
	}
	return searchCredit(raw, 0)
}

// creditKeys 是候选字段名，按可能性排序。全部小写比较。
var creditKeys = []string{
	"creditcost", "costcredit", "consumedcredit", "creditconsumed",
	"usedcredit", "creditused", "cost", "credit", "amount", "credits",
}

func searchCredit(node any, depth int) (float64, bool) {
	if depth > 4 {
		return 0, false
	}
	obj, ok := node.(map[string]any)
	if !ok {
		return 0, false
	}
	// 先在当前层按候选键名找，再下潜 —— 避免深层的同名字段抢在顶层之前被取到
	for _, want := range creditKeys {
		for k, v := range obj {
			if !equalFold(k, want) {
				continue
			}
			if f, ok := toFloat(v); ok && f > 0 {
				return f, true
			}
		}
	}
	for _, v := range obj {
		if f, ok := searchCredit(v, depth+1); ok {
			return f, true
		}
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// equalFold 只做 ASCII 大小写不敏感比较，want 必须已是小写。
func equalFold(got, wantLower string) bool {
	if len(got) != len(wantLower) {
		return false
	}
	for i := 0; i < len(got); i++ {
		c := got[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != wantLower[i] {
			return false
		}
	}
	return true
}
