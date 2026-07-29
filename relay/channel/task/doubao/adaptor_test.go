package doubao

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

// TestStelloriaUpstreamPath 锁定 Stelloria 的路径。
// 它与前三家都不同：提交是复数 videos，查询换到独立的 /v1/tasks 前缀。
func TestStelloriaUpstreamPath(t *testing.T) {
	a := &TaskAdaptor{baseURL: "https://stelloria.link"}
	got, err := a.BuildRequestURL(nil)
	if err != nil {
		t.Fatalf("BuildRequestURL error: %v", err)
	}
	want := "https://stelloria.link/v1/videos/generations"
	if got != want {
		t.Errorf("BuildRequestURL = %q, want %q", got, want)
	}
	// 不能落到火山原生分支
	if got == "https://stelloria.link/api/v3/contents/generations/tasks" {
		t.Error("Stelloria 掉进了火山原生分支，会 404")
	}
}

// TestParseStelloriaTaskResult 用文档里的真实响应验证解析。
//
// 与 seegen 那个坑同源：Stelloria 的视频地址在 result.video_url，
// 状态是 completed 而不是 succeeded，两者都不匹配已有分支，
// 不单独处理会返回 "unknown format"，任务永远轮询不出结果。
func TestParseStelloriaTaskResult(t *testing.T) {
	completed := []byte(`{
  "task_id": "task-abc123def456",
  "status": "completed",
  "model": "seedance-2.0",
  "result": {
    "video_url": "https://cdn.example.com/video/output.mp4",
    "duration": "5s",
    "resolution": "1080p",
    "cover_url": "https://cdn.example.com/video/cover.jpg"
  }
}`)

	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(completed)
	if err != nil {
		t.Fatalf("ParseTaskResult error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Errorf("status = %v, want %v", info.Status, model.TaskStatusSuccess)
	}
	if info.Url != "https://cdn.example.com/video/output.mp4" {
		t.Errorf("url = %q, want the video url", info.Url)
	}

	processing := []byte(`{"task_id":"task-abc","status":"processing","model":"seedance-2.0"}`)
	info, err = a.ParseTaskResult(processing)
	if err != nil {
		t.Fatalf("ParseTaskResult(processing) error: %v", err)
	}
	if info.Status != model.TaskStatusInProgress {
		t.Errorf("processing 应映射为 InProgress，实际 %v", info.Status)
	}

	failed := []byte(`{"task_id":"task-abc","status":"failed","error":"content rejected"}`)
	info, err = a.ParseTaskResult(failed)
	if err != nil {
		t.Fatalf("ParseTaskResult(failed) error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Errorf("failed 应映射为 Failure，实际 %v", info.Status)
	}
	if info.Reason != "content rejected" {
		t.Errorf("reason = %q, want the upstream error", info.Reason)
	}
}

// TestStelloriaPayloadShape 验证扁平请求体的字段转换。
// 重点：duration 必须是 "5s" 这样的字符串，宽高比字段叫 aspect_ratio。
func TestStelloriaPayloadShape(t *testing.T) {
	a := &TaskAdaptor{baseURL: "https://stelloria.link"}
	req := relaycommon.TaskSubmitReq{
		Model:  "moma-seedance-2.0",
		Prompt: "一只金毛犬在海边奔跑",
		Image:  "asset://asset-abc123",
		Metadata: map[string]interface{}{
			"duration":   float64(10),
			"ratio":      "16:9",
			"resolution": "1080p",
		},
	}

	p := a.convertToStelloriaPayload(&req)
	if p.Duration != "10s" {
		t.Errorf("duration = %q, want \"10s\"（上游要字符串不是整数）", p.Duration)
	}
	if p.AspectRatio != "16:9" {
		t.Errorf("aspect_ratio = %q, want 16:9", p.AspectRatio)
	}
	if p.Resolution != "1080p" {
		t.Errorf("resolution = %q", p.Resolution)
	}
	if p.ImageURL != "asset://asset-abc123" {
		t.Errorf("image_url = %q, 素材引用应原样透传", p.ImageURL)
	}
	if p.Prompt != "一只金毛犬在海边奔跑" {
		t.Errorf("prompt = %q", p.Prompt)
	}
	// 下游没传 content 时不应塞这个未文档化的字段
	if len(p.Content) != 0 {
		t.Errorf("下游未传 content 时不应自行添加，实际 %d 项", len(p.Content))
	}
}
