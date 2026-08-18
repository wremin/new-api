package service

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// ============================
// 上游素材库 Provider 抽象
// ============================
//
// 本期仍然是"单渠道"：全站素材共用一个上游账号。
// 但上游本身要可切换——目前支持两家，它们的协议差异非常大：
//
//	           seegen.ai                     Stelloria
//	路径       RESTful /v1/assets            RPC 风 /api/aicc/assets/query
//	查列表     GET + query 参数              POST + JSON body
//	响应       裸对象                        {code,message,data,traceId,timestamp} 包装
//	素材 ID    officialId                    assetId
//	分区       region: cn / intl             无，改为 groupType: AIGC / LivenessFace
//	批量       JSON 数组 + Excel             无
//
// 因此 new-api 对下游**不再原样透传上游响应**——那样会导致换个上游客户端就得改代码，
// 完全违背网关的意义。对下游统一暴露归一化契约（字段名沿用 seegen 的形态，
// 保证已按 seegen 文档接入的客户端不受影响），由各 provider 负责翻译。

type ProviderName = string

const (
	ProviderSeegen    ProviderName = "seegen"
	ProviderStelloria ProviderName = "stelloria"
	ProviderRunyuan   ProviderName = "runyuan"
)

// ProviderCapabilities 声明上游支持哪些能力。
// 控制器与前端据此降级：不支持的能力返回 501 / 隐藏入口，
// 而不是发一个注定失败的请求给上游。
type ProviderCapabilities struct {
	// BatchCreate 是否支持一次提交多条素材。
	// 为 false 时 new-api 会退化成循环单条创建（见 fallbackBatchCreate）。
	BatchCreate bool
	// ExcelTemplate 是否支持 Excel 模板下载与表格批量上传
	ExcelTemplate bool
	// Regions 是否有区域概念（cn / intl）。为 false 时跳过素材与模型的区域一致性校验。
	Regions bool
	// GroupTypes 素材组类型枚举，为空表示上游没有该概念
	GroupTypes []string
	// RenameAsset 是否支持素材改名
	RenameAsset bool
	// DeleteGroup 是否支持删除素材组
	DeleteGroup bool
}

// GroupFields 是归一化后的素材组信息。
type GroupFields struct {
	OfficialId  string
	Name        string
	Description string
	// Region 仅 seegen 有（cn / intl）
	Region string
	// GroupType 仅 Stelloria 有（AIGC / LivenessFace）
	GroupType  string
	UpstreamId int64
	Raw        []byte
}

// CreateAssetInput 是创建单个素材的归一化入参。
type CreateAssetInput struct {
	GroupId   string
	Url       string
	Name      string
	AssetType string // Image / Video / Audio，为空时由 provider 按 URL 推断
}

// CreateGroupInput 是创建素材组的归一化入参。
type CreateGroupInput struct {
	Name        string
	Description string
	Region      string // seegen
	GroupType   string // Stelloria
}

// BatchItemResult 对齐 seegen 批量响应中的单条结果，Stelloria 下由循环单条创建拼装。
type BatchItemResult struct {
	Index      int    `json:"index"`
	Status     string `json:"status"` // ok / error
	OfficialId string `json:"officialId,omitempty"`
	Error      string `json:"error,omitempty"`
}

// AssetsProvider 是上游素材库的统一接口。
type AssetsProvider interface {
	Name() ProviderName
	Capabilities() ProviderCapabilities

	CreateAsset(ctx context.Context, ch *model.Channel, in CreateAssetInput) (AssetFields, *AssetsError)
	GetAsset(ctx context.Context, ch *model.Channel, officialId string) (AssetFields, *AssetsError)
	DeleteAsset(ctx context.Context, ch *model.Channel, officialId string) *AssetsError

	// BatchCreateAssets 返回 batchId 与逐条结果。
	// 上游没有批量接口时由 fallbackBatchCreate 循环单条实现，batchId 为空。
	BatchCreateAssets(ctx context.Context, ch *model.Channel, ins []CreateAssetInput) (string, []BatchItemResult, *AssetsError)

	// BatchCreateFromExcel 透传 multipart 表格上传，不支持时返回 ErrCapabilityUnsupported。
	BatchCreateFromExcel(ctx context.Context, ch *model.Channel, contentType string, body []byte) (string, []BatchItemResult, *AssetsError)

	// ExcelTemplate 返回未读取的模板下载响应，调用方负责关闭 Body。
	ExcelTemplate(ctx context.Context, ch *model.Channel) (*http.Response, *AssetsError)

	CreateGroup(ctx context.Context, ch *model.Channel, in CreateGroupInput) (GroupFields, *AssetsError)
}

// ErrCapabilityUnsupported 构造"当前上游不支持该能力"的错误。
func ErrCapabilityUnsupported(provider ProviderName, capability string) *AssetsError {
	return NewAssetsError(AssetErrUnsupported,
		"upstream provider "+PublicProviderName(provider)+" does not support "+capability,
		http.StatusNotImplemented)
}

// ============================
// 上游身份白标
// ============================
//
// 内部标识符（stelloria / seegen）会落进 assets.provider 列并参与切换比对，
// **绝对不能改**。这里只在响应边界做展示层替换，让下游看不到真实供应商是谁。

// providerPublicNames 内部标识符 → 对外展示名。
// 未列出的上游按原样展示。
var providerPublicNames = map[ProviderName]string{
	ProviderStelloria: "metamind",
}

// providerScrubPatterns 需要从透传文本里清洗掉的上游痕迹。
// 顺序有意义：先替换更长的域名，再替换裸词，避免出现 "metamind.link" 这种半截结果。
var providerScrubPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)stelloria\.link`), "metamind.yun"},
	{regexp.MustCompile(`(?i)stelloria`), "metamind"},
}

// PublicProviderName 返回对外展示的上游名。
func PublicProviderName(name ProviderName) string {
	if public, ok := providerPublicNames[name]; ok {
		return public
	}
	return name
}

// ScrubUpstreamText 清洗透传给下游的上游文本（错误信息等）中的供应商痕迹。
//
// 上游的报错常常带着自己的品牌与域名，原样透传等于把供应商暴露给客户。
// 大小写按原文形态还原：Stelloria→Metamind、STELLORIA→METAMIND。
func ScrubUpstreamText(s string) string {
	if s == "" {
		return s
	}
	for _, p := range providerScrubPatterns {
		s = p.pattern.ReplaceAllStringFunc(s, func(match string) string {
			return matchCase(match, p.replacement)
		})
	}
	return s
}

// matchCase 让替换结果沿用原文的大小写形态。
func matchCase(original, replacement string) string {
	switch {
	case original == strings.ToUpper(original) && original != strings.ToLower(original):
		return strings.ToUpper(replacement)
	case len(original) > 0 && unicode.IsUpper(rune(original[0])):
		if len(replacement) == 0 {
			return replacement
		}
		return strings.ToUpper(replacement[:1]) + replacement[1:]
	default:
		return replacement
	}
}

// ============================
// Provider 解析
// ============================

var (
	seegenProvider    = &seegenAssetsProvider{}
	stelloriaProvider = &stelloriaAssetsProvider{}
	runyuanProvider   = &runyuanAssetsProvider{}
)

// GetAssetsProvider 决定当前渠道使用哪个上游实现。
//
// 解析顺序：
//  1. 配置项 assets_setting.provider 显式指定（seegen / stelloria）；
//  2. auto（默认）时按渠道 base_url 探测；
//  3. 都识别不出来时回落到 seegen——它是本项目最早接入、字段最全的上游。
func GetAssetsProvider(ch *model.Channel) AssetsProvider {
	switch strings.ToLower(strings.TrimSpace(operation_setting.GetAssetsProvider())) {
	case ProviderSeegen:
		return seegenProvider
	case ProviderStelloria:
		return stelloriaProvider
	case ProviderRunyuan:
		return runyuanProvider
	}

	baseURL := ""
	if ch != nil {
		baseURL = strings.ToLower(ch.GetBaseURL())
	}
	switch {
	case strings.Contains(baseURL, "stelloria"):
		return stelloriaProvider
	case strings.Contains(baseURL, "runy") || strings.Contains(baseURL, "yitd"):
		return runyuanProvider
	case strings.Contains(baseURL, "seegen"):
		return seegenProvider
	default:
		return seegenProvider
	}
}

// fallbackBatchCreate 在上游没有批量接口时，循环单条创建。
//
// 逐条独立提交，单条失败不影响其余条目，结果按 index 对齐，
// 与 seegen 原生批量响应的语义保持一致。
func fallbackBatchCreate(
	ctx context.Context,
	ch *model.Channel,
	ins []CreateAssetInput,
	create func(context.Context, *model.Channel, CreateAssetInput) (AssetFields, *AssetsError),
) (string, []BatchItemResult, *AssetsError) {
	results := make([]BatchItemResult, 0, len(ins))
	for i, in := range ins {
		fields, aErr := create(ctx, ch, in)
		if aErr != nil {
			results = append(results, BatchItemResult{
				Index:  i,
				Status: "error",
				Error:  aErr.Message,
			})
			continue
		}
		results = append(results, BatchItemResult{
			Index:      i,
			Status:     "ok",
			OfficialId: fields.OfficialId,
		})
	}
	// 上游没有 batchId 的概念，返回空串，由调用方决定是否自行生成
	return "", results, nil
}

// normalizeAssetStatus 把各上游的状态归一化成 Processing / Active / Failed。
//
// 注意 Stelloria 文档里同一个字段既出现过 "ACTIVE" 也出现过 "Active"，
// 所以这里必须大小写不敏感，否则素材永远不会被判定为可用。
func normalizeAssetStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "active", "success", "succeeded", "done", "passed":
		return model.AssetStatusActive
	case "failed", "failure", "rejected", "error":
		return model.AssetStatusFailed
	case "":
		return ""
	default:
		// processing / pending / running / reviewing 等一律视为审核中
		return model.AssetStatusProcessing
	}
}
