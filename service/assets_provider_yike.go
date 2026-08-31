package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/aliyunsign"
	"github.com/QuantumNous/new-api/model"
)

// yikeAssetsProvider 对接「万象一刻」的素材登记。
//
// 与前三家上游的差异比它们彼此之间的差异还大，逐条对照：
//
//   - 鉴权：阿里云 RAM AK/SK（ACS3-HMAC-SHA256）。**没有**火山那样的多级密钥派生。
//     凭据直接取渠道 Key（形如 "AK|SK"），不像润元那样另挂 OtherSettings ——
//     万象一刻整个渠道只有这一组凭据，没有第二种鉴权方式需要分开存。
//   - 路径：阿里云 RPC，全部 POST 到根路径 "/"，动作放 x-acs-action 头。
//   - **没有素材组的概念**。上游只有扁平的 media 列表。
//     但 RelayAssetCreate 要求 groupId 非空，因此这里用一个合成的固定组 ID
//     （yikeDefaultGroupID）占位，让上层的分组契约仍然成立。
//   - **没有删除接口**。上游资料给出的 Action 只有 ImportMedia / GetMedia
//     （加上任务与计费四个）。DeleteAsset 因此只做本地清理，见下方注释。
//   - 素材可用的判据是 ThirdPartyAssetStatus == Success，
//     **不是** Status。Status 是上游自己的入库状态，它 Success 了不代表
//     Wonder 模型能引用这条素材，拿它判定会让生成请求在提交时才失败。
type yikeAssetsProvider struct{}

// ProviderYike 是内部标识符，会落进 assets.provider 列并参与切换比对，不可更改。
const ProviderYike ProviderName = "yike"

// yikeDefaultGroupID 是合成的默认组 ID。
//
// 上游没有分组，但 new-api 的素材契约以 groupId 为必填。用固定值而不是随机值，
// 这样同一渠道下的素材天然归到同一组，前端筛选与统计不会散架。
const yikeDefaultGroupID = "yike-default"

// yikeRegisterConfig 是 ImportMedia 的登记选项。
//
// NeedThirdPartyAsset 必须为 true —— 它才是让素材进入 Wonder 可引用状态的开关，
// 也是 ThirdPartyAssetStatus 这个字段存在的前提。设成 false 素材能登记成功，
// 但生成任务永远引用不到，且失败信息与本参数毫无关联，极难排查。
const yikeRegisterConfig = `{"NeedThirdPartyAsset":true,"NeedSnapshot":true}`

func (p *yikeAssetsProvider) Name() ProviderName { return ProviderYike }

func (p *yikeAssetsProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		// 上游无批量接口，由 fallbackBatchCreate 退化成循环单条，对下游仍可用
		BatchCreate:   true,
		ExcelTemplate: false,
		Regions:       false,
		// 上游没有素材组概念，留空让前端隐藏分组类型入口
		GroupTypes:  nil,
		RenameAsset: false,
		DeleteGroup: false,
	}
}

// callYike 签名并发起一次 RPC 调用，返回响应体原文。
func callYike(ctx context.Context, ch *model.Channel, action string, payload map[string]any) ([]byte, *AssetsError) {
	creds, err := aliyunsign.ParseAKSK(ch.Key)
	if err != nil {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"yike channel key must be in 'AccessKeyId|AccessKeySecret' format", http.StatusServiceUnavailable)
	}

	baseURL := strings.TrimSuffix(ch.GetBaseURL(), "/")
	if baseURL == "" {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"assets channel base url is empty", http.StatusServiceUnavailable)
	}

	body, mErr := common.Marshal(payload)
	if mErr != nil {
		return nil, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request body: "+mErr.Error(), http.StatusInternalServerError)
	}

	reqCtx, cancel := context.WithTimeout(ctx, assetsUpstreamTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return nil, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request: "+err.Error(), http.StatusInternalServerError)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if err := aliyunsign.Sign(httpReq, creds, body, aliyunsign.Options{
		Action:  action,
		Version: yikeAPIVersion,
	}); err != nil {
		return nil, NewAssetsError(AssetErrInvalidRequest,
			"failed to sign upstream request: "+err.Error(), http.StatusInternalServerError)
	}

	client, err := GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to create http client: "+err.Error(), http.StatusInternalServerError)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"upstream request failed: "+err.Error(), http.StatusBadGateway)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to read upstream response: "+err.Error(), http.StatusBadGateway)
	}

	// 阿里云 RPC 的业务错误走 HTTP 4xx/5xx + {Code,Message}，
	// 不像润元那样 200 里藏错误，因此按状态码判定即可。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		msg := ExtractUpstreamAssetError(respBody)
		if uErr := common.Unmarshal(respBody, &e); uErr == nil && e.Code != "" {
			msg = fmt.Sprintf("%s: %s", e.Code, e.Message)
		}
		return nil, NewAssetsError(AssetErrUpstream,
			ScrubUpstreamText(msg), yikeErrorStatus(resp.StatusCode, e.Code))
	}
	return respBody, nil
}

// yikeAPIVersion 与 relay/channel/task/yike 里的 APIVersion 必须一致。
// 这里不 import 那个包：service 是被 relay 依赖的下层，反向引用会成环。
const yikeAPIVersion = "2026-07-07"

// yikeErrorStatus 把上游错误码映射成对下游有意义的状态码。
// 含 NotFound 的码要映射成 404，否则 SyncAssetFromUpstream 无法软删除已消失的素材。
func yikeErrorStatus(httpStatus int, code string) int {
	if strings.Contains(code, "NotFound") || strings.Contains(code, "NotExist") {
		return http.StatusNotFound
	}
	if httpStatus >= 400 {
		return httpStatus
	}
	return http.StatusBadGateway
}

// ============================
// AssetsProvider 接口实现
// ============================

// CreateAsset 调用 ImportMedia 登记一条素材。
//
// 登记是**异步**的：接口只回 MediaId，可用与否要靠 GetMedia 轮询
// ThirdPartyAssetStatus。因此这里恒定返回 Processing 状态，
// 由上层的同步机制去推进到 Active。
func (p *yikeAssetsProvider) CreateAsset(ctx context.Context, ch *model.Channel, in CreateAssetInput) (AssetFields, *AssetsError) {
	if strings.TrimSpace(in.Url) == "" {
		return AssetFields{}, NewAssetsError(AssetErrInvalidRequest,
			"asset url is required", http.StatusBadRequest)
	}

	mediaType := yikeMediaType(in.AssetType, in.Url)
	payload := map[string]any{
		"ImportSource":   "url",
		"InputURL":       in.Url,
		"MediaType":      mediaType,
		"RegisterConfig": yikeRegisterConfig,
	}
	if in.Name != "" {
		payload["Title"] = in.Name
	}

	data, aErr := callYike(ctx, ch, "ImportMedia", payload)
	if aErr != nil {
		return AssetFields{}, aErr
	}

	var raw map[string]any
	_ = common.Unmarshal(data, &raw)
	mediaID := stringField(raw, "MediaId", "mediaId", "MediaID", "Id", "id")
	if mediaID == "" {
		return AssetFields{}, NewAssetsError(AssetErrUpstream,
			"upstream did not return a MediaId", http.StatusBadGateway)
	}

	return AssetFields{
		OfficialId: mediaID,
		GroupId:    yikeGroupID(in.GroupId),
		Name:       in.Name,
		Url:        in.Url,
		AssetType:  normalizeAssetType(mediaType),
		// 登记是异步的，此刻一定还没到可引用状态
		Status: model.AssetStatusProcessing,
		Raw:    data,
	}, nil
}

// GetAsset 调用 GetMedia 查询登记状态。
func (p *yikeAssetsProvider) GetAsset(ctx context.Context, ch *model.Channel, officialId string) (AssetFields, *AssetsError) {
	data, aErr := callYike(ctx, ch, "GetMedia", map[string]any{
		"MediaId":     officialId,
		"AuthTimeout": yikeAuthTimeout,
	})
	if aErr != nil {
		return AssetFields{}, aErr
	}
	f := parseYikeMedia(data)
	if f.OfficialId == "" {
		f.OfficialId = officialId
	}
	f.GroupId = yikeGroupID(f.GroupId)
	f.Raw = data
	return f, nil
}

// yikeAuthTimeout 是 GetMedia 返回的素材访问地址的有效期（秒）。
const yikeAuthTimeout = 3600

// DeleteAsset 上游没有删除接口，只能做本地清理。
//
// 返回 nil 而不是 501：调用方的语义是「把这条素材从本站移除」，本地记录清掉
// 这个目标已经达成。返回 501 会让本地记录留在库里，用户反复删除反复失败，
// 比"上游还残留一条孤儿记录"糟糕得多。孤儿素材不占用户配额，也不会被引用到。
func (p *yikeAssetsProvider) DeleteAsset(_ context.Context, _ *model.Channel, _ string) *AssetsError {
	return nil
}

func (p *yikeAssetsProvider) BatchCreateAssets(ctx context.Context, ch *model.Channel, ins []CreateAssetInput) (string, []BatchItemResult, *AssetsError) {
	return fallbackBatchCreate(ctx, ch, ins, p.CreateAsset)
}

func (p *yikeAssetsProvider) BatchCreateFromExcel(_ context.Context, _ *model.Channel, _ string, _ []byte) (string, []BatchItemResult, *AssetsError) {
	return "", nil, ErrCapabilityUnsupported(ProviderYike, "excel batch upload")
}

func (p *yikeAssetsProvider) ExcelTemplate(_ context.Context, _ *model.Channel) (*http.Response, *AssetsError) {
	return nil, ErrCapabilityUnsupported(ProviderYike, "excel template download")
}

// CreateGroup 上游没有分组概念，返回那个合成的默认组。
//
// 同样不返回 501：素材创建流程需要一个 groupId 才能走下去，
// 直接拒绝会让整条素材链路对万象一刻不可用。这里把「组」降级成一个纯本地的
// 命名空间，名字沿用调用方传的，ID 固定。
func (p *yikeAssetsProvider) CreateGroup(_ context.Context, _ *model.Channel, in CreateGroupInput) (GroupFields, *AssetsError) {
	name := in.Name
	if name == "" {
		name = "default"
	}
	return GroupFields{
		OfficialId:  yikeDefaultGroupID,
		Name:        name,
		Description: in.Description,
	}, nil
}

// ============================
// 解析辅助
// ============================

// parseYikeMedia 把 GetMedia 的响应翻译成归一化字段。
//
// 响应可能是 {"Media":{...}} 包装，也可能是扁平的，两种都试。
func parseYikeMedia(data []byte) AssetFields {
	var raw map[string]any
	if err := common.Unmarshal(data, &raw); err != nil {
		return AssetFields{}
	}
	// 优先取包装层
	for _, key := range []string{"Media", "media", "MediaInfo"} {
		if inner, ok := raw[key].(map[string]any); ok {
			return yikeMediaFields(inner)
		}
	}
	return yikeMediaFields(raw)
}

func yikeMediaFields(raw map[string]any) AssetFields {
	f := AssetFields{
		OfficialId: stringField(raw, "MediaId", "mediaId", "MediaID", "Id", "id"),
		Name:       stringField(raw, "Title", "title", "Name", "name"),
		Url:        stringField(raw, "Url", "URL", "url", "InputURL", "inputURL"),
		AssetType:  normalizeAssetType(stringField(raw, "MediaType", "mediaType")),
		FailReason: stringField(raw,
			"ThirdPartyAssetErrorMessage", "thirdPartyAssetErrorMessage",
			"ErrorMessage", "errorMessage"),
	}

	// 关键：可用性看 ThirdPartyAssetStatus，不是 Status。
	// Status 只说明上游自己入库成功，不代表 Wonder 模型能引用它。
	third := stringField(raw, "ThirdPartyAssetStatus", "thirdPartyAssetStatus")
	if third != "" {
		f.Status = normalizeAssetStatus(third)
	} else {
		// 字段缺失时保守判为处理中，绝不因为缺字段就判成 Active ——
		// 那会让生成任务带着不可用的素材提交，白扣一次额度。
		f.Status = model.AssetStatusProcessing
	}

	if f.AssetType == "" && f.Url != "" {
		f.AssetType = model.GuessAssetType(f.Url)
	}
	return f
}

// yikeMediaType 归一化成上游要的 image / video / audio。
func yikeMediaType(assetType, rawURL string) string {
	t := strings.ToLower(strings.TrimSpace(assetType))
	if t == "" {
		t = strings.ToLower(model.GuessAssetType(rawURL))
	}
	switch {
	case strings.Contains(t, "video"):
		return "video"
	case strings.Contains(t, "audio"):
		return "audio"
	default:
		return "image"
	}
}

// yikeGroupID 把空组 ID 补成默认组。
func yikeGroupID(groupID string) string {
	if strings.TrimSpace(groupID) == "" {
		return yikeDefaultGroupID
	}
	return groupID
}
