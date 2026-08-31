package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// 素材接口的业务错误码，直接透出给客户端（OpenAI 风格 error.code）。
const (
	AssetErrChannelNotConfigured = "assets_channel_not_configured"
	AssetErrChannelAmbiguous     = "assets_channel_ambiguous"
	AssetErrChannelUnavailable   = "assets_channel_unavailable"
	AssetErrNotFound             = "asset_not_found"
	AssetErrNotActive            = "asset_not_active"
	AssetErrRegionMismatch       = "asset_region_mismatch"
	AssetErrUpstream             = "asset_upstream_error"
	AssetErrInvalidRequest       = "asset_invalid_request"
	AssetErrQuotaExceeded        = "asset_quota_exceeded"
	AssetErrUnsupported          = "asset_unsupported_by_provider"
	AssetErrProviderMismatch     = "asset_provider_mismatch"
)

// assetsChannelTypes 是可以承载素材接口的渠道类型。
var assetsChannelTypes = []int{
	constant.ChannelTypeDoubaoVideo,
	constant.ChannelTypeVolcEngine,
	// 万象一刻的参考素材必须先经 ImportMedia 登记才能被生成任务引用，
	// 不进这个列表素材接口就永远解析不到它的渠道。
	constant.ChannelTypeYike,
}

// AssetsError 携带业务错误码与 HTTP 状态码，便于控制器统一转成 OpenAI 风格错误体。
type AssetsError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *AssetsError) Error() string {
	return e.Message
}

func NewAssetsError(code, message string, statusCode int) *AssetsError {
	return &AssetsError{Code: code, Message: message, StatusCode: statusCode}
}

// GetAssetsChannel 返回本期唯一的素材渠道。
//
// 单渠道模式（见 PRD §4.1）：
//  1. 优先取系统设置 assets_setting.channel_id；
//  2. 未配置时自动探测唯一一个启用的 Seedance 渠道；
//  3. 探测到 0 个 -> assets_channel_not_configured；多于 1 个 -> assets_channel_ambiguous。
//
// 不在启动时校验（渠道可能后建），首次调用素材接口时才校验。
//
// Deprecated: 优先用 GetAssetsChannelForGroup。多上游共存时，
// 不带分组的解析必然在两个渠道之间二选一失败。
func GetAssetsChannel() (*model.Channel, *AssetsError) {
	return GetAssetsChannelForGroup("")
}

// AssetsRequestGroup 取本次请求应当使用的分组。
//
// API 客户端走 TokenAuth，分组已在 middleware/auth.go 写进上下文；
// 控制台会话鉴权只 set 了 id（见 TokenOrUserAuth 的 session 分支），
// 因此要回落到用户自身的分组，否则控制台上传永远匹配不到渠道。
func AssetsRequestGroup(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if g := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); g != "" {
		return g
	}
	if userId := c.GetInt("id"); userId > 0 {
		if g, err := model.GetUserGroup(userId, false); err == nil {
			return g
		}
	}
	return ""
}

// GetAssetsChannelForGroup 按请求分组解析素材渠道。
//
// 为什么必须按分组解析：视频任务的渠道本来就是 distributor 按分组选的，
// 而素材渠道过去是全局单选。两者一旦指向不同上游，用户上传的素材
// 就注定在执行任务的那个上游无效（asset_provider_mismatch）。
// 按同一个维度解析，才能保证"素材落在哪"和"任务在哪跑"是一致的。
//
// 解析顺序（刻意保守，确保存量部署行为不变）：
//  1. assets_setting.channel_id 显式指定 —— 最高优先级，与改动前完全一致；
//  2. 按分组筛选，恰好命中一个就用它；
//  3. 分组筛不出结果时回落到旧的全局单选逻辑 ——
//     没给渠道配分组的老部署不能因为这次改动突然不能传素材。
func GetAssetsChannelForGroup(group string) (*model.Channel, *AssetsError) {
	if id := operation_setting.GetAssetsChannelId(); id > 0 {
		ch, err := model.CacheGetChannel(id)
		if err != nil || ch == nil {
			return nil, NewAssetsError(AssetErrChannelUnavailable,
				fmt.Sprintf("configured assets channel #%d is unavailable", id),
				http.StatusServiceUnavailable)
		}
		if ch.Status != common.ChannelStatusEnabled {
			return nil, NewAssetsError(AssetErrChannelUnavailable,
				fmt.Sprintf("configured assets channel #%d is disabled", id),
				http.StatusServiceUnavailable)
		}
		return ch, nil
	}

	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"failed to enumerate channels: "+err.Error(), http.StatusInternalServerError)
	}

	var matched []*model.Channel
	for _, ch := range channels {
		if ch == nil || ch.Status != common.ChannelStatusEnabled {
			continue
		}
		for _, t := range assetsChannelTypes {
			if ch.Type == t {
				matched = append(matched, ch)
				break
			}
		}
	}

	return selectAssetsChannel(matched, group)
}

// selectAssetsChannel 从候选渠道中挑一个，是分组路由的**全部决策逻辑**。
//
// 单独拆出来是因为它不碰数据库，可以被完整覆盖测试 ——
// 这段逻辑决定素材落到哪个上游，选错的代价是客户的素材集体失效，
// 不适合只靠"看起来对"。
func selectAssetsChannel(matched []*model.Channel, group string) (*model.Channel, *AssetsError) {
	// 分组筛选只在确实有歧义时介入。单渠道部署完全不受影响，连分组都不用配。
	if group != "" && len(matched) > 1 {
		var byGroup []*model.Channel
		for _, ch := range matched {
			if common.StringsContains(ch.GetGroups(), group) {
				byGroup = append(byGroup, ch)
			}
		}
		switch len(byGroup) {
		case 1:
			return byGroup[0], nil
		case 0:
			// 一个都没匹配上：多半是渠道压根没配分组（老部署）。
			// 此时不报错，落回下面的旧逻辑，保持与改动前一致。
		default:
			return nil, NewAssetsError(AssetErrChannelAmbiguous,
				fmt.Sprintf("group %q matches %d assets-capable channels, "+
					"please narrow the channel groups or set assets channel id explicitly",
					group, len(byGroup)),
				http.StatusServiceUnavailable)
		}
	}

	switch len(matched) {
	case 0:
		return nil, NewAssetsError(AssetErrChannelNotConfigured,
			"no enabled Seedance channel found, please configure assets channel in operation settings",
			http.StatusServiceUnavailable)
	case 1:
		return matched[0], nil
	default:
		// 有多个候选，而分组既没能区分、也没配。
		// 报错里带上分组，否则管理员只看到"有 N 个渠道"，无从下手。
		hint := "please set assets channel id explicitly in operation settings"
		if group != "" {
			hint = fmt.Sprintf("no channel is bound to group %q; "+
				"assign the group to exactly one of them, or set assets channel id explicitly", group)
		}
		return nil, NewAssetsError(AssetErrChannelAmbiguous,
			fmt.Sprintf("found %d enabled Seedance channels, %s", len(matched), hint),
			http.StatusServiceUnavailable)
	}
}

// AssetsUpstreamRequest 描述一次到上游素材接口的调用。
type AssetsUpstreamRequest struct {
	Method string
	// Path 是相对上游 base url 的路径，例如 /v1/assets
	Path        string
	Query       url.Values
	Body        []byte
	ContentType string
	// RawBody 用于 multipart 透传（Excel 批量上传），设置后忽略 Body
	RawBody io.Reader
}

// AssetsUpstreamResponse 是上游的原始响应，控制器负责按需透传或解析。
type AssetsUpstreamResponse struct {
	StatusCode  int
	Body        []byte
	ContentType string
}

func (r *AssetsUpstreamResponse) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

const assetsUpstreamTimeout = 60 * time.Second

// DoAssetsUpstreamRequest 向上游素材接口发起请求。
// 响应体一次性读入内存——素材接口的响应都很小，唯一的例外是 Excel 模板下载，
// 由 StreamAssetsUpstream 单独处理。
func DoAssetsUpstreamRequest(ctx context.Context, channel *model.Channel, req AssetsUpstreamRequest) (*AssetsUpstreamResponse, *AssetsError) {
	// 响应体在本函数内读完，因此可以安全地在这里 defer cancel。
	reqCtx, cancel := context.WithTimeout(ctx, assetsUpstreamTimeout)
	defer cancel()

	httpReq, aErr := buildAssetsUpstreamRequest(reqCtx, channel, req)
	if aErr != nil {
		return nil, aErr
	}

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to create http client: "+err.Error(), http.StatusInternalServerError)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"upstream request failed: "+err.Error(), http.StatusBadGateway)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to read upstream response: "+err.Error(), http.StatusBadGateway)
	}

	return &AssetsUpstreamResponse{
		StatusCode:  resp.StatusCode,
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

// StreamAssetsUpstream 返回未读取的上游响应，供二进制流式透传（Excel 模板下载）。
// 调用方负责关闭 resp.Body。
func StreamAssetsUpstream(ctx context.Context, channel *model.Channel, req AssetsUpstreamRequest) (*http.Response, *AssetsError) {
	httpReq, aErr := buildAssetsUpstreamRequest(ctx, channel, req)
	if aErr != nil {
		return nil, aErr
	}
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to create http client: "+err.Error(), http.StatusInternalServerError)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"upstream request failed: "+err.Error(), http.StatusBadGateway)
	}
	return resp, nil
}

func buildAssetsUpstreamRequest(ctx context.Context, channel *model.Channel, req AssetsUpstreamRequest) (*http.Request, *AssetsError) {
	baseURL := strings.TrimSuffix(channel.GetBaseURL(), "/")
	if baseURL == "" {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"assets channel base url is empty", http.StatusServiceUnavailable)
	}

	fullURL := baseURL + req.Path
	if len(req.Query) > 0 {
		fullURL = fullURL + "?" + req.Query.Encode()
	}

	var bodyReader io.Reader
	if req.RawBody != nil {
		bodyReader = req.RawBody
	} else if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	// 这里直接使用调用方传入的 ctx：
	// DoAssetsUpstreamRequest 会在外层套上超时并负责 cancel；
	// StreamAssetsUpstream 需要在函数返回后继续读 body，不能在此处设超时。
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL, bodyReader)
	if err != nil {
		return nil, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request: "+err.Error(), http.StatusInternalServerError)
	}

	key, _, keyErr := channel.GetNextEnabledKey()
	if keyErr != nil {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"assets channel has no available key", http.StatusServiceUnavailable)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Accept", "application/json")
	if req.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	} else if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	return httpReq, nil
}

// ExtractUpstreamAssetError 从上游错误响应中提取可读信息。
func ExtractUpstreamAssetError(body []byte) string {
	return ScrubUpstreamText(extractUpstreamAssetErrorRaw(body))
}

func extractUpstreamAssetErrorRaw(body []byte) string {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return strings.TrimSpace(string(body))
	}
	if msg, ok := m["message"].(string); ok && msg != "" {
		return msg
	}
	if errObj, ok := m["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && msg != "" {
			return msg
		}
	}
	if msg, ok := m["msg"].(string); ok && msg != "" {
		return msg
	}
	return strings.TrimSpace(string(body))
}

// ============================
// 上游素材字段解析
// ============================

// AssetFields 是从上游素材响应（或下游请求体）中提取的、需要落库的字段。
// 这是各 provider 之间的归一化中间形态：seegen 的 officialId 与 Stelloria 的 assetId
// 都落到 OfficialId，其余字段同理，见各 provider 的 parse 函数。
type AssetFields struct {
	OfficialId string
	GroupId    string
	Name       string
	Status     string
	// Region 仅 seegen 有（cn / intl）；Stelloria 下恒为空
	Region     string
	Url        string
	AssetType  string
	UpstreamId int64
	FailReason string
	// Raw 是上游原始响应，落库备查
	Raw []byte
}

// AssetFallbackFields 构造用于兜底的字段集合：
// 上游响应没回显的字段（典型如 name / groupId）用下游请求体里的值补齐。
func AssetFallbackFields(officialId, groupId, name, rawUrl, region string) AssetFields {
	return AssetFields{
		OfficialId: officialId,
		GroupId:    groupId,
		Name:       name,
		Url:        rawUrl,
		Region:     region,
		AssetType:  model.GuessAssetType(rawUrl),
	}
}

// parseUpstreamAsset 从上游素材对象中提取落库字段。
//
// 注意：上游文档只给出了图片素材的响应示例（URL 字段为 imageUrl），
// 视频/音频素材的字段名未在文档中说明。这里按
// url -> imageUrl -> videoUrl -> audioUrl -> fileUrl 依次尝试，取第一个非空值，
// 以免视频素材回填出空 URL。M0 联调需实测确认真实字段名。
func parseUpstreamAsset(raw map[string]any) AssetFields {
	f := AssetFields{}
	f.OfficialId = stringField(raw, "officialId", "official_id")
	f.GroupId = stringField(raw, "groupId", "group_id", "groupOfficialId")
	f.Name = stringField(raw, "name")
	f.Status = stringField(raw, "status")
	f.Region = strings.ToLower(stringField(raw, "region"))
	f.Url = stringField(raw, "url", "imageUrl", "videoUrl", "audioUrl", "fileUrl")
	f.AssetType = normalizeAssetType(stringField(raw, "assetType", "asset_type", "type"))
	f.FailReason = stringField(raw, "failReason", "fail_reason", "reason", "error")

	switch v := raw["id"].(type) {
	case float64:
		f.UpstreamId = int64(v)
	case int64:
		f.UpstreamId = v
	case int:
		f.UpstreamId = int64(v)
	}

	if f.AssetType == "" && f.Url != "" {
		f.AssetType = model.GuessAssetType(f.Url)
	}
	return f
}

func stringField(raw map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := raw[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func normalizeAssetType(t string) string {
	switch strings.ToLower(t) {
	case "image", "picture", "img":
		return model.AssetTypeImage
	case "video":
		return model.AssetTypeVideo
	case "audio":
		return model.AssetTypeAudio
	default:
		return ""
	}
}

// applyUpstreamFields 把上游字段合并进本地记录，只覆盖非空值，
// 避免上游某次返回缺字段时把本地已有信息清空。
func applyUpstreamFields(asset *model.Asset, f AssetFields) {
	if f.GroupId != "" {
		asset.GroupOfficialId = f.GroupId
	}
	if f.Name != "" {
		asset.Name = f.Name
	}
	if f.Status != "" {
		asset.Status = f.Status
	}
	if f.Region != "" {
		asset.Region = f.Region
	}
	if f.Url != "" {
		asset.SourceUrl = f.Url
	}
	if f.AssetType != "" {
		asset.AssetType = f.AssetType
	}
	if f.UpstreamId != 0 {
		asset.UpstreamId = f.UpstreamId
	}
	if f.FailReason != "" {
		asset.FailReason = f.FailReason
	}
}

// RecordUploadedAsset 在上游创建成功后写入归属记录。
//
// fields 是 provider 已经归一化过的字段；fallback 补齐上游没有回显的部分
// （典型如 Stelloria 的创建接口只回 assetId 与 assetName）。
func RecordUploadedAsset(userId, tokenId, channelId int, provider ProviderName, fields AssetFields, fallback AssetFields) (*model.Asset, error) {
	f := fields
	if f.OfficialId == "" {
		f.OfficialId = fallback.OfficialId
	}
	if f.OfficialId == "" {
		return nil, errors.New("upstream response has no asset id")
	}
	if f.GroupId == "" {
		f.GroupId = fallback.GroupId
	}
	if f.Name == "" {
		f.Name = fallback.Name
	}
	if f.Url == "" {
		f.Url = fallback.Url
	}
	if f.AssetType == "" {
		f.AssetType = fallback.AssetType
	}
	if f.AssetType == "" && f.Url != "" {
		f.AssetType = model.GuessAssetType(f.Url)
	}
	if f.Status == "" {
		f.Status = model.AssetStatusProcessing
	}
	if f.Region == "" {
		f.Region = fallback.Region
	}

	asset := &model.Asset{
		OfficialId: f.OfficialId,
		UserId:     userId,
		TokenId:    tokenId,
		ChannelId:  channelId,
		Provider:   provider,
		Status:     f.Status,
	}
	applyUpstreamFields(asset, f)
	if len(f.Raw) > 0 {
		asset.UpstreamRaw = f.Raw
	}
	if err := asset.Insert(); err != nil {
		return nil, err
	}
	return asset, nil
}

// ApplyUpstreamAssetFields 把一次上游查询结果合并进本地记录并落库。
func ApplyUpstreamAssetFields(asset *model.Asset, fields AssetFields) error {
	applyUpstreamFields(asset, fields)
	if len(fields.Raw) > 0 {
		asset.UpstreamRaw = fields.Raw
	}
	return asset.Update()
}

// SyncAssetFromUpstream 调用上游单查接口，回填 / 刷新本地记录。
//
// 这同时承担两件事，且它们本来就是同一个动作：
//  1. 审核状态轮询；
//  2. 创建接口未回显字段的回填（seegen 的 Excel 批量路径、Stelloria 的创建响应都只回 ID）。
func SyncAssetFromUpstream(ctx context.Context, channel *model.Channel, asset *model.Asset) error {
	provider := GetAssetsProvider(channel)
	fields, aErr := provider.GetAsset(ctx, channel, asset.OfficialId)
	if aErr != nil {
		// 素材在上游已被删除时，本地标记为已删除而不是反复重试
		if aErr.StatusCode == http.StatusNotFound {
			return asset.SoftDelete()
		}
		return aErr
	}
	return ApplyUpstreamAssetFields(asset, fields)
}

// SyncPendingAssets 批量同步用户名下待处理的素材（Processing 或字段未回填）。
// 返回实际同步成功的条数。失败不中断，尽最大努力。
func SyncPendingAssets(ctx context.Context, channel *model.Channel, userId int, limit int) int {
	assets, err := model.GetAssetsNeedSync(userId, limit)
	if err != nil || len(assets) == 0 {
		return 0
	}
	synced := 0
	for _, asset := range assets {
		if err := SyncAssetFromUpstream(ctx, channel, asset); err == nil {
			synced++
		}
	}
	return synced
}

// ValidateAssetSourceURL 对用户提交的素材 URL 做基础校验。
// 素材由上游去拉取，new-api 不主动请求该 URL，因此这里不做完整 SSRF 校验，
// 只拒绝明显非法的 scheme。
func ValidateAssetSourceURL(rawUrl string) error {
	rawUrl = strings.TrimSpace(rawUrl)
	if rawUrl == "" {
		return errors.New("url is required")
	}
	parsed, err := url.Parse(rawUrl)
	if err != nil {
		return fmt.Errorf("invalid url: %s", rawUrl)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q, only http and https are allowed", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("invalid url: %s", rawUrl)
	}
	return nil
}
