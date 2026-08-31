package yike

// ChannelName 是渠道的内部标识，用于日志与渠道名展示。
var ChannelName = "yike"

// 上游 RPC 常量。
const (
	// APIVersion 对应 alibabacloud_yike20260707 这个 SDK 版本。
	APIVersion = "2026-07-07"

	ActionSubmitJob         = "SubmitVideoGenerationJob"
	ActionGetJob            = "GetVideoGenerationJob"
	ActionImportMedia       = "ImportMedia"
	ActionGetMedia          = "GetMedia"
	ActionGetAccountCredit  = "GetYikeAccountCredit"
	ActionGetJobCredit      = "GetYikeJobCredit"
	DefaultScene            = "general"
	DefaultMediaAuthTimeout = 3600
)

// jobType 由输入素材的数量与类型决定，不是由调用方指定。
const (
	JobTypeText           = "text_to_video"
	JobTypeImage          = "image_to_video"
	JobTypeFirstLastFrame = "first_last_frame"
	JobTypeReference      = "reference_to_video"
)

// ModelList 是**对外**模型名。
//
// 上游真实模型名通过渠道的「模型重定向」映射过去，见 upstreamAliases 的注释。
// 这样换上游、换线路都不影响调用方，也不把上游品牌暴露出去。
var ModelList = []string{
	// Seedance 系（上游 Wonder-*）
	"seedance-2.0",
	"seedance-2.0-mini",
	"seedance-2.5",
	// 非 Seedance 内核的线路，保留独立命名
	"yk-happyhorse-1.0",
	"yk-happyhorse-1.1",
	"yk-wan-2.7",
	"yk-wan-3.0",
}

// SuggestedModelMapping 是建议写进渠道「模型重定向」的映射，供部署文档与前端提示使用。
//
// 代码本身不依赖它 —— 实际生效的是渠道配置里的 model_mapping，
// 由 helper.ModelMappedHelper 在提交前完成替换。这里只是给管理员一份可直接粘贴的参考，
// 避免手抖把 Wonder-Pro 写成 wonder-pro（上游对大小写和连字符零容忍）。
var SuggestedModelMapping = map[string]string{
	"seedance-2.0":      "Wonder-Standard",
	"seedance-2.0-mini": "Wonder-Pro",
	"seedance-2.5":      "Wonder-Ultra",
	"yk-happyhorse-1.0": "happyhorse-1.0",
	"yk-happyhorse-1.1": "happyhorse-1.1",
	"yk-wan-2.7":        "wan2.7",
	"yk-wan-3.0":        "wan3.0-video",
}

// ModelLimit 是各上游模型的参数边界。
type ModelLimit struct {
	MinDuration int
	MaxDuration int
	// MaxMedias 参考素材总数上限，0 表示未知/不限
	MaxMedias int
}

// upstreamLimits 以**上游**模型名为键 —— 校验发生在模型重定向之后，
// 此时拿到的是 info.UpstreamModelName。
var upstreamLimits = map[string]ModelLimit{
	"Wonder-Standard": {MinDuration: 4, MaxDuration: 15, MaxMedias: 15},
	"Wonder-Pro":      {MinDuration: 4, MaxDuration: 15, MaxMedias: 15},
	"Wonder-Ultra":    {MinDuration: 4, MaxDuration: 30, MaxMedias: 50},
}

// GetModelLimit 返回上游模型的参数边界。
// 未知模型返回 ok=false，调用方应跳过校验而不是判失败 ——
// 管理员可能通过模型重定向接了尚未收录的线路。
func GetModelLimit(upstreamModel string) (ModelLimit, bool) {
	l, ok := upstreamLimits[upstreamModel]
	return l, ok
}

// 支持的画幅。
var supportedAspectRatios = map[string]bool{
	"16:9": true, "9:16": true, "1:1": true, "4:3": true, "3:4": true,
}

// resolutionRatios 分辨率计费倍率。
// 后台应把模型价格设为 720P / 5 秒 的基准价，系统自动乘以 size × seconds。
var resolutionRatios = map[string]float64{
	"720P":  1.0,
	"1080P": 1.5,
}

// baseDurationSeconds 计费的时长单位：每满 5 秒算一个计费单位，不足向上取整。
const baseDurationSeconds = 5
