package doubao

import "github.com/QuantumNous/new-api/constant"

var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	// Seedance 2.0 · 国内
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-mini-260615",
	"doubao-seedance-2-0-fast-260128",
	// Seedance 2.0 · 国际
	"dreamina-seedance-2-0-260128",
	"dreamina-seedance-2-0-mini-260615",
	"dreamina-seedance-2-0-fast-260128",
	// Seedance 2.0 · 国际 · 大尺度
	"ep-20260414121243-hp7w5",
	"ep-20260414121306-pk5j6",
}

var ChannelName = "doubao-video"

// videoInputRatioMap 视频输入折扣比率（含视频单价 / 不含视频单价）。
// 管理员应将 ModelRatio 设置为"不含视频"的较高费率，
// 系统在检测到视频输入时自动乘以此折扣。
var videoInputRatioMap = map[string]float64{
	"doubao-seedance-2-0-260128":      28.0 / 46.0, // ~0.6087
	"doubao-seedance-2-0-fast-260128": 22.0 / 37.0, // ~0.5946
}

func GetVideoInputRatio(modelName string) (float64, bool) {
	r, ok := videoInputRatioMap[modelName]
	return r, ok
}

// GetModelRegion 返回模型所属的素材区域（cn / intl），未知模型返回空字符串。
//
// 素材引用（asset://）要求素材组 region 与模型区域一致：
// 国际版与大尺度模型只能引用 region=intl 的素材。
// 映射表本身放在 constant 包，以便 middleware 层无需依赖 relay 层即可引用。
func GetModelRegion(modelName string) string {
	return constant.GetSeedanceModelRegion(modelName)
}
