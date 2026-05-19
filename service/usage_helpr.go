package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

//func GetPromptTokens(textRequest dto.GeneralOpenAIRequest, relayMode int) (int, error) {
//	switch relayMode {
//	case constant.RelayModeChatCompletions:
//		return CountTokenMessages(textRequest.Messages, textRequest.Model)
//	case constant.RelayModeCompletions:
//		return CountTokenInput(textRequest.Prompt, textRequest.Model), nil
//	case constant.RelayModeModerations:
//		return CountTokenInput(textRequest.Input, textRequest.Model), nil
//	}
//	return 0, errors.New("unknown relay mode")
//}

func ResponseText2Usage(c *gin.Context, responseText string, modeName string, promptTokens int) *dto.Usage {
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	usage := &dto.Usage{}
	usage.PromptTokens = promptTokens
	usage.CompletionTokens = EstimateTokenByModel(modeName, responseText)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}

// ApplyUsageRatioToUsage 将 usage_ratio 系数应用到 Usage 的 tokens 信息
// 策略：PromptTokens 保持不变，通过调整 CompletionTokens 来体现 usage_ratio
// 公式：新CompletionTokens = [(PromptTokens + CompletionTokens * CompletionRatio) * usageRatio - PromptTokens] / CompletionRatio
func ApplyUsageRatioToUsage(usage *dto.Usage, completionRatio float64) {
	if usage == nil {
		fmt.Printf("[ApplyUsageRatioToUsage] usage is nil, skipping\n")
		return
	}
	usageRatio := operation_setting.GetQuotaUsageRatio()
	fmt.Printf("[ApplyUsageRatioToUsage] usage_ratio=%.2f, completionRatio=%.2f, before: prompt=%d, completion=%d, total=%d\n",
		usageRatio, completionRatio, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)

	if usageRatio == 1.0 {
		fmt.Printf("[ApplyUsageRatioToUsage] usage_ratio is 1.0, no change needed\n")
		return
	}

	// 保存原始值用于日志
	originalPrompt := usage.PromptTokens
	originalCompletion := usage.CompletionTokens
	originalTotal := usage.TotalTokens

	// PromptTokens 保持不变，只调整 CompletionTokens
	// 原始配额 = PromptTokens + CompletionTokens * CompletionRatio
	// 新配额 = 原始配额 * usageRatio
	// 新CompletionTokens = (新配额 - PromptTokens) / CompletionRatio
	if completionRatio > 0 && originalCompletion > 0 {
		originalQuota := float64(originalPrompt) + float64(originalCompletion)*completionRatio
		newQuota := originalQuota * usageRatio
		newCompletionTokens := int((newQuota - float64(originalPrompt)) / completionRatio)

		usage.CompletionTokens = newCompletionTokens
		usage.TotalTokens = originalPrompt + newCompletionTokens

		// 同时调整详细信息中的 completion 相关 tokens
		if originalCompletion > 0 {
			ratio := float64(newCompletionTokens) / float64(originalCompletion)
			usage.CompletionTokenDetails.TextTokens = int(float64(usage.CompletionTokenDetails.TextTokens) * ratio)
			usage.CompletionTokenDetails.AudioTokens = int(float64(usage.CompletionTokenDetails.AudioTokens) * ratio)
			usage.CompletionTokenDetails.ReasoningTokens = int(float64(usage.CompletionTokenDetails.ReasoningTokens) * ratio)
		}
	}

	fmt.Printf("[ApplyUsageRatioToUsage] after: prompt=%d (unchanged), completion=%d->%d, total=%d->%d\n",
		originalPrompt,
		originalCompletion, usage.CompletionTokens,
		originalTotal, usage.TotalTokens)
}

func ValidUsage(usage *dto.Usage) bool {
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}
