// Package aliyunsign 实现阿里云 ACS3-HMAC-SHA256（V3）请求签名。
//
// 与 common/volcsign（火山引擎）最容易混淆、也最容易写错的一点：
//
//	火山：kDate → kRegion → kService → kSigning 四级密钥派生，再签
//	阿里云 V3：**没有派生链**，直接用 SecretKey 对 StringToSign 做一次 HMAC-SHA256
//
// 照着火山那套写必然 403 SignatureNotMatch。
package aliyunsign

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Algorithm 是签名算法标识，同时用于 Authorization 头前缀与 StringToSign 首行。
const Algorithm = "ACS3-HMAC-SHA256"

// Credentials 是签名所需的 RAM 密钥对。
type Credentials struct {
	AccessKeyID     string
	AccessKeySecret string
}

// ParseAKSK 解析 "AK|SK" 形式的渠道密钥。
//
// 沿用仓库既有的云厂商密钥书写约定（见 relay/channel/jimeng/sign.go）：
// 渠道 Key 字段用 "|" 分隔 AK 与 SK。
//
// 额外容错全角竖线 "｜"（U+FF5C）：中文输入法下敲竖线默认出的就是它，
// 而它与半角 "|"（U+007C）在界面上几乎无法分辨 —— 报错信息里印的是半角，
// 用户对照着看只会觉得自己填对了。AK/SK 本身是字母数字，
// 不可能合法包含任一种竖线，所以这个替换不会误伤。
func ParseAKSK(apiKey string) (Credentials, error) {
	apiKey = strings.ReplaceAll(apiKey, "｜", "|")
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return Credentials{}, errors.New("invalid api key format: expected 'AccessKeyId|AccessKeySecret'")
	}
	id := strings.TrimSpace(parts[0])
	secret := strings.TrimSpace(parts[1])
	if id == "" || secret == "" {
		return Credentials{}, errors.New("invalid api key format: access key id or secret is empty")
	}
	return Credentials{AccessKeyID: id, AccessKeySecret: secret}, nil
}

// Options 描述一次调用的动作与版本。
type Options struct {
	// Action 如 SubmitVideoGenerationJob
	Action string
	// Version 如 2026-07-07
	Version string
	// Now 可注入固定时间以便单测；零值表示使用 time.Now()。
	Now time.Time
	// Nonce 可注入固定随机串以便单测；为空时自动生成。
	Nonce string
}

// Sign 就地给 req 补齐 V3 签名相关的请求头：
// host、x-acs-action、x-acs-version、x-acs-date、x-acs-signature-nonce、
// x-acs-content-sha256，以及 Authorization。
//
// body 必须是即将发送的完整请求体（可为 nil）。调用方需保证 req.Body 与之一致，
// 否则上游会判定签名不匹配。
//
// 查询参数从 req.URL 读取，因此调用前必须先把业务参数拼进 URL。
func Sign(req *http.Request, creds Credentials, body []byte, opts Options) error {
	if req == nil || req.URL == nil {
		return errors.New("aliyunsign: request or request url is nil")
	}
	if opts.Action == "" || opts.Version == "" {
		return errors.New("aliyunsign: action and version are required")
	}

	sum := sha256.Sum256(body)
	hashedPayload := hex.EncodeToString(sum[:])

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	// 阿里云要求 ISO8601 UTC，秒级精度
	xAcsDate := now.UTC().Format("2006-01-02T15:04:05Z")

	nonce := opts.Nonce
	if nonce == "" {
		var err error
		if nonce, err = randomNonce(); err != nil {
			return fmt.Errorf("aliyunsign: generate nonce failed: %w", err)
		}
	}

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	req.Header.Set("host", host)
	req.Header.Set("x-acs-action", opts.Action)
	req.Header.Set("x-acs-version", opts.Version)
	req.Header.Set("x-acs-date", xAcsDate)
	req.Header.Set("x-acs-signature-nonce", nonce)
	req.Header.Set("x-acs-content-sha256", hashedPayload)

	// 参与签名的头：host + 全部 x-acs-*。
	// 其余头（含 Content-Type）不纳入，避免中间层改写 Content-Type 导致签名失效。
	signedNames := []string{"host"}
	for name := range req.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-acs-") {
			signedNames = append(signedNames, lower)
		}
	}
	sort.Strings(signedNames)

	var canonicalHeaders strings.Builder
	for _, name := range signedNames {
		value := host
		if name != "host" {
			value = req.Header.Get(name)
		}
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(value))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(signedNames, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		CanonicalQueryString(req.URL.Query()),
		canonicalHeaders.String(),
		signedHeaders,
		hashedPayload,
	}, "\n")

	hashedCanonical := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := Algorithm + "\n" + hex.EncodeToString(hashedCanonical[:])

	// 关键：直接用 SecretKey 签一次，没有火山那样的多级密钥派生。
	signature := hex.EncodeToString(hmacSHA256([]byte(creds.AccessKeySecret), []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s,SignedHeaders=%s,Signature=%s",
		Algorithm, creds.AccessKeyID, signedHeaders, signature,
	))
	return nil
}

// canonicalURI 返回规范化路径，空路径按 "/" 处理。
func canonicalURI(u *url.URL) string {
	if u.Path == "" {
		return "/"
	}
	return u.Path
}

// CanonicalQueryString 按 key 排序拼接查询串，使用阿里云要求的百分号编码。
//
// 导出是为了让适配器在拼 URL 时能复用同一套编码规则 —— 两处编码不一致
// 是签名失败最隐蔽的原因之一。
func CanonicalQueryString(query url.Values) string {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		values := append([]string(nil), query[k]...)
		sort.Strings(values)
		for _, v := range values {
			parts = append(parts, PercentEncode(k)+"="+PercentEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

// PercentEncode 按阿里云要求做 RFC3986 编码。
//
// 与 url.QueryEscape 的三处差异，缺一不可：
// 空格必须是 %20 而不是 +；星号必须编码成 %2A；波浪号必须保持原样。
func PercentEncode(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

// randomNonce 生成 32 位十六进制随机串，用于 x-acs-signature-nonce 防重放。
func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
