// Package yike 适配阿里云「万象一刻」（Yike）视频生成。
//
// 与已接入的豆包/润元相比，这是一套**完全不同的协议**，没有任何一层能复用：
//
//	         豆包 / 润元                     万象一刻
//	协议     REST，路径区分操作              阿里云 RPC，x-acs-action 区分操作
//	鉴权     Authorization: Bearer          阿里云 RAM AK/SK（ACS3-HMAC-SHA256）
//	提示词   content[] 数组                  Input 字段内的 JSON **字符串**
//	时长     duration: 5（整数）             Duration: "10"（**字符串**）
//	分辨率   "720p"（小写）                  "720P"（**大写 P**）
//	宽高比   ratio                          **AspectRatio**
//	音频     generate_audio                 JobParameters 里的 EnableAudio
//	水印     watermark                      **不支持**
//	状态     succeeded/failed               Finished/Failed/Created/Queuing/Executing
//	结果     content.video_url              Output 字符串里的 Medias[].OutputUrl
package yike

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/aliyunsign"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string

	// reqBody 暂存 BuildRequestBody 产出的字节，供 BuildRequestHeader 签名使用。
	//
	// 签名必须覆盖请求体，而接口把「构造体」与「设置头」拆成了两步。
	// 好在 relay_task.go 的调用顺序是 BuildRequestBody → DoRequest(→BuildRequestHeader)，
	// 这里存一次即可。适配器实例是每次请求新建的，不存在并发复用问题。
	reqBody []byte
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = strings.TrimSuffix(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL 阿里云 RPC 没有按操作区分的路径 —— 动作放在 x-acs-action 头里。
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	if a.baseURL == "" {
		return "", errors.New("channel base url is empty")
	}
	return a.baseURL + "/", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	creds, err := aliyunsign.ParseAKSK(a.apiKey)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return aliyunsign.Sign(req, creds, a.reqBody, aliyunsign.Options{
		Action:  ActionSubmitJob,
		Version: APIVersion,
	})
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	// 上游模型名此时已由 ModelMappedHelper 完成重定向。
	upstreamModel := info.UpstreamModelName
	if upstreamModel == "" {
		upstreamModel = req.Model
		info.UpstreamModelName = upstreamModel
	}

	body, err := a.convertToSubmitRequest(&req, upstreamModel, info.PublicTaskID)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	a.reqBody = data

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("yike upstream request: %s", string(data)))
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("upstream error: %s", ExtractUpstreamError(responseBody)),
			"upstream_error", resp.StatusCode)
	}

	var submitted submitResponse
	if err := common.Unmarshal(responseBody, &submitted); err == nil && submitted.JobID != "" {
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return submitted.JobID, responseBody, nil
	}

	// JobId 为空必须报错：静默成功会让任务入库却永远轮询不出结果，而额度已经预扣。
	return "", nil, service.TaskErrorWrapper(
		fmt.Errorf("JobId is empty, body: %s", string(responseBody)),
		"invalid_response", http.StatusInternalServerError)
}

// FetchTask 轮询任务状态。
//
// 与 REST 上游不同，这里仍然是 POST 到根路径，靠 x-acs-action 区分动作。
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	jobID, ok := body["task_id"].(string)
	if !ok || jobID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	return doAction(baseUrl, key, proxy, ActionGetJob, map[string]any{"JobId": jobID})
}

// doAction 发一次已签名的 RPC 调用，供轮询、计费查询、素材登记共用。
func doAction(baseUrl, key, proxy, action string, params map[string]any) (*http.Response, error) {
	creds, err := aliyunsign.ParseAKSK(key)
	if err != nil {
		return nil, err
	}
	payload, err := common.Marshal(params)
	if err != nil {
		return nil, err
	}

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(baseUrl, "/")+"/", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if err := aliyunsign.Sign(req, creds, payload, aliyunsign.Options{
		Action:  action,
		Version: APIVersion,
	}); err != nil {
		return nil, err
	}
	return client.Do(req)
}

// ParseTaskResult 解析查询响应。
//
// 成功时把地址写进 RemoteUrl 而**不是** Url —— 与润元同理：
// Url 非空会让轮询层把上游地址直接当作客户端可访问的地址存进 ResultURL。
// 留空则回落到 taskcommon.BuildProxyURL()，客户端改走网关代理，
// 既不暴露上游域名，也能兜住 OutputUrl 的有效期问题（上游明确是临时地址）。
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	detail, err := parseJobDetail(respBody)
	if err != nil {
		return nil, err
	}
	return buildTaskInfo(detail), nil
}

// parseJobDetail 从查询响应里取出任务体。
//
// 阿里云 RPC 响应的字段名大小写偶有出入，因此先按包装层解，
// 解不出来再把整个响应当作任务体试一次 —— 上游改形态时不至于整条链路失效。
func parseJobDetail(respBody []byte) (*jobDetail, error) {
	var wrapper getJobResponse
	if err := common.Unmarshal(respBody, &wrapper); err == nil && len(wrapper.VideoGenerationJob) > 0 {
		var d jobDetail
		if err := common.Unmarshal(wrapper.VideoGenerationJob, &d); err == nil && (d.Status != "" || d.JobID != "") {
			return &d, nil
		}
	}

	var flat jobDetail
	if err := common.Unmarshal(respBody, &flat); err == nil && (flat.Status != "" || flat.JobID != "") {
		return &flat, nil
	}

	if msg := ExtractUpstreamError(respBody); msg != "" && msg != string(respBody) {
		return nil, errors.New(msg)
	}
	return nil, errors.New("unmarshal task result failed, unknown format")
}

func buildTaskInfo(detail *jobDetail) *relaycommon.TaskInfo {
	ti := relaycommon.TaskInfo{Code: 0}

	switch detail.Status {
	case StatusCreated:
		ti.Status = model.TaskStatusSubmitted
		ti.Progress = "10%"
	case StatusQueuing:
		ti.Status = model.TaskStatusQueued
		ti.Progress = "20%"
	case StatusExecuting:
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
	case StatusFinished:
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		ti.RemoteUrl = ExtractOutputURL(detail.Output)
	case StatusFailed:
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		ti.Reason = service.ScrubUpstreamText(detail.ErrorMessage)
		if ti.Reason == "" {
			ti.Reason = service.ScrubUpstreamText(detail.ErrorCode)
		}
		if ti.Reason == "" {
			ti.Reason = "task failed"
		}
	default:
		// 未知状态按"还在跑"处理，但不能把已知终态漏进这里 ——
		// 那会导致任务被无限轮询、预扣额度永不结算。
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "30%"
	}

	return &ti
}

// ExtractOutputURL 从 Output 字符串里取出视频地址。
//
// Output 是**序列化后的 JSON 字符串**，需要二次解析：
//
//	{"Medias":[{"OutputUrl":"https://.../x.mp4"}]}
//
// 取不出来时返回空串，由调用方按"没拿到地址"处理，而不是让整个响应解析失败。
func ExtractOutputURL(output string) string {
	if strings.TrimSpace(output) == "" {
		return ""
	}
	var parsed jobOutput
	if err := common.Unmarshal([]byte(output), &parsed); err != nil {
		return ""
	}
	for _, m := range parsed.Medias {
		if m.OutputURL != "" {
			return m.OutputURL
		}
	}
	return ""
}

// ExtractOutputURLFromTaskData 从任务快照里取上游真实视频地址，供视频代理回源使用。
func ExtractOutputURLFromTaskData(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	detail, err := parseJobDetail(data)
	if err != nil {
		return ""
	}
	return ExtractOutputURL(detail.Output)
}

// ExtractUpstreamError 从阿里云 RPC 错误体中提取可读信息，并清洗上游品牌痕迹。
func ExtractUpstreamError(body []byte) string {
	var e errorResponse
	if err := common.Unmarshal(body, &e); err == nil {
		if e.Message != "" {
			return service.ScrubUpstreamText(e.Message)
		}
		if e.Code != "" {
			return service.ScrubUpstreamText(e.Code)
		}
	}
	return service.ScrubUpstreamText(string(body))
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

// ConvertToOpenAIVideo metadata.url 必须是代理地址，
// 不能是 Output 里那个有时效的上游地址。
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	ov := dto.NewOpenAIVideo()
	ov.ID = originTask.TaskID
	ov.TaskID = originTask.TaskID
	ov.Status = originTask.Status.ToVideoStatus()
	ov.SetProgressStr(originTask.Progress)
	ov.CreatedAt = originTask.CreatedAt
	ov.CompletedAt = originTask.UpdatedAt
	ov.Model = originTask.Properties.OriginModelName

	if originTask.Status == model.TaskStatusSuccess {
		ov.SetMetadata("url", taskcommon.BuildProxyURL(originTask.TaskID))
	}
	if originTask.Status == model.TaskStatusFailure {
		if detail, err := parseJobDetail(originTask.Data); err == nil && detail.ErrorMessage != "" {
			ov.Error = &dto.OpenAIVideoError{
				Message: service.ScrubUpstreamText(detail.ErrorMessage),
				Code:    detail.ErrorCode,
			}
		}
	}

	return common.Marshal(ov)
}

// EstimateBilling 返回附加倍率：时长与分辨率。
// 后台应将模型价格设为 5 秒 720P 的基准价。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	ratios := make(map[string]float64, 2)

	seconds := resolveDuration(&req)
	if seconds <= 0 {
		seconds = baseDurationSeconds
	}
	ratios["seconds"] = math.Max(1, math.Ceil(float64(seconds)/float64(baseDurationSeconds)))

	ratios["size"] = 1.0
	if r, ok := resolutionRatios[normalizeResolution(resolveResolution(&req))]; ok {
		ratios["size"] = r
	}

	return ratios
}

// ============================
// 请求体构造
// ============================

func (a *TaskAdaptor) convertToSubmitRequest(req *relaycommon.TaskSubmitReq, upstreamModel, clientToken string) (*submitRequest, error) {
	// 模型重定向漏配的早期拦截。
	//
	// 对外模型名（seedance-2.0 等）上游根本不存在，直接发过去只会换回一句
	// "模型不存在"，而真正的原因是渠道「模型重定向」没配 —— 两者之间毫无线索可循。
	// 这里在出网之前就把话说清楚，并直接给出该填的值。
	if target, isExternalName := SuggestedModelMapping[upstreamModel]; isExternalName {
		return nil, errors.Errorf(
			"model %q was not redirected: configure the channel's model mapping with %q -> %q",
			upstreamModel, upstreamModel, target)
	}

	medias := collectMedias(req)

	// 素材数量本地先拦一道：超限时上游的报错很难对应回具体参数，
	// 而且额度此时已经预扣，让用户白等一轮轮询。
	if limit, ok := GetModelLimit(upstreamModel); ok && limit.MaxMedias > 0 && len(medias) > limit.MaxMedias {
		return nil, errors.Errorf("too many reference medias: %d provided, at most %d allowed",
			len(medias), limit.MaxMedias)
	}

	input := jobInput{Prompt: req.Prompt, Medias: medias}
	inputJSON, err := common.Marshal(input)
	if err != nil {
		return nil, errors.Wrap(err, "marshal input failed")
	}

	r := &submitRequest{
		Model: upstreamModel,
		// jobType 由素材构成推导，不接受调用方指定 —— 传错上游会直接拒。
		JobType:     resolveJobType(medias),
		Scene:       DefaultScene,
		Input:       string(inputJSON), // 序列化成字符串，不是嵌套对象
		AspectRatio: resolveAspectRatio(req),
		Resolution:  normalizeResolution(resolveResolution(req)),
		N:           1,
		ClientToken: clientToken,
	}

	if seconds := resolveDuration(req); seconds > 0 {
		if limit, ok := GetModelLimit(upstreamModel); ok {
			if seconds < limit.MinDuration {
				seconds = limit.MinDuration
			}
			if seconds > limit.MaxDuration {
				seconds = limit.MaxDuration
			}
		}
		r.Duration = strconv.Itoa(seconds) // 字符串，不是整数
	}

	if enableAudio, ok := metadataBool(req.Metadata, "generate_audio"); ok {
		params, err := common.Marshal(jobParameters{EnableAudio: &enableAudio})
		if err != nil {
			return nil, errors.Wrap(err, "marshal job parameters failed")
		}
		r.JobParameters = string(params)
	}

	return r, nil
}

// collectMedias 把请求里的素材引用收成上游要的形态。
//
// 只接受**已登记**的 MediaId（形如 media-xxx）。Wonder 系列不接受外部 URL，
// 必须先经 ImportMedia 登记。这里对直链一律跳过并让 jobType 退化，
// 而不是塞给上游让它报一个难懂的错。
func collectMedias(req *relaycommon.TaskSubmitReq) []Media {
	var medias []Media
	add := func(list []string, mediaType string) {
		for _, ref := range list {
			if id := normalizeMediaID(ref); id != "" {
				medias = append(medias, Media{Type: mediaType, MediaID: id})
			}
		}
	}
	if req.Image != "" {
		add([]string{req.Image}, "image")
	}
	add(req.Images, "image")
	add(req.Videos, "video")
	add(req.Audios, "audio")
	return medias
}

// normalizeMediaID 接受 media-xxx 与 asset://media-xxx 两种写法，
// 后者是为了与已有渠道的素材引用习惯保持一致。直链一律返回空串。
func normalizeMediaID(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "asset://")
	if ref == "" || strings.HasPrefix(strings.ToLower(ref), "http") {
		return ""
	}
	return ref
}

// resolveJobType 按素材构成推导 jobType，规则来自上游文档。
func resolveJobType(medias []Media) string {
	if len(medias) == 0 {
		return JobTypeText
	}
	images := 0
	for _, m := range medias {
		if m.Type != "image" {
			// 出现视频或音频，一律走 reference_to_video
			return JobTypeReference
		}
		images++
	}
	switch images {
	case 1:
		return JobTypeImage
	case 2:
		return JobTypeFirstLastFrame
	default:
		return JobTypeReference
	}
}

func resolveDuration(req *relaycommon.TaskSubmitReq) int {
	if s, _ := strconv.Atoi(req.Seconds); s > 0 {
		return s
	}
	if req.Duration > 0 {
		return req.Duration
	}
	if v, ok := metadataInt(req.Metadata, "duration"); ok {
		return v
	}
	return 0
}

func resolveResolution(req *relaycommon.TaskSubmitReq) string {
	if v := metadataString(req.Metadata, "resolution"); v != "" {
		return v
	}
	return req.Size
}

func resolveAspectRatio(req *relaycommon.TaskSubmitReq) string {
	for _, key := range []string{"aspect_ratio", "aspectRatio", "ratio"} {
		if v := metadataString(req.Metadata, key); v != "" && supportedAspectRatios[v] {
			return v
		}
	}
	return ""
}

// normalizeResolution 归一化到上游要的 720P / 1080P。
//
// 三种来源都要接住，否则上游直接拒：
//   - "720p" 小写 —— 上游只认大写 P
//   - "1280x720" OpenAI 风格的 size —— 按短边归档
//   - "720P" 已经正确 —— 原样返回
//
// 认不出来的写法返回空串而不是原样透传：空串上游会用模型默认值，
// 透传一个它不认识的字符串则是必然失败。
func normalizeResolution(raw string) string {
	r := strings.ToUpper(strings.TrimSpace(raw))
	if r == "" {
		return ""
	}
	if _, ok := resolutionRatios[r]; ok {
		return r
	}
	// "1280X720" / "720X1280" → 取短边
	if w, h, ok := parseDimensions(r); ok {
		short := w
		if h < short {
			short = h
		}
		switch {
		case short >= 1080:
			return "1080P"
		case short >= 720:
			return "720P"
		}
		return ""
	}
	return ""
}

func parseDimensions(s string) (int, int, bool) {
	parts := strings.Split(s, "X")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key].(string); ok {
		return v
	}
	return ""
}

func metadataInt(meta map[string]any, key string) (int, bool) {
	if meta == nil {
		return 0, false
	}
	switch v := meta[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case string:
		if n, err := strconv.Atoi(strings.TrimSuffix(v, "s")); err == nil {
			return n, true
		}
	}
	return 0, false
}

func metadataBool(meta map[string]any, key string) (bool, bool) {
	if meta == nil {
		return false, false
	}
	switch v := meta[key].(type) {
	case bool:
		return v, true
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b, true
		}
	}
	return false, false
}
