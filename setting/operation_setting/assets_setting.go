package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// AssetsSetting 素材库（Seedance 2.0 assets）相关配置。
//
// 本期为单渠道模式：全站素材统一走一个上游账号（= 一个渠道）。
// ChannelId 为 0 时自动探测唯一一个启用的 Seedance 渠道，
// 探测到 0 个或多于 1 个时返回明确错误，提示管理员显式配置。
type AssetsSetting struct {
	// 素材渠道 ID，0 = 自动探测
	ChannelId int `json:"channel_id"`
	// 上游实现：auto（默认，按渠道 base_url 探测）/ seegen / stelloria。
	// 两家上游的协议差异极大，new-api 对下游统一暴露归一化契约，
	// 切换上游只需要改这一个配置，不用重启。
	Provider string `json:"provider"`
	// 每用户每分钟素材接口调用次数上限，0 = 不限
	RateLimitCount int `json:"rate_limit_count"`
	// 单次批量上传条数上限（上游硬上限为 50）
	BatchMaxItems int `json:"batch_max_items"`
	// 每用户素材总数上限，0 = 不限。
	// 上游对单账号素材总数与存储容量无配额上限，故默认放开，仅作为异常刷量时的应急开关。
	UserMaxTotal int `json:"user_max_total"`
	// Excel 批量上传文件大小上限（MB）
	UploadMaxFileMB int `json:"upload_max_file_mb"`
}

var assetsSetting = AssetsSetting{
	ChannelId:       0,
	Provider:        "auto",
	RateLimitCount:  60,
	BatchMaxItems:   50,
	UserMaxTotal:    0,
	UploadMaxFileMB: 10,
}

func init() {
	config.GlobalConfig.Register("assets_setting", &assetsSetting)
}

func GetAssetsSetting() *AssetsSetting {
	return &assetsSetting
}

func GetAssetsChannelId() int {
	return assetsSetting.ChannelId
}

// GetAssetsProvider 返回配置的上游实现名；"auto" 或空表示由调用方按渠道 base_url 探测。
func GetAssetsProvider() string {
	if assetsSetting.Provider == "" {
		return "auto"
	}
	return assetsSetting.Provider
}

func GetAssetsRateLimitCount() int {
	return assetsSetting.RateLimitCount
}

// GetAssetsBatchMaxItems 返回单批上限，并强制不超过上游的 50 条硬上限。
func GetAssetsBatchMaxItems() int {
	if assetsSetting.BatchMaxItems <= 0 || assetsSetting.BatchMaxItems > 50 {
		return 50
	}
	return assetsSetting.BatchMaxItems
}

func GetAssetsUserMaxTotal() int {
	return assetsSetting.UserMaxTotal
}

func GetAssetsUploadMaxBytes() int64 {
	mb := assetsSetting.UploadMaxFileMB
	if mb <= 0 {
		mb = 10
	}
	return int64(mb) * 1024 * 1024
}
