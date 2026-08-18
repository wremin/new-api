package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// runyuanAssetsProvider 对接润元（runy.yitd.cn）的素材库。
//
// 协议与 seegen / Stelloria 又完全不同：
//   - 鉴权不是 Bearer Token，而是火山引擎 AK/SK HMAC-SHA256 签名
//     （Service=ark，Region=cn-beijing，Version=2024-01-01）。
//     渠道的 Key 字段是视频生成任务（/v1/video/tasks）用的 Bearer Token，
//     AK/SK 挂在渠道 OtherSettings 的 runyuan_assets_ak / runyuan_assets_sk。
//   - 路径是 RPC 风格：所有接口都是 POST /v1/video?Action=<Action>&Version=2024-01-01
//   - 响应统一包 {ResponseMetadata:{...}, Result:{...}}，
//     业务错误走 HTTP 200 + ResponseMetadata.Error（Code/Message），
//     只有鉴权失败才回 HTTP 403。
//   - 素材组有 groupType（AIGC / LivenessFace），没有 region。
//     CreateAssetGroup 实际只支持 AIGC，LivenessFace 组由真人认证流程产生，
//     因此 GroupTypes 只声明 AIGC，避免客户端以为能直接建 LivenessFace 组。
//   - 没有批量接口与 Excel 模板，批量退化为循环单条。
type runyuanAssetsProvider struct{}

// 润元签名常量，来自润元接口文档 2.1 节。
const (
	runyuanSignService = "ark"
	runyuanSignRegion  = "cn-beijing"
	runyuanAPIVersion  = "2024-01-01"
	runyuanVideoPath   = "/v1/video"
)

func (p *runyuanAssetsProvider) Name() ProviderName { return ProviderRunyuan }

func (p *runyuanAssetsProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		// 上游没有批量接口，但 new-api 会退化成循环单条创建，对下游仍然可用
		BatchCreate:   true,
		ExcelTemplate: false,
		Regions:       false,
		GroupTypes:    []string{RunyuanGroupTypeAIGC},
		RenameAsset:   true,
		DeleteGroup:   true,
	}
}

// Runyuan 素材组类型。CreateAssetGroup 仅支持 AIGC（传其他值上游按 AIGC 处理），
// LivenessFace 组由真人认证流程（CreateVisualValidateSession）产生，不走这里。
const RunyuanGroupTypeAIGC = "AIGC"

// runyuanEnvelope 是润元的统一响应包装。
// 业务错误也回 HTTP 200，必须看 ResponseMetadata.Error 才能判定成败。
type runyuanEnvelope struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	Result json.RawMessage `json:"Result"`
}

// runyuanCredentials 从渠道 OtherSettings 取 AK/SK。
func runyuanCredentials(ch *model.Channel) (string, string, *AssetsError) {
	settings := ch.GetOtherSettings()
	ak := strings.TrimSpace(settings.RunyuanAssetsAK)
	sk := strings.TrimSpace(settings.RunyuanAssetsSK)
	if ak == "" || sk == "" {
		return "", "", NewAssetsError(AssetErrChannelUnavailable,
			"runyuan assets AK/SK is not configured on this channel "+
				"(set runyuan_assets_ak / runyuan_assets_sk in channel other settings)",
			http.StatusServiceUnavailable)
	}
	return ak, sk, nil
}

// callRunyuan 签名并调用一次 /v1/video?Action=<action>，返回 Result 的原始 JSON。
func callRunyuan(ctx context.Context, ch *model.Channel, action string, payload any) ([]byte, *AssetsError) {
	ak, sk, aErr := runyuanCredentials(ch)
	if aErr != nil {
		return nil, aErr
	}

	var body []byte
	if payload != nil {
		marshaled, err := common.Marshal(payload)
		if err != nil {
			return nil, NewAssetsError(AssetErrInvalidRequest,
				"failed to build upstream request body: "+err.Error(), http.StatusInternalServerError)
		}
		body = marshaled
	}

	baseURL := strings.TrimSuffix(ch.GetBaseURL(), "/")
	if baseURL == "" {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"assets channel base url is empty", http.StatusServiceUnavailable)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return nil, NewAssetsError(AssetErrChannelUnavailable,
			"assets channel base url is invalid", http.StatusServiceUnavailable)
	}

	query := url.Values{}
	query.Set("Action", action)
	query.Set("Version", runyuanAPIVersion)

	fullURL := baseURL + runyuanVideoPath + "?" + query.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, assetsUpstreamTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request: "+err.Error(), http.StatusInternalServerError)
	}

	signRunyuanRequest(httpReq, parsed.Host, body, ak, sk)

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
	defer func() {
		_ = resp.Body.Close()
	}()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to read upstream response: "+err.Error(), http.StatusBadGateway)
	}

	var env runyuanEnvelope
	if uErr := common.Unmarshal(respBody, &env); uErr != nil {
		return nil, NewAssetsError(AssetErrUpstream,
			"failed to parse upstream response: "+uErr.Error(), http.StatusBadGateway)
	}

	// HTTP 403 是签名/账户/IP 白名单问题；HTTP 200 + Error 是业务错误。
	// 两者都在 ResponseMetadata.Error 里，统一按错误码映射状态。
	if env.ResponseMetadata.Error != nil {
		upErr := env.ResponseMetadata.Error
		return nil, NewAssetsError(AssetErrUpstream,
			ScrubUpstreamText(fmt.Sprintf("%s: %s", upErr.Code, upErr.Message)),
			runyuanErrorStatus(resp.StatusCode, upErr.Code))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewAssetsError(AssetErrUpstream,
			ExtractUpstreamAssetError(respBody), resp.StatusCode)
	}
	return env.Result, nil
}

// runyuanErrorStatus 把润元的错误码映射成对下游有意义的 HTTP 状态码。
//
// 业务错误统一走 HTTP 200 + Error.Code，不能直接透传 200——
// 否则 SyncAssetFromUpstream 会把「素材不存在」当成同步成功。
// 含 NotFound 的错误码映射成 404，让本地记录能被正确软删除。
func runyuanErrorStatus(httpStatus int, code string) int {
	if httpStatus == http.StatusForbidden {
		return http.StatusForbidden
	}
	if strings.Contains(code, "NotFound") {
		return http.StatusNotFound
	}
	if code == "InvalidParameter" {
		return http.StatusBadRequest
	}
	if httpStatus >= 400 {
		return httpStatus
	}
	return http.StatusBadGateway
}

// signRunyuanRequest 按火山引擎 HMAC-SHA256 规则给请求签名。
//
// 步骤（润元文档 2.1）：
//  1. body SHA-256 -> X-Content-Sha256
//  2. CanonicalRequest = method\npath\nsortedQuery\ncanonicalHeaders\nsignedHeaders\nbodyHash
//  3. StringToSign = HMAC-SHA256\nxDate\ncredentialScope\nhash(CanonicalRequest)
//  4. kDate/kRegion/kService/kSigning 逐层派生（SK 按原始字符串参与，不做 Base64 解码）
//  5. Signature = hex(HMAC(kSigning, StringToSign))
func signRunyuanRequest(req *http.Request, host string, body []byte, ak, sk string) {
	signRunyuanRequestAt(req, host, body, ak, sk, time.Now().UTC())
}

// signRunyuanRequestAt 把时间显式传入，供单测用固定时间复现签名向量。
func signRunyuanRequestAt(req *http.Request, host string, body []byte, ak, sk string, now time.Time) {
	xDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	bodyHashBytes := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodyHashBytes[:])

	signedHeaders := "content-type;host;x-content-sha256;x-date"
	canonicalHeaders := "content-type:application/json\n" +
		"host:" + host + "\n" +
		"x-content-sha256:" + bodyHash + "\n" +
		"x-date:" + xDate + "\n"

	// req.URL.Query() 已按 key 排序编码，与文档的规范化查询字符串一致
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.Path,
		req.URL.Query().Encode(),
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")
	crHashBytes := sha256.Sum256([]byte(canonicalRequest))
	crHash := hex.EncodeToString(crHashBytes[:])

	credentialScope := dateStamp + "/" + runyuanSignRegion + "/" + runyuanSignService + "/request"
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		xDate,
		credentialScope,
		crHash,
	}, "\n")

	kDate := hmacSHA256([]byte(sk), dateStamp)
	kRegion := hmacSHA256(kDate, runyuanSignRegion)
	kService := hmacSHA256(kRegion, runyuanSignService)
	kSigning := hmacSHA256(kService, "request")
	sigBytes := hmacSHA256(kSigning, stringToSign)
	signature := hex.EncodeToString(sigBytes)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", bodyHash)
	req.Header.Set("Authorization", "HMAC-SHA256 Credential="+ak+"/"+credentialScope+
		", SignedHeaders="+signedHeaders+", Signature="+signature)
	// Host 走 req.Host 而不是 Header，Go 的 http client 会用它构造请求行
	req.Host = host
}

func hmacSHA256(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}

// ============================
// AssetsProvider 接口实现
// ============================

func (p *runyuanAssetsProvider) CreateAsset(ctx context.Context, ch *model.Channel, in CreateAssetInput) (AssetFields, *AssetsError) {
	payload := map[string]any{
		"URL": in.Url,
	}
	if in.GroupId != "" {
		payload["GroupId"] = in.GroupId
	}
	if in.Name != "" {
		payload["Name"] = in.Name
	}
	if in.AssetType != "" {
		payload["AssetType"] = in.AssetType
	}

	data, aErr := callRunyuan(ctx, ch, "CreateAsset", payload)
	if aErr != nil {
		return AssetFields{}, aErr
	}

	f := parseRunyuanAsset(data)
	// 创建接口只回显 AssetId，其余字段用请求参数补齐
	if f.OfficialId == "" {
		var raw map[string]any
		_ = common.Unmarshal(data, &raw)
		f.OfficialId = stringField(raw, "AssetId", "assetId", "Id", "id")
	}
	if f.GroupId == "" {
		f.GroupId = in.GroupId
	}
	if f.Name == "" {
		f.Name = in.Name
	}
	if f.Url == "" {
		f.Url = in.Url
	}
	if f.AssetType == "" {
		f.AssetType = in.AssetType
	}
	if f.AssetType == "" {
		f.AssetType = model.GuessAssetType(in.Url)
	}
	if f.Status == "" {
		f.Status = model.AssetStatusProcessing
	}
	f.Raw = data
	return f, nil
}

func (p *runyuanAssetsProvider) GetAsset(ctx context.Context, ch *model.Channel, officialId string) (AssetFields, *AssetsError) {
	data, aErr := callRunyuan(ctx, ch, "GetAsset", map[string]any{"Id": officialId})
	if aErr != nil {
		return AssetFields{}, aErr
	}
	f := parseRunyuanAsset(data)
	f.Raw = data
	return f, nil
}

func (p *runyuanAssetsProvider) DeleteAsset(ctx context.Context, ch *model.Channel, officialId string) *AssetsError {
	_, aErr := callRunyuan(ctx, ch, "DeleteAsset", map[string]any{"Id": officialId})
	if aErr != nil {
		// 上游已不存在时视为删除成功，本地照常清理
		if aErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return aErr
	}
	return nil
}

// BatchCreateAssets 润元没有批量接口，循环单条创建。
func (p *runyuanAssetsProvider) BatchCreateAssets(ctx context.Context, ch *model.Channel, ins []CreateAssetInput) (string, []BatchItemResult, *AssetsError) {
	return fallbackBatchCreate(ctx, ch, ins, p.CreateAsset)
}

func (p *runyuanAssetsProvider) BatchCreateFromExcel(_ context.Context, _ *model.Channel, _ string, _ []byte) (string, []BatchItemResult, *AssetsError) {
	return "", nil, ErrCapabilityUnsupported(ProviderRunyuan, "excel batch upload")
}

func (p *runyuanAssetsProvider) ExcelTemplate(_ context.Context, _ *model.Channel) (*http.Response, *AssetsError) {
	return nil, ErrCapabilityUnsupported(ProviderRunyuan, "excel template download")
}

func (p *runyuanAssetsProvider) CreateGroup(ctx context.Context, ch *model.Channel, in CreateGroupInput) (GroupFields, *AssetsError) {
	groupType := in.GroupType
	if groupType == "" {
		groupType = RunyuanGroupTypeAIGC
	}
	payload := map[string]any{
		"Name":      in.Name,
		"GroupType": groupType,
	}
	if in.Description != "" {
		payload["Description"] = in.Description
	}

	data, aErr := callRunyuan(ctx, ch, "CreateAssetGroup", payload)
	if aErr != nil {
		return GroupFields{}, aErr
	}

	var raw map[string]any
	_ = common.Unmarshal(data, &raw)
	g := GroupFields{
		OfficialId:  stringField(raw, "Id", "id", "GroupId", "groupId"),
		Name:        stringField(raw, "Name", "name"),
		Description: stringField(raw, "Description", "description"),
		GroupType:   stringField(raw, "GroupType", "groupType"),
		Raw:         data,
	}
	// 创建接口只回显 Id，其余用请求参数补齐
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

// parseRunyuanAsset 把润元的素材对象翻译成归一化字段。
//
// 字段映射：Id→OfficialId、Name→Name、URL→Url、AssetType→AssetType、
// GroupId→GroupId、Status→Status（Processing / Active / Failed）。
// 注意润元的字段名是大写开头，与 seegen/Stelloria 的小写风格不同。
func parseRunyuanAsset(data []byte) AssetFields {
	var raw map[string]any
	if err := common.Unmarshal(data, &raw); err != nil {
		return AssetFields{}
	}
	f := AssetFields{
		OfficialId: stringField(raw, "Id", "id", "AssetId", "assetId"),
		GroupId:    stringField(raw, "GroupId", "groupId"),
		Name:       stringField(raw, "Name", "name"),
		Url:        stringField(raw, "URL", "Url", "url"),
		AssetType:  normalizeAssetType(stringField(raw, "AssetType", "assetType", "asset_type")),
		Status:     normalizeAssetStatus(stringField(raw, "Status", "status")),
		FailReason: stringField(raw, "FailReason", "failReason", "fail_reason", "Reason"),
	}
	if f.AssetType == "" && f.Url != "" {
		f.AssetType = model.GuessAssetType(f.Url)
	}
	return f
}
