package yike

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// TestResolveJobType 钉死 jobType 推导规则。
// 传错 jobType 上游直接拒，而错误信息不会指向"素材数量"这个真正的原因。
func TestResolveJobType(t *testing.T) {
	img := Media{Type: "image", MediaID: "media-1"}
	vid := Media{Type: "video", MediaID: "media-2"}
	aud := Media{Type: "audio", MediaID: "media-3"}

	cases := []struct {
		name   string
		medias []Media
		want   string
	}{
		{"无素材", nil, JobTypeText},
		{"单图", []Media{img}, JobTypeImage},
		{"两图首尾帧", []Media{img, img}, JobTypeFirstLastFrame},
		{"三图", []Media{img, img, img}, JobTypeReference},
		{"含视频", []Media{img, vid}, JobTypeReference},
		{"含音频", []Media{img, aud}, JobTypeReference},
		{"仅视频", []Media{vid}, JobTypeReference},
	}
	for _, tc := range cases {
		if got := resolveJobType(tc.medias); got != tc.want {
			t.Errorf("%s: resolveJobType = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestNormalizeMediaID 只接受已登记的 MediaId。
// 直链必须被挡掉 —— Wonder 系列不接受外部 URL，透传过去只会换来一个难懂的上游错误。
func TestNormalizeMediaID(t *testing.T) {
	cases := map[string]string{
		"media-abc":                   "media-abc",
		"asset://media-abc":           "media-abc",
		"  media-abc  ":               "media-abc",
		"https://example.com/a.jpg":   "",
		"HTTP://example.com/a.jpg":    "",
		"http://example.com/a.jpg":    "",
		"":                            "",
		"asset://https://x.com/a.mp4": "",
	}
	for in, want := range cases {
		if got := normalizeMediaID(in); got != want {
			t.Errorf("normalizeMediaID(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNormalizeResolution 三类写法都要归一化到上游认的大写 P 形态。
func TestNormalizeResolution(t *testing.T) {
	cases := map[string]string{
		"720p":      "720P",
		"720P":      "720P",
		" 1080p ":   "1080P",
		"1280x720":  "720P",
		"720x1280":  "720P",
		"1920x1080": "1080P",
		"1080x1920": "1080P",
		"":          "",
		// 认不出来的一律返回空串走上游默认值，绝不透传
		"4k":     "",
		"480p":   "",
		"abc":    "",
		"100x50": "",
	}
	for in, want := range cases {
		if got := normalizeResolution(in); got != want {
			t.Errorf("normalizeResolution(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExtractOutputURL Output 是 JSON 字符串，必须二次解析。
func TestExtractOutputURL(t *testing.T) {
	out := `{"Medias":[{"OutputUrl":"https://oss.example.com/a.mp4","MediaId":"m1","Type":"video"}]}`
	if got := ExtractOutputURL(out); got != "https://oss.example.com/a.mp4" {
		t.Errorf("ExtractOutputURL = %q", got)
	}
	for _, bad := range []string{"", "   ", "not json", `{"Medias":[]}`, `{"Medias":[{"OutputUrl":""}]}`} {
		if got := ExtractOutputURL(bad); got != "" {
			t.Errorf("ExtractOutputURL(%q) = %q, want empty", bad, got)
		}
	}
}

// TestBuildTaskInfoRemoteURLNotURL 这条是本文件最重要的断言。
//
// 成功时地址必须落在 RemoteUrl 而不是 Url：Url 非空会让轮询层把上游那个
// 有时效的 OSS 地址直接当成客户端可访问地址存进 ResultURL，
// 既暴露上游域名，也会在链接过期后变成死链。留空才会回落到网关代理。
func TestBuildTaskInfoRemoteURLNotURL(t *testing.T) {
	detail := &jobDetail{
		JobID:  "ag_1",
		Status: StatusFinished,
		Output: `{"Medias":[{"OutputUrl":"https://oss.example.com/a.mp4"}]}`,
	}
	ti := buildTaskInfo(detail)
	if ti.Status != model.TaskStatusSuccess {
		t.Errorf("Status = %v, want success", ti.Status)
	}
	if ti.RemoteUrl != "https://oss.example.com/a.mp4" {
		t.Errorf("RemoteUrl = %q", ti.RemoteUrl)
	}
	if ti.Url != "" {
		t.Errorf("Url 必须留空以触发网关代理回落，实际 = %q", ti.Url)
	}
}

// TestBuildTaskInfoStatusMapping 终态不能漏进 default 分支 ——
// 漏了会让任务被无限轮询，预扣额度永不结算。
func TestBuildTaskInfoStatusMapping(t *testing.T) {
	// TaskInfo.Status 是裸 string，而 model.TaskStatus* 是无类型常量，
	// 这里必须声明成 map[string]string，否则常量会被推断成 model.TaskStatus 而无法比较。
	cases := map[string]string{
		StatusCreated:   model.TaskStatusSubmitted,
		StatusQueuing:   model.TaskStatusQueued,
		StatusExecuting: model.TaskStatusInProgress,
		StatusFinished:  model.TaskStatusSuccess,
		StatusFailed:    model.TaskStatusFailure,
	}
	for status, want := range cases {
		ti := buildTaskInfo(&jobDetail{Status: status, Output: `{"Medias":[{"OutputUrl":"u"}]}`})
		if ti.Status != want {
			t.Errorf("status %s → %v, want %v", status, ti.Status, want)
		}
	}
	// 未知状态按进行中处理
	if ti := buildTaskInfo(&jobDetail{Status: "Weird"}); ti.Status != model.TaskStatusInProgress {
		t.Errorf("未知状态 → %v, want in-progress", ti.Status)
	}
}

// TestBuildTaskInfoFailureAlwaysHasReason 失败必须带原因，
// 否则前端展示空白，用户无从判断是自己的参数问题还是上游故障。
func TestBuildTaskInfoFailureAlwaysHasReason(t *testing.T) {
	cases := []jobDetail{
		{Status: StatusFailed, ErrorMessage: "content rejected"},
		{Status: StatusFailed, ErrorCode: "InvalidParameter"},
		{Status: StatusFailed},
	}
	for i, d := range cases {
		if ti := buildTaskInfo(&d); ti.Reason == "" {
			t.Errorf("case %d: 失败任务的 Reason 为空", i)
		}
	}
}

// TestParseJobDetailBothShapes 包装与扁平两种响应形态都要能解。
func TestParseJobDetailBothShapes(t *testing.T) {
	wrapped := []byte(`{"RequestId":"r1","VideoGenerationJob":{"JobId":"ag_1","Status":"Finished","Output":"{}"}}`)
	d, err := parseJobDetail(wrapped)
	if err != nil || d.JobID != "ag_1" || d.Status != StatusFinished {
		t.Errorf("包装形态解析失败: %+v, err=%v", d, err)
	}

	flat := []byte(`{"JobId":"ag_2","Status":"Executing"}`)
	d, err = parseJobDetail(flat)
	if err != nil || d.JobID != "ag_2" || d.Status != StatusExecuting {
		t.Errorf("扁平形态解析失败: %+v, err=%v", d, err)
	}

	if _, err := parseJobDetail([]byte(`{"Code":"Throttling","Message":"rate limited"}`)); err == nil {
		t.Error("错误响应应当返回 error 而不是一个空任务体")
	}
	if _, err := parseJobDetail([]byte(`not json`)); err == nil {
		t.Error("非 JSON 应当返回 error")
	}
}

// TestConvertToSubmitRequestFormats 集中验证那几个格式陷阱。
func TestConvertToSubmitRequestFormats(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Prompt:  "一只橘猫",
		Seconds: "10",
		Size:    "720p",
		Images:  []string{"media-a", "https://example.com/x.jpg"},
		Metadata: map[string]any{
			"aspect_ratio":   "16:9",
			"generate_audio": false,
		},
	}

	r, err := a.convertToSubmitRequest(&req, "Wonder-Pro", "task-1")
	if err != nil {
		t.Fatalf("convertToSubmitRequest 失败: %v", err)
	}

	if r.Duration != "10" {
		t.Errorf("Duration = %q，必须是字符串 \"10\"", r.Duration)
	}
	if r.Resolution != "720P" {
		t.Errorf("Resolution = %q，必须是大写 P", r.Resolution)
	}
	if r.AspectRatio != "16:9" {
		t.Errorf("AspectRatio = %q", r.AspectRatio)
	}
	if r.Model != "Wonder-Pro" {
		t.Errorf("Model = %q，应当用重定向后的上游模型名", r.Model)
	}
	// 直链被剔除，只剩一张登记素材 → image_to_video
	if r.JobType != JobTypeImage {
		t.Errorf("JobType = %q, want %q", r.JobType, JobTypeImage)
	}

	// Input 必须是序列化后的字符串，且直链不能混进去
	if !strings.HasPrefix(r.Input, "{") {
		t.Errorf("Input 应当是 JSON 字符串，实际 %q", r.Input)
	}
	if strings.Contains(r.Input, "example.com") {
		t.Errorf("直链混进了 Input: %s", r.Input)
	}
	if !strings.Contains(r.Input, "media-a") {
		t.Errorf("登记素材没进 Input: %s", r.Input)
	}

	// 显式传 false 必须下发，不能被 omitempty 吞掉（CLAUDE.md Rule 6）
	if !strings.Contains(r.JobParameters, `"EnableAudio":false`) {
		t.Errorf("显式 generate_audio=false 未下发，JobParameters = %q", r.JobParameters)
	}
}

// TestConvertToSubmitRequestClampsDuration 时长超出模型边界时夹到边界，
// 而不是原样透传让上游拒绝。
func TestConvertToSubmitRequestClampsDuration(t *testing.T) {
	a := &TaskAdaptor{}
	cases := []struct {
		model   string
		seconds string
		want    string
	}{
		{"Wonder-Pro", "60", "15"},   // 上限 15
		{"Wonder-Pro", "1", "4"},     // 下限 4
		{"Wonder-Ultra", "60", "30"}, // 上限 30
		{"Wonder-Ultra", "20", "20"}, // 区间内不动
		// 未收录的模型跳过夹取，原样下发 —— 管理员可能接了尚未收录的线路
		{"happyhorse-1.0", "60", "60"},
	}
	for _, tc := range cases {
		req := relaycommon.TaskSubmitReq{Prompt: "p", Seconds: tc.seconds}
		r, err := a.convertToSubmitRequest(&req, tc.model, "t")
		if err != nil {
			t.Fatalf("%s: %v", tc.model, err)
		}
		if r.Duration != tc.want {
			t.Errorf("%s seconds=%s → Duration=%q, want %q", tc.model, tc.seconds, r.Duration, tc.want)
		}
	}
}

// TestConvertToSubmitRequestRejectsTooManyMedias 素材超限本地就拦下，
// 不让用户白等一轮轮询再收到一个难懂的上游错误。
func TestConvertToSubmitRequestRejectsTooManyMedias(t *testing.T) {
	a := &TaskAdaptor{}
	many := make([]string, 20)
	for i := range many {
		many[i] = "media-" + string(rune('a'+i))
	}
	req := relaycommon.TaskSubmitReq{Prompt: "p", Images: many}

	if _, err := a.convertToSubmitRequest(&req, "Wonder-Pro", "t"); err == nil {
		t.Error("Wonder-Pro 上限 15，20 个素材应当被拒")
	}
	// Wonder-Ultra 上限 50，同样的输入应当放行
	if _, err := a.convertToSubmitRequest(&req, "Wonder-Ultra", "t"); err != nil {
		t.Errorf("Wonder-Ultra 上限 50，20 个素材不应被拒: %v", err)
	}
}

// TestEnableAudioOmittedWhenUnset 客户端没传时不能凭空下发默认值。
func TestEnableAudioOmittedWhenUnset(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{Prompt: "p"}
	r, err := a.convertToSubmitRequest(&req, "Wonder-Pro", "t")
	if err != nil {
		t.Fatal(err)
	}
	if r.JobParameters != "" {
		t.Errorf("未传 generate_audio 时不应下发 JobParameters，实际 %q", r.JobParameters)
	}
}

// TestModelMappingCoversModelList 每个对外模型名都要有建议映射，
// 漏一个就会有管理员照文档配完发现某个模型报"模型不存在"。
func TestModelMappingCoversModelList(t *testing.T) {
	for _, m := range ModelList {
		upstream, ok := SuggestedModelMapping[m]
		if !ok || upstream == "" {
			t.Errorf("对外模型 %q 缺少建议的上游映射", m)
		}
	}
	if len(SuggestedModelMapping) != len(ModelList) {
		t.Errorf("映射表有 %d 项，模型表有 %d 项，存在多余条目",
			len(SuggestedModelMapping), len(ModelList))
	}
}

// TestEstimateBillingRatios 计费倍率：时长按 5 秒一档向上取整，分辨率按表查。
func TestEstimateBillingRatios(t *testing.T) {
	cases := []struct {
		seconds     int
		wantSeconds float64
	}{
		{0, 1}, {1, 1}, {5, 1}, {6, 2}, {10, 2}, {11, 3}, {15, 3}, {30, 6},
	}
	for _, tc := range cases {
		got := float64(1)
		if tc.seconds > 0 {
			got = ceilUnits(tc.seconds)
		}
		if got != tc.wantSeconds {
			t.Errorf("%d 秒 → %g 个计费单位, want %g", tc.seconds, got, tc.wantSeconds)
		}
	}

	if resolutionRatios["720P"] != 1.0 {
		t.Errorf("720P 应当是基准价 1.0")
	}
	if resolutionRatios["1080P"] <= resolutionRatios["720P"] {
		t.Error("1080P 的倍率必须高于 720P")
	}
}

// ceilUnits 复刻 EstimateBilling 里的时长换算，便于在不构造 gin.Context 的情况下断言。
func ceilUnits(seconds int) float64 {
	units := float64(seconds) / float64(baseDurationSeconds)
	if units <= 1 {
		return 1
	}
	whole := float64(int(units))
	if units > whole {
		return whole + 1
	}
	return whole
}

// TestParseJobCredit 上游没有给响应样例，字段名与嵌套层级都待联调核对，
// 因此解析是"按候选键名递归找数值"。这里覆盖几种可能的形态。
func TestParseJobCredit(t *testing.T) {
	ok := []struct {
		body string
		want float64
	}{
		{`{"RequestId":"r","CreditCost":12.5}`, 12.5},
		{`{"RequestId":"r","Credit":{"CreditCost":3}}`, 3},
		{`{"Credit":{"Cost":"7.25"}}`, 7.25},
		{`{"Data":{"Credit":{"ConsumedCredit":9}}}`, 9},
	}
	for _, tc := range ok {
		got, found := parseJobCredit([]byte(tc.body))
		if !found || got != tc.want {
			t.Errorf("parseJobCredit(%s) = %g,%v; want %g,true", tc.body, got, found, tc.want)
		}
	}

	// 拿不到必须返回 false —— 返回 0,true 会被上层当成"本次免费"而全额退款
	bad := []string{
		`not json`,
		`{}`,
		`{"RequestId":"r"}`,
		`{"CreditCost":0}`,
		`{"CreditCost":"abc"}`,
	}
	for _, b := range bad {
		if got, found := parseJobCredit([]byte(b)); found {
			t.Errorf("parseJobCredit(%s) = %g,true; 应当返回 false", b, got)
		}
	}
}

// TestAdjustBillingOnCompleteGuards 三道闸门：非成功态、空入参一律保持预扣。
// 返回正数会触发差额结算，在信息不足时那等于拿客户余额赌一个未验证的假设。
func TestAdjustBillingOnCompleteGuards(t *testing.T) {
	a := &TaskAdaptor{}
	if got := a.AdjustBillingOnComplete(nil, nil); got != 0 {
		t.Errorf("空入参应返回 0，实际 %d", got)
	}
	task := &model.Task{ChannelId: 1}
	for _, status := range []string{model.TaskStatusFailure, model.TaskStatusInProgress, model.TaskStatusQueued} {
		if got := a.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: status}); got != 0 {
			t.Errorf("状态 %s 应返回 0（保持预扣），实际 %d", status, got)
		}
	}
}

// TestRejectsUnmappedExternalModel 模型重定向漏配时必须在出网前拦下。
//
// 对外模型名上游不存在，发过去只会换回一句"模型不存在"，
// 而真正原因是渠道配置漏了一项 —— 报错必须指向那一项。
func TestRejectsUnmappedExternalModel(t *testing.T) {
	a := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{Prompt: "p"}

	for _, external := range ModelList {
		_, err := a.convertToSubmitRequest(&req, external, "t")
		if err == nil {
			t.Errorf("对外模型名 %q 未重定向，应当被拦下", external)
			continue
		}
		// 报错要带上该配的目标值，否则管理员还得去翻文档
		if !strings.Contains(err.Error(), SuggestedModelMapping[external]) {
			t.Errorf("%q 的报错没给出应配的目标值: %v", external, err)
		}
	}

	// 已正确重定向的上游名必须放行
	for _, upstream := range []string{"Wonder-Standard", "Wonder-Pro", "Wonder-Ultra", "wan2.7"} {
		if _, err := a.convertToSubmitRequest(&req, upstream, "t"); err != nil {
			t.Errorf("上游模型名 %q 不应被拦下: %v", upstream, err)
		}
	}
}
