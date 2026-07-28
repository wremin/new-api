package middleware

import (
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestExtractAssetRefs(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "纯 URL 请求不应匹配到任何素材引用",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.jpg"}}]}`,
			want: nil,
		},
		{
			name: "首尾帧两个素材",
			body: `{"content":[{"image_url":{"url":"asset://asset-2026abc"},"role":"first_frame"},{"image_url":{"url":"asset://asset-2026xyz"},"role":"last_frame"}]}`,
			want: []string{"asset-2026abc", "asset-2026xyz"},
		},
		{
			name: "重复引用应去重",
			body: `{"a":"asset://asset-1","b":"asset://asset-1","c":"asset://asset-2"}`,
			want: []string{"asset-1", "asset-2"},
		},
		{
			name: "嵌套在 metadata 深处也应被扫描到",
			body: `{"model":"m","metadata":{"content":[{"video_url":{"url":"asset://asset-deep"}}]}}`,
			want: []string{"asset-deep"},
		},
		{
			name: "空请求体",
			body: ``,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractAssetRefs([]byte(tc.body))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractAssetRefs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetSeedanceModelRegion(t *testing.T) {
	cases := map[string]string{
		"doubao-seedance-2-0-260128":        constant.SeedanceRegionCN,
		"doubao-seedance-2-0-mini-260615":   constant.SeedanceRegionCN,
		"doubao-seedance-2-0-fast-260128":   constant.SeedanceRegionCN,
		"dreamina-seedance-2-0-260128":      constant.SeedanceRegionINTL,
		"dreamina-seedance-2-0-mini-260615": constant.SeedanceRegionINTL,
		"ep-20260414121243-hp7w5":           constant.SeedanceRegionINTL,
		"ep-20260414121306-pk5j6":           constant.SeedanceRegionINTL,
		// 未知模型返回空串，调用方应跳过区域校验而非判定失败
		"gpt-4o":                         "",
		"doubao-seedance-1-0-pro-250528": "",
	}

	for modelName, want := range cases {
		if got := constant.GetSeedanceModelRegion(modelName); got != want {
			t.Errorf("GetSeedanceModelRegion(%q) = %q, want %q", modelName, got, want)
		}
	}
}
