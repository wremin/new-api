package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// stelloriaAssetsProvider 对接星瞳 Stelloria 的 AICC 素材库。
//
// 协议特点（与 seegen 几乎处处不同）：
//   - 路径是 RPC 风格 /api/aicc/assets、/api/aicc/assets/query
//   - 查列表用 POST + JSON body（pageNo / pageSize），不是 GET + query 参数
//   - 所有响应都包一层 {code, message, data, traceId, timestamp}，code=0 才算成功
//   - 素材 ID 字段是 assetId，素材组是 groupId
//   - 没有 region，改用 groupType（AIGC / LivenessFace）
//   - 没有批量接口与 Excel 模板
//   - assetType 是必填项（seegen 可以不传）
type stelloriaAssetsProvider struct{}

// Stelloria 素材组类型
const (
	StelloriaGroupTypeAIGC          = "AIGC"
	StelloriaGroupTypeLivenessFace  = "LivenessFace"
	stelloriaDefaultGroupTypeForGen = StelloriaGroupTypeAIGC
)

func (p *stelloriaAssetsProvider) Name() ProviderName { return ProviderStelloria }

func (p *stelloriaAssetsProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		// 上游没有批量接口，但 new-api 会退化成循环单条创建，对下游仍然可用
		BatchCreate:   true,
		ExcelTemplate: false,
		Regions:       false,
		GroupTypes:    []string{StelloriaGroupTypeAIGC, StelloriaGroupTypeLivenessFace},
		RenameAsset:   true,
		DeleteGroup:   true,
	}
}

// stelloriaEnvelope 是 Stelloria 的统一响应包装。
type stelloriaEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TraceId string `json:"traceId"`
}

// callStelloria 发起一次调用并拆掉 {code,message,data} 包装，返回 data 的原始 JSON。
//
// 注意：HTTP 200 不代表成功，必须再看 body 里的 code——
// 只有 code == 0 才是成功，否则要把 message 透出给用户。
func callStelloria(ctx context.Context, ch *model.Channel, method, path string, payload any) ([]byte, *AssetsError) {
	var body []byte
	if payload != nil {
		marshaled, err := common.Marshal(payload)
		if err != nil {
			return nil, NewAssetsError(AssetErrInvalidRequest,
				"failed to build upstream request body: "+err.Error(), http.StatusInternalServerError)
		}
		body = marshaled
	}

	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method:      method,
		Path:        path,
		Body:        body,
		ContentType: "application/json",
	})
	if aErr != nil {
		return nil, aErr
	}

	if !resp.IsSuccess() {
		return nil, NewAssetsError(AssetErrUpstream,
			extractStelloriaError(resp.Body), resp.StatusCode)
	}

	var env stelloriaEnvelope
	if err := common.Unmarshal(resp.Body, &env); err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to parse upstream response: "+err.Error(), http.StatusBadGateway)
	}
	if env.Code != 0 {
		return nil, NewAssetsError(AssetErrUpstream,
			fmt.Sprintf("upstream error (code=%d): %s", env.Code, env.Message), http.StatusBadGateway)
	}

	// data 的形态在不同接口之间不一致（对象 / 数组 / 布尔），
	// 所以只把它原样切出来交给调用方按需解析。
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := common.Unmarshal(resp.Body, &wrapper); err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to parse upstream data: "+err.Error(), http.StatusBadGateway)
	}
	return wrapper.Data, nil
}

func extractStelloriaError(body []byte) string {
	var env stelloriaEnvelope
	if err := common.Unmarshal(body, &env); err == nil && env.Message != "" {
		// 上游报错常带自己的品牌与域名，透传前清洗掉
		return ScrubUpstreamText(env.Message)
	}
	return ExtractUpstreamAssetError(body)
}

func (p *stelloriaAssetsProvider) CreateAsset(ctx context.Context, ch *model.Channel, in CreateAssetInput) (AssetFields, *AssetsError) {
	assetType := in.AssetType
	if assetType == "" {
		assetType = model.GuessAssetType(in.Url)
	}
	if assetType == "" {
		// Stelloria 的 assetType 是必填项，推断不出来时按图片处理并在错误里给出提示
		assetType = model.AssetTypeImage
	}
	name := in.Name
	if name == "" {
		// assetName 也是必填项，用 URL 的文件名兜底
		name = fileNameFromURL(in.Url)
	}

	data, aErr := callStelloria(ctx, ch, http.MethodPost, "/api/aicc/assets", map[string]any{
		"groupId":   in.GroupId,
		"assetName": name,
		"assetUrl":  in.Url,
		"assetType": assetType,
	})
	if aErr != nil {
		return AssetFields{}, aErr
	}

	f := parseStelloriaAsset(data)
	// 创建接口只回显 assetId 与 assetName，其余字段用请求参数补齐
	if f.GroupId == "" {
		f.GroupId = in.GroupId
	}
	if f.Url == "" {
		f.Url = in.Url
	}
	if f.AssetType == "" {
		f.AssetType = assetType
	}
	if f.Name == "" {
		f.Name = name
	}
	if f.Status == "" {
		f.Status = model.AssetStatusProcessing
	}
	f.Raw = data
	return f, nil
}

func (p *stelloriaAssetsProvider) GetAsset(ctx context.Context, ch *model.Channel, officialId string) (AssetFields, *AssetsError) {
	data, aErr := callStelloria(ctx, ch, http.MethodGet,
		"/api/aicc/assets/"+url.PathEscape(officialId), nil)
	if aErr != nil {
		return AssetFields{}, aErr
	}
	f := parseStelloriaAsset(data)
	f.Raw = data
	return f, nil
}

func (p *stelloriaAssetsProvider) DeleteAsset(ctx context.Context, ch *model.Channel, officialId string) *AssetsError {
	_, aErr := callStelloria(ctx, ch, http.MethodDelete,
		"/api/aicc/assets/"+url.PathEscape(officialId), nil)
	if aErr != nil {
		// 上游已不存在时视为删除成功，本地照常清理
		if aErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return aErr
	}
	return nil
}

// BatchCreateAssets Stelloria 没有批量接口，循环单条创建。
func (p *stelloriaAssetsProvider) BatchCreateAssets(ctx context.Context, ch *model.Channel, ins []CreateAssetInput) (string, []BatchItemResult, *AssetsError) {
	return fallbackBatchCreate(ctx, ch, ins, p.CreateAsset)
}

func (p *stelloriaAssetsProvider) BatchCreateFromExcel(_ context.Context, _ *model.Channel, _ string, _ []byte) (string, []BatchItemResult, *AssetsError) {
	return "", nil, ErrCapabilityUnsupported(ProviderStelloria, "excel batch upload")
}

func (p *stelloriaAssetsProvider) ExcelTemplate(_ context.Context, _ *model.Channel) (*http.Response, *AssetsError) {
	return nil, ErrCapabilityUnsupported(ProviderStelloria, "excel template download")
}

func (p *stelloriaAssetsProvider) CreateGroup(ctx context.Context, ch *model.Channel, in CreateGroupInput) (GroupFields, *AssetsError) {
	groupType := in.GroupType
	if groupType == "" {
		groupType = stelloriaDefaultGroupTypeForGen
	}
	payload := map[string]any{
		"groupType": groupType,
		"groupName": in.Name,
	}
	if in.Description != "" {
		payload["description"] = in.Description
	}

	data, aErr := callStelloria(ctx, ch, http.MethodPost, "/api/aicc/asset-groups", payload)
	if aErr != nil {
		return GroupFields{}, aErr
	}

	var raw map[string]any
	_ = common.Unmarshal(data, &raw)
	g := GroupFields{
		OfficialId:  stringField(raw, "groupId", "id"),
		Name:        stringField(raw, "groupName", "name"),
		Description: stringField(raw, "description"),
		GroupType:   stringField(raw, "groupType"),
		Raw:         data,
	}
	// 创建接口只回显 groupId，其余用请求参数补齐
	if g.Name == "" {
		g.Name = in.Name
	}
	if g.Description == "" {
		g.Description = in.Description
	}
	if g.GroupType == "" {
		g.GroupType = groupType
	}
	return g, nil
}

// parseStelloriaAsset 把 Stelloria 的素材对象翻译成归一化字段。
//
// 字段映射：assetId→OfficialId、assetName→Name、assetUrl→Url、
// assetType→AssetType、errorMessage→FailReason。
// status 大小写在文档里不一致（ACTIVE / Active），统一走 normalizeAssetStatus。
func parseStelloriaAsset(data []byte) AssetFields {
	var raw map[string]any
	if err := common.Unmarshal(data, &raw); err != nil {
		return AssetFields{}
	}
	f := AssetFields{
		OfficialId: stringField(raw, "assetId", "id"),
		GroupId:    stringField(raw, "groupId"),
		Name:       stringField(raw, "assetName", "name"),
		Url:        stringField(raw, "assetUrl", "url"),
		AssetType:  normalizeAssetType(stringField(raw, "assetType", "type")),
		Status:     normalizeAssetStatus(stringField(raw, "status")),
		FailReason: stringField(raw, "errorMessage", "error_message", "message"),
	}
	if f.AssetType == "" && f.Url != "" {
		f.AssetType = model.GuessAssetType(f.Url)
	}
	return f
}

// fileNameFromURL 从 URL 中取出文件名，用作 assetName 的兜底值。
func fileNameFromURL(rawUrl string) string {
	parsed, err := url.Parse(rawUrl)
	if err != nil {
		return "asset"
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	name := segments[len(segments)-1]
	if name == "" {
		return "asset"
	}
	return name
}
