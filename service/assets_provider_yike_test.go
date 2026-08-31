package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

// TestYikeStatusUsesThirdPartyAssetStatus 这条是本文件最重要的断言。
//
// 素材能否被 Wonder 模型引用，看的是 ThirdPartyAssetStatus，**不是** Status。
// 拿 Status 判定会让素材过早变成 Active，生成任务带着不可用素材提交，
// 白扣一次额度，而失败信息与素材状态毫无关联，极难排查。
func TestYikeStatusUsesThirdPartyAssetStatus(t *testing.T) {
	// Status 已 Success，但第三方登记还在跑 → 必须仍是 Processing
	f := parseYikeMedia([]byte(`{"Media":{
		"MediaId":"media-1",
		"Status":"Success",
		"ThirdPartyAssetStatus":"Processing"
	}}`))
	if f.Status != model.AssetStatusProcessing {
		t.Errorf("ThirdPartyAssetStatus=Processing 时应为 Processing，实际 %q", f.Status)
	}

	// 两者都 Success → Active
	f = parseYikeMedia([]byte(`{"Media":{
		"MediaId":"media-1",
		"Status":"Success",
		"ThirdPartyAssetStatus":"Success"
	}}`))
	if f.Status != model.AssetStatusActive {
		t.Errorf("ThirdPartyAssetStatus=Success 时应为 Active，实际 %q", f.Status)
	}

	// 第三方登记失败 → Failed，即便 Status 是 Success
	f = parseYikeMedia([]byte(`{"Media":{
		"MediaId":"media-1",
		"Status":"Success",
		"ThirdPartyAssetStatus":"Failed",
		"ThirdPartyAssetErrorMessage":"format not supported"
	}}`))
	if f.Status != model.AssetStatusFailed {
		t.Errorf("ThirdPartyAssetStatus=Failed 时应为 Failed，实际 %q", f.Status)
	}
	if f.FailReason != "format not supported" {
		t.Errorf("FailReason = %q", f.FailReason)
	}
}

// TestYikeMissingThirdPartyStatusIsProcessing 字段缺失时必须保守判为处理中。
// 缺字段就判 Active 会让不可用的素材被提交，代价是用户的额度。
func TestYikeMissingThirdPartyStatusIsProcessing(t *testing.T) {
	f := parseYikeMedia([]byte(`{"Media":{"MediaId":"media-1","Status":"Success"}}`))
	if f.Status != model.AssetStatusProcessing {
		t.Errorf("缺 ThirdPartyAssetStatus 时应为 Processing，实际 %q", f.Status)
	}
}

// TestYikeParseMediaBothShapes 包装与扁平两种响应形态都要能解。
func TestYikeParseMediaBothShapes(t *testing.T) {
	wrapped := parseYikeMedia([]byte(`{"RequestId":"r","Media":{"MediaId":"m1","ThirdPartyAssetStatus":"Success"}}`))
	if wrapped.OfficialId != "m1" || wrapped.Status != model.AssetStatusActive {
		t.Errorf("包装形态解析失败: %+v", wrapped)
	}

	flat := parseYikeMedia([]byte(`{"MediaId":"m2","ThirdPartyAssetStatus":"Success"}`))
	if flat.OfficialId != "m2" || flat.Status != model.AssetStatusActive {
		t.Errorf("扁平形态解析失败: %+v", flat)
	}

	if got := parseYikeMedia([]byte(`not json`)); got.OfficialId != "" {
		t.Errorf("非 JSON 应当返回零值，实际 %+v", got)
	}
}

// TestYikeMediaType 归一化到上游要的三种取值。
func TestYikeMediaType(t *testing.T) {
	cases := []struct {
		assetType string
		url       string
		want      string
	}{
		{"Image", "", "image"},
		{"Video", "", "video"},
		{"Audio", "", "audio"},
		{"video/mp4", "", "video"},
		// assetType 为空时按 URL 推断
		{"", "https://x.com/a.mp4", "video"},
		{"", "https://x.com/a.jpg", "image"},
		// 认不出来时回落到 image —— 参考素材以图片为绝对多数
		{"", "", "image"},
	}
	for _, tc := range cases {
		if got := yikeMediaType(tc.assetType, tc.url); got != tc.want {
			t.Errorf("yikeMediaType(%q,%q) = %q, want %q", tc.assetType, tc.url, got, tc.want)
		}
	}
}

// TestYikeGroupIDFallback 上游没有分组，空组 ID 要补成合成的默认组，
// 否则 RelayAssetCreate 的非空校验会挡下整条素材链路。
func TestYikeGroupIDFallback(t *testing.T) {
	if got := yikeGroupID(""); got != yikeDefaultGroupID {
		t.Errorf("空组 ID 应补成 %q，实际 %q", yikeDefaultGroupID, got)
	}
	if got := yikeGroupID("   "); got != yikeDefaultGroupID {
		t.Errorf("空白组 ID 应补成 %q，实际 %q", yikeDefaultGroupID, got)
	}
	if got := yikeGroupID("custom"); got != "custom" {
		t.Errorf("已有组 ID 不应被改写，实际 %q", got)
	}
}

// TestYikeErrorStatusMapsNotFound NotFound 类错误必须映射成 404，
// 否则 SyncAssetFromUpstream 无法软删除上游已消失的素材。
func TestYikeErrorStatusMapsNotFound(t *testing.T) {
	if got := yikeErrorStatus(400, "MediaNotFound"); got != 404 {
		t.Errorf("MediaNotFound → %d, want 404", got)
	}
	if got := yikeErrorStatus(400, "ResourceNotExist"); got != 404 {
		t.Errorf("ResourceNotExist → %d, want 404", got)
	}
	if got := yikeErrorStatus(403, "SignatureNotMatch"); got != 403 {
		t.Errorf("SignatureNotMatch → %d, want 403", got)
	}
	if got := yikeErrorStatus(200, "Weird"); got != 502 {
		t.Errorf("2xx 配业务错误应映射成 502，实际 %d", got)
	}
}

// TestYikeCapabilities 能力声明要与实现一致，
// 前端据此隐藏入口，声明错了会给用户一个点了必然失败的按钮。
func TestYikeCapabilities(t *testing.T) {
	caps := (&yikeAssetsProvider{}).Capabilities()
	if caps.ExcelTemplate {
		t.Error("上游没有 Excel 模板，不应声明支持")
	}
	if caps.Regions {
		t.Error("上游没有区域概念，不应声明支持")
	}
	if len(caps.GroupTypes) != 0 {
		t.Error("上游没有素材组类型，应留空")
	}
	if !caps.BatchCreate {
		t.Error("批量由 fallbackBatchCreate 兜底，对下游可用，应声明支持")
	}
}
