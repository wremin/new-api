package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

// realGetMediaResponse 是 2026-08-31 从生产抓到的**真实** GetMedia 响应，
// 只把签名地址与用户 ID 做了脱敏，结构一字未改。
//
// 之前这里放的是按参考文档臆造的形态（扁平的 ThirdPartyAssetStatus），
// 结果测试全绿而线上素材永远停在 Processing —— 拿臆造的样例当夹具，
// 测的只是"我的假设自洽"，不是"我的解析对"。
const realGetMediaResponse = `{
  "MediaInfo": {
    "MediaId": "f1005070a53771f1851ef6e7c5486601",
    "EntityId": "AiSaasMediaAsset",
    "FileInfoList": [
      {
        "FileBasicInfo": {
          "Width": "768",
          "Height": "454",
          "Region": "ap-southeast-1",
          "FileUrl": "https://ice-ai-saas-sg.oss-ap-southeast-1.aliyuncs.com/yiketemp/x/f100.png?Expires=1788183286&Signature=REDACTED",
          "FileName": "f1005070a53771f1851ef6e7c5486601.png",
          "FileSize": "431154",
          "FileType": "source_file",
          "FileStatus": "Normal",
          "FormatName": "png"
        }
      }
    ],
    "MediaBasicInfo": {
      "Title": "yike_ref_image",
      "Source": "url",
      "Status": "Normal",
      "MediaId": "f1005070a53771f1851ef6e7c5486601",
      "InputURL": "https://example.com/global_images/ref.png",
      "MediaType": "image",
      "BusinessType": "general"
    },
    "MediaDynamicInfo": {
      "MediaExtraInfo": { "AiAuditStatus": "Init", "ManualAuditStatus": "Init" },
      "DynamicMetaData": {
        "Data": "{\"MainUserId\":\"000\",\"MediaAssetSubType\":\"Image\",\"AuditStatus\":\"pass\",\"ThirdPartyAssetStatus\":\"Success\"}",
        "EntityId": "AiSaasMediaAsset"
      }
    }
  },
  "RequestId": "01A057D0-A3C1-30AE-89B6-791292378B64"
}`

// TestParseRealGetMediaResponse 用真实响应验证解析 —— 本文件最重要的测试。
//
// 三个当初全部猜错的点：
//  1. 包装层是 MediaInfo，业务字段又在 MediaBasicInfo 里，比预期多一层；
//  2. ThirdPartyAssetStatus 埋在 DynamicMetaData.Data 里，且那是**二次编码**的 JSON 字符串；
//  3. MediaId 是裸 32 位十六进制，不是文档写的 media-xxx。
func TestParseRealGetMediaResponse(t *testing.T) {
	f, found := parseYikeMedia([]byte(realGetMediaResponse))

	if !found {
		t.Fatal("没能从真实响应里读出 ThirdPartyAssetStatus —— 这正是线上素材卡在 Processing 的原因")
	}
	if f.Status != model.AssetStatusActive {
		t.Errorf("Status = %q, want Active（原始响应里 ThirdPartyAssetStatus 是 Success）", f.Status)
	}
	if f.OfficialId != "f1005070a53771f1851ef6e7c5486601" {
		t.Errorf("OfficialId = %q", f.OfficialId)
	}
	if f.Name != "yike_ref_image" {
		t.Errorf("Name = %q（应从 MediaBasicInfo.Title 取）", f.Name)
	}
	if f.AssetType != model.AssetTypeImage {
		t.Errorf("AssetType = %q（应从 MediaBasicInfo.MediaType 取）", f.AssetType)
	}
	// Url 取用户提交的源地址，不是 FileInfoList 里那个带签名的临时地址
	if !strings.Contains(f.Url, "example.com") {
		t.Errorf("Url = %q，应当是 InputURL 而不是临时签名地址", f.Url)
	}
	if strings.Contains(f.Url, "Signature=") {
		t.Errorf("Url 用了带签名的临时地址，它会过期：%s", f.Url)
	}
}

// TestParseYikeMediaStatusNotFound 找不到判据时必须报告 found=false。
//
// 状态同样是 Processing，但调用方要能区分"上游还在处理"和"我们没找到字段" ——
// 不能区分的话，上游一改结构就是静默地永远卡住。
func TestParseYikeMediaStatusNotFound(t *testing.T) {
	// 有完整结构但 Data 里没有那个键
	body := `{"MediaInfo":{"MediaId":"m1","MediaDynamicInfo":{"DynamicMetaData":{"Data":"{\"AuditStatus\":\"pass\"}"}}}}`
	f, found := parseYikeMedia([]byte(body))
	if found {
		t.Error("Data 里没有 ThirdPartyAssetStatus，found 应为 false")
	}
	if f.Status != model.AssetStatusProcessing {
		t.Errorf("找不到判据时应保守判为 Processing，实际 %q", f.Status)
	}

	// Data 不是合法 JSON
	body = `{"MediaInfo":{"MediaId":"m1","MediaDynamicInfo":{"DynamicMetaData":{"Data":"not json"}}}}`
	if _, found = parseYikeMedia([]byte(body)); found {
		t.Error("Data 解析失败时 found 应为 false")
	}

	// 整个响应不是 JSON
	if f, found = parseYikeMedia([]byte("nope")); found || f.OfficialId != "" {
		t.Error("非 JSON 应返回零值且 found=false")
	}
}

// TestParseYikeMediaFlatFallback 顶层扁平形态也要能解 ——
// 上游哪天扁平化了不至于整条链路失效。
func TestParseYikeMediaFlatFallback(t *testing.T) {
	body := `{"MediaId":"m2","Title":"t","MediaType":"video","ThirdPartyAssetStatus":"Success"}`
	f, found := parseYikeMedia([]byte(body))
	if !found || f.Status != model.AssetStatusActive {
		t.Errorf("扁平形态解析失败: %+v found=%v", f, found)
	}
	if f.OfficialId != "m2" || f.AssetType != model.AssetTypeVideo {
		t.Errorf("扁平形态字段不对: %+v", f)
	}
}

// TestParseYikeMediaFailed 第三方登记失败要落到 Failed，且带上原因。
func TestParseYikeMediaFailed(t *testing.T) {
	body := `{"MediaInfo":{
		"MediaId":"m3",
		"MediaBasicInfo":{"Status":"Normal","ThirdPartyAssetErrorMessage":"format not supported"},
		"MediaDynamicInfo":{"DynamicMetaData":{"Data":"{\"ThirdPartyAssetStatus\":\"Failed\"}"}}
	}}`
	f, found := parseYikeMedia([]byte(body))
	if !found || f.Status != model.AssetStatusFailed {
		t.Fatalf("应判为 Failed，实际 %q found=%v", f.Status, found)
	}
	if f.FailReason != "format not supported" {
		t.Errorf("FailReason = %q", f.FailReason)
	}
}

// TestParseYikeMediaIgnoresOwnStatus 上游自己的 Status=Normal 不能被当成可用判据。
//
// Normal 只说明入库正常，不代表 Wonder 能引用。拿它判定会让素材过早变 Active，
// 生成任务带着不可用素材提交，白扣一次额度。
func TestParseYikeMediaIgnoresOwnStatus(t *testing.T) {
	body := `{"MediaInfo":{
		"MediaId":"m4",
		"MediaBasicInfo":{"Status":"Normal"},
		"MediaDynamicInfo":{"DynamicMetaData":{"Data":"{\"ThirdPartyAssetStatus\":\"Processing\"}"}}
	}}`
	f, _ := parseYikeMedia([]byte(body))
	if f.Status == model.AssetStatusActive {
		t.Error("Status=Normal 不能让素材变成 Active")
	}
}

// TestYikeMediaType 归一化到上游要的三种取值。
func TestYikeMediaType(t *testing.T) {
	cases := []struct {
		assetType, url, want string
	}{
		{"Image", "", "image"},
		{"Video", "", "video"},
		{"Audio", "", "audio"},
		{"video/mp4", "", "video"},
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

// TestYikeGroupIDFallback 上游没有分组，空组 ID 要补成合成的默认组。
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

// TestYikeCapabilities 能力声明要与实现一致，前端据此隐藏入口。
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
