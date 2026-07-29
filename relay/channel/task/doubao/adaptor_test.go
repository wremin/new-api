package doubao

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

// TestBuildUpstreamPath 锁定三种上游的路径拼接。
// seegen 的路径前缀是 /v1 而不是火山原生的 /api/v3，
// 配错会表现为提交任务时上游 404。
func TestBuildUpstreamPath(t *testing.T) {
	cases := []struct {
		baseURL    string
		wantSubmit string
		wantFetch  string
	}{
		{
			baseURL:    "https://api.seegen.ai",
			wantSubmit: "https://api.seegen.ai/v1/contents/generations/tasks",
			wantFetch:  "https://api.seegen.ai/v1/contents/generations/tasks/cgt-123",
		},
		{
			baseURL:    "https://ai.kkidc.com",
			wantSubmit: "https://ai.kkidc.com/v1/video/generations",
			wantFetch:  "https://ai.kkidc.com/v1/video/generations/cgt-123",
		},
		{
			baseURL:    "https://ark.cn-beijing.volces.com",
			wantSubmit: "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks",
			wantFetch:  "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks/cgt-123",
		},
	}

	for _, tc := range cases {
		a := &TaskAdaptor{baseURL: tc.baseURL}
		got, err := a.BuildRequestURL(nil)
		if err != nil {
			t.Fatalf("BuildRequestURL(%s) error: %v", tc.baseURL, err)
		}
		if got != tc.wantSubmit {
			t.Errorf("BuildRequestURL(%s) = %q, want %q", tc.baseURL, got, tc.wantSubmit)
		}
	}
}

// TestExtractVideoURL 覆盖各上游的 content 形态。
//
// 这是一个真实踩过的坑：seegen 的 content 是数组，
// 而适配器原先声明为固定对象结构，导致整个响应 unmarshal 失败——
// 表现为任务提交成功但永远轮询不出结果。
func TestExtractVideoURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "seegen 数组形态",
			raw:  `[{"type":"video_url","video_url":{"url":"https://cdn.example.com/a.mp4"}}]`,
			want: "https://cdn.example.com/a.mp4",
		},
		{
			name: "火山原生对象形态",
			raw:  `{"video_url":"https://cdn.example.com/b.mp4"}`,
			want: "https://cdn.example.com/b.mp4",
		},
		{
			name: "对象嵌套形态",
			raw:  `{"video_url":{"url":"https://cdn.example.com/c.mp4"}}`,
			want: "https://cdn.example.com/c.mp4",
		},
		{
			name: "数组里混有非视频元素",
			raw:  `[{"type":"text","text":"x"},{"type":"video_url","video_url":{"url":"https://cdn.example.com/d.mp4"}}]`,
			want: "https://cdn.example.com/d.mp4",
		},
		{
			name: "任务未完成时 content 为空",
			raw:  ``,
			want: "",
		},
		{
			name: "空数组",
			raw:  `[]`,
			want: "",
		},
		{
			name: "无法识别的形态不应 panic",
			raw:  `"just a string"`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractVideoURL(json.RawMessage(tc.raw)); got != tc.want {
				t.Errorf("extractVideoURL(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestParseSeegenTaskResult 用 seegen 文档里的真实响应做端到端解析验证。
func TestParseSeegenTaskResult(t *testing.T) {
	body := []byte(`{
  "id": "cgt-2026abc123xyz",
  "status": "succeeded",
  "model": "doubao-seedance-2-0-260128",
  "content": [
    {
      "type": "video_url",
      "video_url": { "url": "https://cdn.example.com/output/video.mp4"}
    }
  ],
  "ratio": "16:9",
  "duration": 8,
  "usage": { "completion_tokens": 28672, "total_tokens": 28772 }
}`)

	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Errorf("status = %v, want %v", info.Status, model.TaskStatusSuccess)
	}
	if info.Url != "https://cdn.example.com/output/video.mp4" {
		t.Errorf("url = %q, want the video url", info.Url)
	}
	if info.TotalTokens != 28772 {
		t.Errorf("total_tokens = %d, want 28772", info.TotalTokens)
	}
}

// TestParseArkTaskResultStillWorks 确认新增 seegen 兼容后，火山原生格式没有回归。
func TestParseArkTaskResultStillWorks(t *testing.T) {
	body := []byte(`{
  "id": "cgt-ark-1",
  "status": "succeeded",
  "content": { "video_url": "https://ark.example.com/out.mp4" },
  "usage": { "completion_tokens": 100, "total_tokens": 120 }
}`)

	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Errorf("status = %v, want %v", info.Status, model.TaskStatusSuccess)
	}
	if info.Url != "https://ark.example.com/out.mp4" {
		t.Errorf("url = %q, want the ark video url", info.Url)
	}
}
