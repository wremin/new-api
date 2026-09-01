package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// TestTaskUsageFromResult 日志用量的换算。
//
// 这些数字**不参与计费**，只决定日志页「输入 / 输出」两列显示什么。
// 之前这两列对所有异步任务恒为空：金额按 token 算对了，用量却没传进日志，
// 对账时只能回上游账单去比。
func TestTaskUsageFromResult(t *testing.T) {
	cases := []struct {
		name                       string
		completion, total          int
		wantPrompt, wantCompletion int
		wantTotal                  int
	}{
		{
			// seedance 的实际形态：completion == total，输入应为 0
			name: "总数等于输出", completion: 324900, total: 324900,
			wantPrompt: 0, wantCompletion: 324900, wantTotal: 324900,
		},
		{
			// 上游区分输入输出时，差额落到输入，保证两列相加等于总数
			name: "总数大于输出", completion: 800, total: 1000,
			wantPrompt: 200, wantCompletion: 800, wantTotal: 1000,
		},
		{
			// 只给总数、没给 completion：全部算作输入
			name: "只有总数", completion: 0, total: 500,
			wantPrompt: 500, wantCompletion: 0, wantTotal: 500,
		},
		{
			// 都没有：三个都为 0，日志页那两列会因为 >0 判断而留空
			name: "都没有", completion: 0, total: 0,
			wantPrompt: 0, wantCompletion: 0, wantTotal: 0,
		},
		{
			// 上游数据不自洽（completion > total）时不能算出负数
			name: "输出大于总数", completion: 900, total: 500,
			wantPrompt: 0, wantCompletion: 900, wantTotal: 500,
		},
	}

	for _, tc := range cases {
		got := taskUsageFromResult(&relaycommon.TaskInfo{
			CompletionTokens: tc.completion,
			TotalTokens:      tc.total,
		})
		if got.PromptTokens != tc.wantPrompt ||
			got.CompletionTokens != tc.wantCompletion ||
			got.TotalTokens != tc.wantTotal {
			t.Errorf("%s: got prompt=%d completion=%d total=%d, want %d/%d/%d",
				tc.name, got.PromptTokens, got.CompletionTokens, got.TotalTokens,
				tc.wantPrompt, tc.wantCompletion, tc.wantTotal)
		}
		if got.PromptTokens < 0 {
			t.Errorf("%s: 输入 token 为负数 %d", tc.name, got.PromptTokens)
		}
	}
}

// TestTaskUsageFromResultNil 空结果不能 panic —— 轮询路径上拿到 nil 是可能的。
func TestTaskUsageFromResultNil(t *testing.T) {
	if got := taskUsageFromResult(nil); got != (TaskUsage{}) {
		t.Errorf("nil 应返回零值，实际 %+v", got)
	}
}
