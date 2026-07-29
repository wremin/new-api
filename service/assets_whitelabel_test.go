package service

import "testing"

// TestPublicProviderName 上游内部标识符不能直接暴露给下游。
//
// 内部名（stelloria）会落进 assets.provider 列并参与上游切换比对，不能改；
// 这里只做展示层映射。
func TestPublicProviderName(t *testing.T) {
	if got := PublicProviderName(ProviderStelloria); got != "metamind" {
		t.Errorf("PublicProviderName(stelloria) = %q, want metamind", got)
	}
	// 未配置别名的上游按原样返回
	if got := PublicProviderName(ProviderSeegen); got != ProviderSeegen {
		t.Errorf("PublicProviderName(seegen) = %q, want %q", got, ProviderSeegen)
	}
	if got := PublicProviderName(""); got != "" {
		t.Errorf("PublicProviderName(\"\") = %q, want empty", got)
	}
}

// TestScrubUpstreamText 上游报错常带自己的品牌与域名，透传前必须清洗。
func TestScrubUpstreamText(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"upstream error from stelloria": "upstream error from metamind",
		"Stelloria returned 502":        "Metamind returned 502",
		"STELLORIA IS DOWN":             "METAMIND IS DOWN",
		"failed to reach https://stelloria.link/v1": "failed to reach https://metamind.yun/v1",
		// 域名要先于裸词替换，不能出现 metamind.link 这种半截结果
		"stelloria.link timed out":           "metamind.yun timed out",
		"no upstream keyword here":           "no upstream keyword here",
		"mixed Stelloria and stelloria.link": "mixed Metamind and metamind.yun",
	}
	for input, want := range cases {
		if got := ScrubUpstreamText(input); got != want {
			t.Errorf("ScrubUpstreamText(%q) = %q, want %q", input, got, want)
		}
	}
}
