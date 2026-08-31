package aliyunsign

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseAKSK(t *testing.T) {
	creds, err := ParseAKSK(" LTAI5tXXXX | secretXXXX ")
	if err != nil {
		t.Fatalf("ParseAKSK returned error: %v", err)
	}
	if creds.AccessKeyID != "LTAI5tXXXX" || creds.AccessKeySecret != "secretXXXX" {
		t.Errorf("ParseAKSK = %+v", creds)
	}
	for _, bad := range []string{"", "onlyak", "ak|sk|extra", "|sk", "ak|"} {
		if _, err := ParseAKSK(bad); err == nil {
			t.Errorf("ParseAKSK(%q) should have failed", bad)
		}
	}
}

// TestPercentEncode 锁定阿里云与 url.QueryEscape 的三处差异。
// 任何一处不一致都会让 CanonicalQueryString 算错，进而 403。
func TestPercentEncode(t *testing.T) {
	cases := map[string]string{
		"a b":   "a%20b", // 空格是 %20，不是 +
		"a*b":   "a%2Ab", // 星号必须编码
		"a~b":   "a~b",   // 波浪号必须保持原样
		"a/b":   "a%2Fb",
		"中文":    "%E4%B8%AD%E6%96%87",
		"a=b&c": "a%3Db%26c",
	}
	for in, want := range cases {
		if got := PercentEncode(in); got != want {
			t.Errorf("PercentEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCanonicalQueryStringSorted 查询串必须按 key 排序，与书写顺序无关。
func TestCanonicalQueryStringSorted(t *testing.T) {
	a := CanonicalQueryString(url.Values{"B": {"2"}, "A": {"1"}, "C": {"3"}})
	b := CanonicalQueryString(url.Values{"C": {"3"}, "A": {"1"}, "B": {"2"}})
	if a != b {
		t.Errorf("查询串依赖了书写顺序：\n  %s\n  %s", a, b)
	}
	if want := "A=1&B=2&C=3"; a != want {
		t.Errorf("CanonicalQueryString = %q, want %q", a, want)
	}
}

func fixedOpts() Options {
	return Options{
		Action:  "SubmitVideoGenerationJob",
		Version: "2026-07-07",
		Now:     time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		Nonce:   "0123456789abcdef0123456789abcdef",
	}
}

// TestSignSetsRequiredHeaders 校验 V3 要求的头齐全且形态正确。
// 缺任何一个上游都会返回 403。
func TestSignSetsRequiredHeaders(t *testing.T) {
	body := []byte(`{"Prompt":"一只橘猫"}`)
	req, err := http.NewRequest(http.MethodPost,
		"https://yike.ap-southeast-1.aliyuncs.com/?Model=Wonder-Pro", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}

	if err := Sign(req, Credentials{AccessKeyID: "AK", AccessKeySecret: "SK"}, body, fixedOpts()); err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	checks := map[string]string{
		"x-acs-action":          "SubmitVideoGenerationJob",
		"x-acs-version":         "2026-07-07",
		"x-acs-date":            "2026-08-31T12:00:00Z",
		"x-acs-signature-nonce": "0123456789abcdef0123456789abcdef",
		"host":                  "yike.ap-southeast-1.aliyuncs.com",
	}
	for name, want := range checks {
		if got := req.Header.Get(name); got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}

	sum := sha256.Sum256(body)
	if got, want := req.Header.Get("x-acs-content-sha256"), hex.EncodeToString(sum[:]); got != want {
		t.Errorf("x-acs-content-sha256 = %q, want %q", got, want)
	}

	auth := req.Header.Get("Authorization")
	for _, want := range []string{
		"ACS3-HMAC-SHA256 ",
		"Credential=AK,",
		"SignedHeaders=host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version,",
		"Signature=",
	} {
		if !strings.Contains(auth, want) {
			t.Errorf("Authorization 缺少 %q\n实际: %s", want, auth)
		}
	}
}

// TestSignNoKeyDerivation 这条是本文件最重要的断言。
//
// 阿里云 V3 直接用 SecretKey 对 StringToSign 签一次，**没有**火山那样的
// kDate→kRegion→kService→kSigning 派生链。照火山写必然 403，且本地无从察觉。
// 这里用手工重算的方式把算法钉死。
func TestSignNoKeyDerivation(t *testing.T) {
	body := []byte(`{}`)
	req, _ := http.NewRequest(http.MethodPost, "https://yike.ap-southeast-1.aliyuncs.com/", nil)
	opts := fixedOpts()
	if err := Sign(req, Credentials{AccessKeyID: "AK", AccessKeySecret: "SK"}, body, opts); err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	// 按算法手工复算一遍
	payload := sha256.Sum256(body)
	hashedPayload := hex.EncodeToString(payload[:])
	signedHeaders := "host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version"
	canonicalHeaders := strings.Join([]string{
		"host:yike.ap-southeast-1.aliyuncs.com",
		"x-acs-action:" + opts.Action,
		"x-acs-content-sha256:" + hashedPayload,
		"x-acs-date:2026-08-31T12:00:00Z",
		"x-acs-signature-nonce:" + opts.Nonce,
		"x-acs-version:" + opts.Version,
	}, "\n") + "\n"
	canonicalRequest := strings.Join([]string{
		"POST", "/", "", canonicalHeaders, signedHeaders, hashedPayload,
	}, "\n")
	hashed := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := Algorithm + "\n" + hex.EncodeToString(hashed[:])
	want := hex.EncodeToString(hmacSHA256([]byte("SK"), []byte(stringToSign)))

	if !strings.Contains(req.Header.Get("Authorization"), "Signature="+want) {
		t.Errorf("签名与手工复算不一致\n  期望 Signature=%s\n  实际 %s", want, req.Header.Get("Authorization"))
	}
}

// TestSignIsDeterministic 固定输入必须得到固定签名，
// 否则说明签名过程混进了 map 迭代顺序之类的不确定因素。
func TestSignIsDeterministic(t *testing.T) {
	sign := func() string {
		req, _ := http.NewRequest(http.MethodPost,
			"https://yike.ap-southeast-1.aliyuncs.com/?B=2&A=1", nil)
		_ = Sign(req, Credentials{AccessKeyID: "AK", AccessKeySecret: "SK"}, []byte(`{"x":1}`), fixedOpts())
		return req.Header.Get("Authorization")
	}
	first := sign()
	for i := 0; i < 20; i++ {
		if sign() != first {
			t.Fatal("签名不确定")
		}
	}
}

// TestSignBodyAndQueryAffectSignature 请求体与查询参数都必须参与签名。
func TestSignBodyAndQueryAffectSignature(t *testing.T) {
	sign := func(rawURL, body string) string {
		req, _ := http.NewRequest(http.MethodPost, rawURL, nil)
		_ = Sign(req, Credentials{AccessKeyID: "AK", AccessKeySecret: "SK"}, []byte(body), fixedOpts())
		return req.Header.Get("Authorization")
	}
	base := "https://yike.ap-southeast-1.aliyuncs.com/?Model=Wonder-Pro"

	if sign(base, `{"a":1}`) == sign(base, `{"a":2}`) {
		t.Error("请求体变化没有反映到签名上")
	}
	if sign(base, `{"a":1}`) == sign(base+"&N=2", `{"a":1}`) {
		t.Error("查询参数变化没有反映到签名上")
	}
}

// TestSignRejectsMissingActionOrVersion Action / Version 必填，
// 缺了会算出上游认不出的签名。
func TestSignRejectsMissingActionOrVersion(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://yike.ap-southeast-1.aliyuncs.com/", nil)
	if err := Sign(req, Credentials{AccessKeyID: "AK", AccessKeySecret: "SK"}, nil, Options{}); err == nil {
		t.Error("Sign 应当拒绝空的 Action/Version")
	}
}
