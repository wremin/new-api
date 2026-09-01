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
	"github.com/QuantumNous/new-api/logger"
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

	var mediaID string
	if aErr != nil {
		// 上游按 inputUrl 去重，同一 URL 第二次登记回 400 而不是回那条已有素材。
		// 但 mediaId 就写在错误文案里，抠出来当成功返回，让重复登记变成幂等 ——
		// 否则客户重复上传同一张图，拿到的是一句与「素材已经在那儿」毫无关联的报错。
		mediaID = yikeMediaIDInError(aErr.Message)
		if mediaID == "" {
			return AssetFields{}, aErr
		}
		logger.LogInfo(ctx, fmt.Sprintf(
			"yike: inputUrl already registered upstream, reusing mediaId %s", mediaID))
		data = nil
	} else {
		var raw map[string]any
		_ = common.Unmarshal(data, &raw)
		mediaID = stringField(raw, "MediaId", "mediaId", "MediaID", "Id", "id")
		if mediaID == "" {
			return AssetFields{}, NewAssetsError(AssetErrUpstream,
				"upstream did not return a MediaId", http.StatusBadGateway)
		}
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
	f, statusFound := parseYikeMedia(data)
	if f.OfficialId == "" {
		f.OfficialId = officialId
	}
	f.GroupId = yikeGroupID(f.GroupId)
	f.Raw = data

	// 读不到判据时状态同样是 Processing，但那是"我们不知道"而不是"上游还在处理"。
	// 不留声音的话，上游一改字段名，素材就会静默地永远停在处理中 ——
	// 这个坑真踩过：ThirdPartyAssetStatus 埋在二次编码的 JSON 字符串里，
	// 按顶层字段找不到，素材实际早已 Success，界面上却一直转圈。
	if !statusFound {
		logger.LogWarn(ctx, fmt.Sprintf(
			"yike: 素材 %s 的响应里找不到 ThirdPartyAssetStatus，暂按处理中对待；"+
				"上游响应结构可能已变更，请核对 parseYikeMedia", officialId))
	}
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
// 真实响应的层级（2026-08-31 实测，与参考文档出入很大）：
//
//	{
//	  "MediaInfo": {
//	    "MediaId": "f100...",                      ← 裸 32 位十六进制，不是 media-xxx
//	    "MediaBasicInfo":   { "Title", "MediaType", "InputURL", "Status": "Normal" },
//	    "FileInfoList":     [ { "FileBasicInfo": { "FileUrl": "<带签名的临时地址>" } } ],
//	    "MediaDynamicInfo": {
//	      "DynamicMetaData": { "Data": "{...ThirdPartyAssetStatus...}" }   ← JSON **字符串**
//	    }
//	  }
//	}
//
// 关键：ThirdPartyAssetStatus 埋在 DynamicMetaData.Data 里，而那是个**二次编码**的
// JSON 字符串，不是嵌套对象。按顶层字段去找必然找不到，素材会永远停在 Processing。
//
// 返回的 bool 表示是否真的读到了 ThirdPartyAssetStatus。读不到时状态同样是
// Processing，但那是"我们不知道"，不是"上游还在处理" —— 两者必须能被区分，
// 否则字段一改名，素材就静默地永远不可用（这个坑已经踩过一次）。
func parseYikeMedia(data []byte) (AssetFields, bool) {
	var root map[string]any
	if err := common.Unmarshal(data, &root); err != nil {
		return AssetFields{}, false
	}

	// 剥掉包装层。MediaInfo 是实测的形态，其余是兼容性兜底。
	media := root
	for _, key := range []string{"MediaInfo", "Media", "media"} {
		if inner, ok := root[key].(map[string]any); ok {
			media = inner
			break
		}
	}
	basic := yikeSubMap(media, "MediaBasicInfo", "mediaBasicInfo")

	f := AssetFields{
		// MediaId 在 MediaInfo 顶层和 MediaBasicInfo 里都有，两处都找
		OfficialId: yikeField(media, basic, "MediaId", "mediaId", "MediaID", "Id", "id"),
		Name:       yikeField(basic, media, "Title", "title", "Name", "name"),
		// Url 取用户当初提交的源地址，不用 FileInfoList 里那个带签名的临时地址
		Url:       yikeField(basic, media, "InputURL", "inputURL", "InputUrl", "Url", "URL", "url"),
		AssetType: normalizeAssetType(yikeField(basic, media, "MediaType", "mediaType")),
	}

	third, found := yikeThirdPartyStatus(media)
	if found {
		f.Status = normalizeAssetStatus(third)
	} else {
		// 读不到判据时保守判为处理中，绝不因为缺字段就判成 Active ——
		// 那会让生成任务带着不可用的素材提交，白扣一次额度。
		f.Status = model.AssetStatusProcessing
	}
	if f.Status == model.AssetStatusFailed {
		f.FailReason = yikeField(basic, media,
			"ThirdPartyAssetErrorMessage", "thirdPartyAssetErrorMessage",
			"ErrorMessage", "errorMessage")
	}

	if f.AssetType == "" && f.Url != "" {
		f.AssetType = model.GuessAssetType(f.Url)
	}
	return f, found
}

// yikeThirdPartyStatus 找出 ThirdPartyAssetStatus。
//
// 先看顶层（万一上游哪天扁平化了），再钻 MediaDynamicInfo.DynamicMetaData.Data ——
// 那是实测所在，且是个需要二次解析的 JSON 字符串。
func yikeThirdPartyStatus(media map[string]any) (string, bool) {
	if v := stringField(media, "ThirdPartyAssetStatus", "thirdPartyAssetStatus"); v != "" {
		return v, true
	}

	dyn := yikeSubMap(media, "MediaDynamicInfo", "mediaDynamicInfo")
	meta := yikeSubMap(dyn, "DynamicMetaData", "dynamicMetaData")
	rawData := stringField(meta, "Data", "data")
	if rawData == "" {
		return "", false
	}

	var inner map[string]any
	if err := common.Unmarshal([]byte(rawData), &inner); err != nil {
		return "", false
	}
	if v := stringField(inner, "ThirdPartyAssetStatus", "thirdPartyAssetStatus"); v != "" {
		return v, true
	}
	return "", false
}

// yikeSubMap 取子对象，取不到返回空 map 而不是 nil，省去调用方逐层判空。
func yikeSubMap(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		if sub, ok := m[k].(map[string]any); ok {
			return sub
		}
	}
	return map[string]any{}
}

// yikeField 依次在 primary、fallback 两层里找同一组键名。
func yikeField(primary, fallback map[string]any, keys ...string) string {
	if v := stringField(primary, keys...); v != "" {
		return v
	}
	return stringField(fallback, keys...)
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

// yikeMediaIDInError 从 MediaAlreadyExist 的错误文案里抠出已有素材的 mediaId。
//
// 上游按 inputUrl 去重：同一个 URL 第二次登记不会回那条已有素材，
// 而是回 400 MediaAlreadyExist —— 但它把 mediaId 明文写在了错误信息里：
//
//	MediaAlreadyExist: The media with the given inputUrl "https://..." has
//	already been registered with mediaId "0d6f1a50a5fd71f1802ae7e7d5496601".
//
// 解析错误文案是下策，但这是唯一的信息来源：上游没有「按 url 查 media」的接口。
// 兜底是安全的 —— 认不出就返回空串，调用方照常把原错误抛出去，不会比现在更糟。
// 换言之上游改文案的代价是「退回今天的行为」，而不是「拿到一个错的 id」。
//
// 不校验形态（实测是 32 位裸十六进制，但上游随时可能换），只要求它是一段
// 长度合理的标识符字符 —— 加形态校验只会把能用的素材挡在门外。
func yikeMediaIDInError(msg string) string {
	if !strings.Contains(msg, "MediaAlreadyExist") {
		return ""
	}
	// 用 LastIndex：inputUrl 本身也可能含 "mediaId" 字样，真正的那个在末尾
	i := strings.LastIndex(msg, "mediaId")
	if i < 0 {
		i = strings.LastIndex(msg, "MediaId")
	}
	if i < 0 {
		return ""
	}

	rest := msg[i+len("mediaId"):]
	start := -1
	for j, r := range rest {
		if isYikeIDRune(r) {
			start = j
			break
		}
		if !isYikeIDSeparator(r) {
			return "" // 出现意料之外的字符，说明文案不是预期格式，不猜
		}
	}
	if start < 0 {
		return ""
	}
	end := len(rest)
	for j, r := range rest[start:] {
		if !isYikeIDRune(r) {
			end = start + j
			break
		}
	}

	id := rest[start:end]
	if len(id) < 8 || len(id) > 128 {
		return "" // 太短多半抠到了别的词，太长不像 id
	}
	return id
}

func isYikeIDRune(r rune) bool {
	return r >= '0' && r <= '9' ||
		r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r == '-' || r == '_'
}

// isYikeIDSeparator 是 mediaId 与其值之间允许出现的字符。
// 中英文引号都认 —— ScrubUpstreamText 与上游本地化都可能换引号形态。
func isYikeIDSeparator(r rune) bool {
	switch r {
	case ' ', '\t', ':', '=', '"', '\'', '“', '”', '‘', '’', '「', '」':
		return true
	}
	return false
}
