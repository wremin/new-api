package doubao

import (
	"bytes"
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

// TestParseStelloriaTaskResult 用**线上实测**的响应验证解析。
//
// 注意这份报文与官方文档不一致，以实测为准：
// 视频地址在顶层 result_url，而 result 里装的是上游火山 Ark 的原始响应
// （content.video_url）。文档写的 result.video_url 根本不存在——
// 按文档写会得到「任务成功但 URL 为空」。
func TestParseStelloriaTaskResult(t *testing.T) {
	const videoURL = "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/doubao-seedance-2-0/0217853119587090.mp4?X-Tos-Expires=86400"
	completed := []byte(`{
  "task_id": "task-d2fc5e66915049cb9ba2",
  "model": "moma-seedance-2.0",
  "type": "video",
  "status": "completed",
  "submitted_at": "2026-07-29T15:59:19",
  "completed_at": "2026-07-29T16:03:05",
  "result_url": "` + videoURL + `",
  "result": {
    "id": "cgt-20260729155918-g2cc5",
    "model": "doubao-seedance-2-0-260128",
    "status": "succeeded",
    "content": { "video_url": "` + videoURL + `" },
    "usage": { "completion_tokens": 108900, "total_tokens": 108900 },
    "resolution": "720p", "ratio": "16:9", "duration": 5
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
	if info.Url != videoURL {
		t.Errorf("url = %q, want %q", info.Url, videoURL)
	}
	if info.TotalTokens != 108900 {
		t.Errorf("total_tokens = %d, want 108900（result.usage 应被提取）", info.TotalTokens)
	}

	// 只有嵌套 content 没有顶层 result_url 时也要能取到
	nestedOnly := []byte(`{"task_id":"t1","status":"completed","result":{"content":{"video_url":"https://x/a.mp4"}}}`)
	info, err = a.ParseTaskResult(nestedOnly)
	if err != nil {
		t.Fatalf("ParseTaskResult(nested) error: %v", err)
	}
	if info.Url != "https://x/a.mp4" {
		t.Errorf("嵌套形态未取到 url，实际 %q", info.Url)
	}

	// 文档描述的 result.video_url 形态也要兼容（万一上游改回去）
	documented := []byte(`{"task_id":"t2","status":"completed","result":{"video_url":"https://x/b.mp4"}}`)
	info, _ = a.ParseTaskResult(documented)
	if info.Url != "https://x/b.mp4" {
		t.Errorf("文档形态未取到 url，实际 %q", info.Url)
	}

	processing := []byte(`{"task_id":"task-abc","status":"processing","estimated_time":120}`)
	info, err = a.ParseTaskResult(processing)
	if err != nil {
		t.Fatalf("ParseTaskResult(processing) error: %v", err)
	}
	if info.Status != model.TaskStatusInProgress {
		t.Errorf("processing 应映射为 InProgress，实际 %v", info.Status)
	}
}

// TestStelloriaErrorShapes 失败时 error 既可能是字符串也可能是对象，
// 声明成固定类型会让整个响应解析失败，必须两种都认。
func TestStelloriaErrorShapes(t *testing.T) {
	a := &TaskAdaptor{}
	cases := map[string]string{
		`{"task_id":"t","status":"failed","error":"content rejected"}`:                     "content rejected",
		`{"task_id":"t","status":"failed","error":{"message":"nsfw detected"}}`:            "nsfw detected",
		`{"task_id":"t","status":"failed","result":{"error":{"message":"upstream down"}}}`: "upstream down",
	}
	for body, want := range cases {
		info, err := a.ParseTaskResult([]byte(body))
		if err != nil {
			t.Fatalf("ParseTaskResult(%s) error: %v", body, err)
		}
		if info.Status != model.TaskStatusFailure {
			t.Errorf("%s: status = %v, want Failure", body, info.Status)
		}
		if info.Reason != want {
			t.Errorf("%s: reason = %q, want %q", body, info.Reason, want)
		}
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
			// 客户端常按 seedance 原生格式同时带 prompt 和 content，
			// 但 Stelloria 收到 content 会回 502，必须丢弃
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "一只金毛犬在海边奔跑"},
			},
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
	// 实测：带上 content 时 Stelloria 会回 502 "upstream returned empty task id"，
	// 所以即便下游传了也必须丢弃。这里用序列化结果断言，防止字段被重新加回来。
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload error: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"content"`)) {
		t.Errorf("请求体不能包含 content 字段，实际: %s", encoded)
	}
}
