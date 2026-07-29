package service

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// seegenAssetsProvider 对接 seegen.ai 的素材库。
//
// 协议特点：RESTful，路径 /v1/assets*，响应为裸对象（无 code/data 包装），
// 素材与素材组都用 officialId 标识，素材组带 region（cn / intl）。
type seegenAssetsProvider struct{}

func (p *seegenAssetsProvider) Name() ProviderName { return ProviderSeegen }

func (p *seegenAssetsProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		BatchCreate:   true,
		ExcelTemplate: true,
		Regions:       true,
		GroupTypes:    nil,
		RenameAsset:   false,
		DeleteGroup:   false,
	}
}

func (p *seegenAssetsProvider) CreateAsset(ctx context.Context, ch *model.Channel, in CreateAssetInput) (AssetFields, *AssetsError) {
	body, err := common.Marshal(map[string]any{
		"groupId": in.GroupId,
		"url":     in.Url,
		"name":    in.Name,
	})
	if err != nil {
		return AssetFields{}, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request body: "+err.Error(), http.StatusInternalServerError)
	}

	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets",
		Body:        body,
		ContentType: "application/json",
	})
	if aErr != nil {
		return AssetFields{}, aErr
	}
	if !resp.IsSuccess() {
		return AssetFields{}, NewAssetsError(AssetErrUpstream,
			ExtractUpstreamAssetError(resp.Body), resp.StatusCode)
	}
	return p.parseAsset(resp.Body), nil
}

func (p *seegenAssetsProvider) GetAsset(ctx context.Context, ch *model.Channel, officialId string) (AssetFields, *AssetsError) {
	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method: http.MethodGet,
		Path:   "/v1/assets/" + url.PathEscape(officialId),
	})
	if aErr != nil {
		return AssetFields{}, aErr
	}
	if !resp.IsSuccess() {
		return AssetFields{}, NewAssetsError(AssetErrUpstream,
			ExtractUpstreamAssetError(resp.Body), resp.StatusCode)
	}
	return p.parseAsset(resp.Body), nil
}

func (p *seegenAssetsProvider) DeleteAsset(ctx context.Context, ch *model.Channel, officialId string) *AssetsError {
	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method: http.MethodDelete,
		Path:   "/v1/assets/" + url.PathEscape(officialId),
	})
	if aErr != nil {
		return aErr
	}
	// 上游已不存在时视为删除成功，本地照常清理
	if !resp.IsSuccess() && resp.StatusCode != http.StatusNotFound {
		return NewAssetsError(AssetErrUpstream, ExtractUpstreamAssetError(resp.Body), resp.StatusCode)
	}
	return nil
}

func (p *seegenAssetsProvider) BatchCreateAssets(ctx context.Context, ch *model.Channel, ins []CreateAssetInput) (string, []BatchItemResult, *AssetsError) {
	items := make([]map[string]any, 0, len(ins))
	for _, in := range ins {
		items = append(items, map[string]any{
			"groupId": in.GroupId,
			"url":     in.Url,
			"name":    in.Name,
		})
	}
	body, err := common.Marshal(items)
	if err != nil {
		return "", nil, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request body: "+err.Error(), http.StatusInternalServerError)
	}

	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets/batch",
		Body:        body,
		ContentType: "application/json",
	})
	if aErr != nil {
		return "", nil, aErr
	}
	if !resp.IsSuccess() {
		return "", nil, NewAssetsError(AssetErrUpstream,
			ExtractUpstreamAssetError(resp.Body), resp.StatusCode)
	}
	return parseSeegenBatchResponse(resp.Body)
}

func (p *seegenAssetsProvider) BatchCreateFromExcel(ctx context.Context, ch *model.Channel, contentType string, body []byte) (string, []BatchItemResult, *AssetsError) {
	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets/batch",
		RawBody:     bytes.NewReader(body),
		ContentType: contentType,
	})
	if aErr != nil {
		return "", nil, aErr
	}
	if !resp.IsSuccess() {
		return "", nil, NewAssetsError(AssetErrUpstream,
			ExtractUpstreamAssetError(resp.Body), resp.StatusCode)
	}
	return parseSeegenBatchResponse(resp.Body)
}

func (p *seegenAssetsProvider) ExcelTemplate(ctx context.Context, ch *model.Channel) (*http.Response, *AssetsError) {
	return StreamAssetsUpstream(ctx, ch, AssetsUpstreamRequest{
		Method: http.MethodGet,
		Path:   "/v1/assets/batch/template",
	})
}

func (p *seegenAssetsProvider) CreateGroup(ctx context.Context, ch *model.Channel, in CreateGroupInput) (GroupFields, *AssetsError) {
	payload := map[string]any{"name": in.Name}
	if in.Description != "" {
		payload["description"] = in.Description
	}
	if in.Region != "" {
		payload["region"] = in.Region
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return GroupFields{}, NewAssetsError(AssetErrInvalidRequest,
			"failed to build upstream request body: "+err.Error(), http.StatusInternalServerError)
	}

	resp, aErr := DoAssetsUpstreamRequest(ctx, ch, AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets/groups",
		Body:        body,
		ContentType: "application/json",
	})
	if aErr != nil {
		return GroupFields{}, aErr
	}
	if !resp.IsSuccess() {
		return GroupFields{}, NewAssetsError(AssetErrUpstream,
			ExtractUpstreamAssetError(resp.Body), resp.StatusCode)
	}

	var raw map[string]any
	_ = common.Unmarshal(resp.Body, &raw)
	g := GroupFields{
		OfficialId:  stringField(raw, "officialId", "official_id", "id"),
		Name:        stringField(raw, "name"),
		Description: stringField(raw, "description"),
		Region:      stringField(raw, "region"),
		Raw:         resp.Body,
	}
	if v, ok := raw["id"].(float64); ok {
		g.UpstreamId = int64(v)
	}
	if g.Name == "" {
		g.Name = in.Name
	}
	if g.Description == "" {
		g.Description = in.Description
	}
	if g.Region == "" {
		g.Region = in.Region
	}
	return g, nil
}

// parseAsset 把 seegen 的裸对象响应翻译成归一化字段。
func (p *seegenAssetsProvider) parseAsset(body []byte) AssetFields {
	var raw map[string]any
	_ = common.Unmarshal(body, &raw)
	f := parseUpstreamAsset(raw)
	f.Status = normalizeAssetStatus(f.Status)
	f.Raw = body
	return f
}

func parseSeegenBatchResponse(body []byte) (string, []BatchItemResult, *AssetsError) {
	var resp struct {
		BatchId string            `json:"batchId"`
		Total   int               `json:"total"`
		Results []BatchItemResult `json:"results"`
	}
	if err := common.Unmarshal(body, &resp); err != nil {
		return "", nil, NewAssetsError(AssetErrUpstream,
			"failed to parse batch response: "+err.Error(), http.StatusBadGateway)
	}
	return resp.BatchId, resp.Results, nil
}
