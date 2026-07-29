package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func ptr(s string) *string { return &s }

// TestNormalizeAssetStatus 锁定状态归一化。
//
// 这不是理论问题：Stelloria 文档里同一个 status 字段既写成 "ACTIVE"（列表、更新接口）
// 又写成 "Active"（详情接口）。若大小写敏感，素材永远不会被判定为可用，
// AssetRefCheck 会把所有引用都拦成 asset_not_active。
func TestNormalizeAssetStatus(t *testing.T) {
	cases := map[string]string{
		"Active":     model.AssetStatusActive,
		"ACTIVE":     model.AssetStatusActive,
		"active":     model.AssetStatusActive,
		"succeeded":  model.AssetStatusActive,
		"Failed":     model.AssetStatusFailed,
		"FAILED":     model.AssetStatusFailed,
		"rejected":   model.AssetStatusFailed,
		"Processing": model.AssetStatusProcessing,
		"PENDING":    model.AssetStatusProcessing,
		"reviewing":  model.AssetStatusProcessing,
		"":           "",
	}
	for input, want := range cases {
		if got := normalizeAssetStatus(input); got != want {
			t.Errorf("normalizeAssetStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestParseStelloriaAsset 用文档里的真实响应验证字段映射。
func TestParseStelloriaAsset(t *testing.T) {
	data := []byte(`{
    "id": "asset-xyz789",
    "assetId": "asset-xyz789",
    "groupId": "group-abc123",
    "assetName": "产品图片",
    "assetType": "Image",
    "assetUrl": "https://example.com/image.jpg",
    "status": "Active",
    "errorMessage": null,
    "createdTime": "2026-01-15 10:30:00"
  }`)

	f := parseStelloriaAsset(data)
	if f.OfficialId != "asset-xyz789" {
		t.Errorf("OfficialId = %q, want asset-xyz789", f.OfficialId)
	}
	if f.GroupId != "group-abc123" {
		t.Errorf("GroupId = %q, want group-abc123", f.GroupId)
	}
	if f.Name != "产品图片" {
		t.Errorf("Name = %q, want 产品图片", f.Name)
	}
	if f.Url != "https://example.com/image.jpg" {
		t.Errorf("Url = %q", f.Url)
	}
	if f.AssetType != model.AssetTypeImage {
		t.Errorf("AssetType = %q, want Image", f.AssetType)
	}
	if f.Status != model.AssetStatusActive {
		t.Errorf("Status = %q, want Active", f.Status)
	}
	// errorMessage 为 null 时不应被解析成字符串 "null"
	if f.FailReason != "" {
		t.Errorf("FailReason = %q, want empty", f.FailReason)
	}
}

// TestParseStelloriaAssetUppercaseStatus 覆盖列表接口返回的大写状态。
func TestParseStelloriaAssetUppercaseStatus(t *testing.T) {
	data := []byte(`{"assetId":"asset-1","assetName":"x","assetType":"Video","assetUrl":"https://a/b.mp4","status":"ACTIVE"}`)
	f := parseStelloriaAsset(data)
	if f.Status != model.AssetStatusActive {
		t.Errorf("Status = %q, want Active (大写 ACTIVE 必须能识别)", f.Status)
	}
	if f.AssetType != model.AssetTypeVideo {
		t.Errorf("AssetType = %q, want Video", f.AssetType)
	}
}

// TestProviderResolutionByBaseURL 验证 auto 模式下按 base_url 探测上游。
func TestProviderResolutionByBaseURL(t *testing.T) {
	cases := map[string]ProviderName{
		"https://api.seegen.ai":  ProviderSeegen,
		"https://stelloria.link": ProviderStelloria,
		// 识别不出来时回落到 seegen（最早接入、字段最全）
		"https://example.com": ProviderSeegen,
	}
	for baseURL, want := range cases {
		ch := &model.Channel{BaseURL: ptr(baseURL)}
		if got := GetAssetsProvider(ch).Name(); got != want {
			t.Errorf("GetAssetsProvider(%q) = %q, want %q", baseURL, got, want)
		}
	}
}

// TestProviderCapabilitiesDiffer 锁定两家上游的能力差异。
// 控制器与前端都依赖这张表做降级，改动时必须同步评估影响面。
func TestProviderCapabilitiesDiffer(t *testing.T) {
	seegen := (&seegenAssetsProvider{}).Capabilities()
	stelloria := (&stelloriaAssetsProvider{}).Capabilities()

	if !seegen.Regions {
		t.Error("seegen 应当有 cn/intl 区域概念")
	}
	if stelloria.Regions {
		t.Error("Stelloria 没有区域概念，Regions 必须为 false，否则区域校验会误伤全部请求")
	}
	if !seegen.ExcelTemplate {
		t.Error("seegen 支持 Excel 模板")
	}
	if stelloria.ExcelTemplate {
		t.Error("Stelloria 没有 Excel 上传接口")
	}
	// 上游没有原生批量接口，但 new-api 会退化成循环单条，对下游仍然可用
	if !stelloria.BatchCreate {
		t.Error("Stelloria 应通过循环单条创建对下游提供批量能力")
	}
	if len(stelloria.GroupTypes) == 0 {
		t.Error("Stelloria 应声明 groupType 枚举")
	}
	if len(seegen.GroupTypes) != 0 {
		t.Error("seegen 没有 groupType 概念")
	}
}

// TestFileNameFromURL 覆盖 Stelloria assetName 必填时的兜底取值。
func TestFileNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a/b/product.jpg": "product.jpg",
		"https://example.com/product.png":     "product.png",
		"https://example.com/":                "asset",
		"https://example.com":                 "asset",
	}
	for input, want := range cases {
		if got := fileNameFromURL(input); got != want {
			t.Errorf("fileNameFromURL(%q) = %q, want %q", input, got, want)
		}
	}
}
