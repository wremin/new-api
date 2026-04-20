package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// QuotaUsageRatioSetting 用户使用量比例配置
type QuotaUsageRatioSetting struct {
	UsageRatio float64 `json:"usage_ratio"` // 用户使用量比例，默认 1.0（100%），大于 1 表示放大使用量，小于 1 表示缩小使用量
}

// 默认配置
var quotaUsageRatioSetting = QuotaUsageRatioSetting{
	UsageRatio: 1.0, // 默认 100% 不调整
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("quota_usage_ratio_setting", &quotaUsageRatioSetting)
}

// GetQuotaUsageRatioSetting 获取用户使用量比例配置
func GetQuotaUsageRatioSetting() *QuotaUsageRatioSetting {
	return &quotaUsageRatioSetting
}

// GetQuotaUsageRatio 获取用户使用量比例值
func GetQuotaUsageRatio() float64 {
	return quotaUsageRatioSetting.UsageRatio
}
