package constant

import "strings"

// Seedance 2.0 素材区域。
//
// 区域是上游素材组的属性：同一个上游账号下可以同时存在 cn 与 intl 素材组。
// 但模型是绑区域的——国际版与大尺度模型只能引用 region=intl 的素材，
// 因此提交生成任务时需要校验「素材区域」与「模型所属区域」是否一致。
const (
	SeedanceRegionCN   = "cn"
	SeedanceRegionINTL = "intl"
)

// seedanceModelRegions 是模型 ID 到区域的映射。
// 放在 constant 包而非 relay/channel/task/doubao，是为了让 middleware 也能引用
// 而不必依赖 relay 层。
var seedanceModelRegions = map[string]string{
	// 国内
	"doubao-seedance-2-0-260128":      SeedanceRegionCN,
	"doubao-seedance-2-0-mini-260615": SeedanceRegionCN,
	"doubao-seedance-2-0-fast-260128": SeedanceRegionCN,
	// 国际
	"dreamina-seedance-2-0-260128":      SeedanceRegionINTL,
	"dreamina-seedance-2-0-mini-260615": SeedanceRegionINTL,
	"dreamina-seedance-2-0-fast-260128": SeedanceRegionINTL,
	// 国际 · 大尺度
	"ep-20260414121243-hp7w5": SeedanceRegionINTL,
	"ep-20260414121306-pk5j6": SeedanceRegionINTL,
}

// GetSeedanceModelRegion 返回模型所属区域，未知模型返回空字符串。
//
// 返回空字符串时调用方应跳过区域校验而不是判定失败：
// 管理员可能通过模型重定向使用了自定义模型名。
func GetSeedanceModelRegion(modelName string) string {
	if modelName == "" {
		return ""
	}
	name := strings.ToLower(strings.TrimSpace(modelName))
	if region, ok := seedanceModelRegions[name]; ok {
		return region
	}
	// 兜底：按模型名前缀判断，覆盖后续新增的同系列模型
	switch {
	case strings.HasPrefix(name, "dreamina-seedance"):
		return SeedanceRegionINTL
	case strings.HasPrefix(name, "doubao-seedance-2"):
		return SeedanceRegionCN
	default:
		return ""
	}
}

// SeedanceModelsByRegion 返回某区域下的全部已知模型，供前端提示使用。
func SeedanceModelsByRegion(region string) []string {
	region = strings.ToLower(region)
	models := make([]string, 0, len(seedanceModelRegions))
	for name, r := range seedanceModelRegions {
		if r == region {
			models = append(models, name)
		}
	}
	return models
}
