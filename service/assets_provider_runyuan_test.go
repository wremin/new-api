package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSignRunyuanRequestGolden 用 Python 参考实现（与润元文档 2.1 节一致）算出的
// 金向向量锁定签名算法。任何一步（canonical query 排序、header 拼装、密钥派生、
// SK 不做 Base64 解码）出错都会导致签名不同，上游直接 403 SignatureNotMatch。
//
// 金向量生成参数：
//
//	AK=AK_TEST_1234567890  SK=SK_TEST_abcdefgh
//	host=runy.yitd.cn  x-date=20260817T120000Z
//	body={"AssetType":"Image","GroupId":"group-1","Name":"test.png","URL":"https://example.com/a.jpg"}
//	（Go json.Marshal 对 map key 按字典序输出，金向量按此字节序计算）
func TestSignRunyuanRequestGolden(t *testing.T) {
	body := []byte(`{"AssetType":"Image","GroupId":"group-1","Name":"test.png","URL":"https://example.com/a.jpg"}`)

	req := httptest.NewRequest(http.MethodPost,
		"https://runy.yitd.cn/v1/video?Action=CreateAsset&Version=2024-01-01", nil)

	fixed := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	signRunyuanRequestAt(req, "runy.yitd.cn", body, "AK_TEST_1234567890", "SK_TEST_abcdefgh", fixed)

	if got := req.Header.Get("X-Date"); got != "20260817T120000Z" {
		t.Errorf("X-Date = %q, want 20260817T120000Z", got)
	}
	wantHash := "205aee7ae7267f6be6600152f790dbdcc4482ff4cef074e9bb022c981aab8d3f"
	if got := req.Header.Get("X-Content-Sha256"); got != wantHash {
		t.Errorf("X-Content-Sha256 = %q, want %q", got, wantHash)
	}

	wantAuth := "HMAC-SHA256 Credential=AK_TEST_1234567890/20260817/cn-beijing/ark/request, " +
		"SignedHeaders=content-type;host;x-content-sha256;x-date, " +
		"Signature=da88877fc8b14272a4c6241ca15847ab70450bb4fb01d25fc093891e1ac3c0ed"
	if got := req.Header.Get("Authorization"); got != wantAuth {
		t.Errorf("Authorization = %q, want %q", got, wantAuth)
	}
	if req.Host != "runy.yitd.cn" {
		t.Errorf("req.Host = %q, want runy.yitd.cn", req.Host)
	}
}

// TestParseRunyuanAsset 用润元文档 2.4.2 的真实响应验证字段映射。
// 润元的字段名是大写开头（Id / Name / URL / AssetType / GroupId / Status），
// 与 seegen / Stelloria 的小写风格不同，映射错了会落库空字段。
func TestParseRunyuanAsset(t *testing.T) {
	data := []byte(`{
		"Id": "asset-20260701120000-hdbmf",
		"Name": "用户头像",
		"URL": "https://example.com/avatar.jpg",
		"AssetType": "Image",
		"GroupId": "group-20260701120000-8196",
		"Status": "Active",
		"Moderation": {"Strategy": "Default"},
		"ProjectName": "default",
		"CreateTime": "2026-07-01T00:00:00Z",
		"UpdateTime": "2026-07-01T00:00:00Z"
	}`)

	f := parseRunyuanAsset(data)
	if f.OfficialId != "asset-20260701120000-hdbmf" {
		t.Errorf("OfficialId = %q, want asset-20260701120000-hdbmf", f.OfficialId)
	}
	if f.GroupId != "group-20260701120000-8196" {
		t.Errorf("GroupId = %q, want group-20260701120000-8196", f.GroupId)
	}
	if f.Name != "用户头像" {
		t.Errorf("Name = %q, want 用户头像", f.Name)
	}
	if f.Url != "https://example.com/avatar.jpg" {
		t.Errorf("Url = %q", f.Url)
	}
	if f.AssetType != "Image" {
		t.Errorf("AssetType = %q, want Image", f.AssetType)
	}
	if f.Status != "Active" {
		t.Errorf("Status = %q, want Active", f.Status)
	}
}

// TestParseRunyuanCreateAssetResult 创建接口只回显 AssetId。
func TestParseRunyuanCreateAssetResult(t *testing.T) {
	data := []byte(`{"AssetId": "asset-20260701120000-hdbmf"}`)
	f := parseRunyuanAsset(data)
	if f.OfficialId != "asset-20260701120000-hdbmf" {
		t.Errorf("OfficialId = %q, want asset-20260701120000-hdbmf", f.OfficialId)
	}
}

// TestRunyuanErrorStatus 锁定业务错误码到 HTTP 状态的映射。
//
// 润元的业务错误走 HTTP 200 + ResponseMetadata.Error，直接透传 200 会让
// SyncAssetFromUpstream 把「素材不存在」当成同步成功，本地脏数据永远清不掉。
func TestRunyuanErrorStatus(t *testing.T) {
	cases := []struct {
		httpStatus int
		code       string
		want       int
	}{
		{http.StatusOK, "InvalidParameter", http.StatusBadRequest},
		{http.StatusOK, "AssetNotFound", http.StatusNotFound},
		{http.StatusOK, "ResourceNotFound", http.StatusNotFound},
		{http.StatusForbidden, "SignatureNotMatch", http.StatusForbidden},
		{http.StatusOK, "InternalError", http.StatusBadGateway},
	}
	for _, tc := range cases {
		if got := runyuanErrorStatus(tc.httpStatus, tc.code); got != tc.want {
			t.Errorf("runyuanErrorStatus(%d, %q) = %d, want %d", tc.httpStatus, tc.code, got, tc.want)
		}
	}
}

// TestRunyuanCapabilities 锁定能力声明：
// 没有区域概念（否则 AssetRefCheck 会误伤全部请求）、没有 Excel、
// GroupTypes 只声明 AIGC（LivenessFace 组由真人认证流程产生，CreateAssetGroup 建不出来）。
func TestRunyuanCapabilities(t *testing.T) {
	caps := (&runyuanAssetsProvider{}).Capabilities()
	if caps.Regions {
		t.Error("润元没有区域概念，Regions 必须为 false")
	}
	if caps.ExcelTemplate {
		t.Error("润元没有 Excel 模板接口")
	}
	if !caps.BatchCreate {
		t.Error("润元应通过循环单条创建对下游提供批量能力")
	}
	if len(caps.GroupTypes) != 1 || caps.GroupTypes[0] != RunyuanGroupTypeAIGC {
		t.Errorf("GroupTypes = %v, want [AIGC]", caps.GroupTypes)
	}
}
