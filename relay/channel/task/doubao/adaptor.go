package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	"github.com/samber/lo"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string                 `json:"model"`
	Prompt                string                 `json:"prompt,omitempty"`
	Content               []ContentItem          `json:"content,omitempty"`
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
	CallbackURL           string                 `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue         `json:"return_last_frame,omitempty"`
	ServiceTier           string                 `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue          `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue         `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue         `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	Resolution  string         `json:"resolution,omitempty"`
	Ratio       string         `json:"ratio,omitempty"`
	Duration    *dto.IntValue  `json:"duration,omitempty"`
	Frames      *dto.IntValue  `json:"frames,omitempty"`
	Seed        *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark   *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
}

// stelloriaRequestPayload 是 Stelloria 的扁平请求体，与 Ark 的 content[] 结构完全不同。
//
// duration 是字符串（"5s" / "10s"），不是整数秒；宽高比字段叫 aspect_ratio 而非 ratio。
//
// 只发官方参数表里列出的字段。文档「注意事项」里那句「content 数组中必须包含至少一个
// text 类型元素」是从 seedance 原生文档抄漏的——实测把 content 一起发过去，
// Stelloria 会回 502 "upstream returned empty task id"；去掉后同一请求立即成功。
// 参考图走 image_url（可填 asset:// 素材引用），这是上游唯一支持的素材入口。
type stelloriaRequestPayload struct {
	Model       string        `json:"model"`
	Prompt      string        `json:"prompt"`
	ImageURL    string        `json:"image_url,omitempty"`
	Duration    string        `json:"duration,omitempty"`
	AspectRatio string        `json:"aspect_ratio,omitempty"`
	Resolution  string        `json:"resolution,omitempty"`
	FPS         *dto.IntValue `json:"fps,omitempty"`
	Seed        *dto.IntValue `json:"seed,omitempty"`
}

// stelloriaNativePayload 是 Stelloria 原生协议透传请求体。
//
// 与扁平模式的区别：
//   - content 数组原样透传（支持 first_frame / last_frame / reference_image / reference_video / reference_audio）
//   - duration 是整数秒（5 / 10），不是字符串 "5s"
//   - 宽高比字段叫 ratio，不是 aspect_ratio
//   - 没有独立的 prompt 字段（文本在 content[].text 里）
//   - 支持 generate_audio / watermark 布尔参数
type stelloriaNativePayload struct {
	Model         string          `json:"model"`
	Content       json.RawMessage `json:"content"`
	Resolution    string          `json:"resolution,omitempty"`
	Duration      int             `json:"duration,omitempty"`
	Ratio         string          `json:"ratio,omitempty"`
	GenerateAudio *bool           `json:"generate_audio,omitempty"`
	Watermark     *bool           `json:"watermark,omitempty"`
	Seed          *dto.IntValue   `json:"seed,omitempty"`
}

// stelloriaTaskResponse 是 Stelloria 的提交与查询响应。
//
// 实测的查询响应与官方文档不一致，以实测为准：
//   - 视频地址在**顶层 result_url**，不是文档写的 result.video_url
//   - result 里装的是上游（火山 Ark）的原始响应，形如
//     {"id":"cgt-...","status":"succeeded","content":{"video_url":"..."},"usage":{...}}
//
// 因此 Result / Error 都用 json.RawMessage 延迟解析：上游形态会变，
// 声明成固定结构一旦对不上就会导致整个响应 unmarshal 失败、任务永远轮询不出结果
// （seegen 的 content 数组就是这么踩的）。
type stelloriaTaskResponse struct {
	TaskID    string          `json:"task_id"`
	Status    string          `json:"status"`
	Model     string          `json:"model"`
	ResultURL string          `json:"result_url"`
	Result    json.RawMessage `json:"result"`
	Error     json.RawMessage `json:"error"`
}

// stelloriaResultDetail 是 result 字段内部可能出现的几种形态的并集。
type stelloriaResultDetail struct {
	Status    string          `json:"status"`
	VideoURL  string          `json:"video_url"`
	ResultURL string          `json:"result_url"`
	Content   json.RawMessage `json:"content"`
	Usage     struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// stelloriaVideoURL 按优先级取视频地址，覆盖已知的全部形态。
func stelloriaVideoURL(resp *stelloriaTaskResponse) string {
	// 1. 顶层 result_url —— 实测就是这个
	if resp.ResultURL != "" {
		return resp.ResultURL
	}
	detail, ok := parseStelloriaResultDetail(resp)
	if !ok {
		return ""
	}
	if detail.ResultURL != "" {
		return detail.ResultURL
	}
	// 2. 文档描述的 result.video_url
	if detail.VideoURL != "" {
		return detail.VideoURL
	}
	// 3. result.content —— 上游 Ark 原始响应，复用已有的三形态兼容解析
	return extractVideoURL(detail.Content)
}

func parseStelloriaResultDetail(resp *stelloriaTaskResponse) (*stelloriaResultDetail, bool) {
	if len(resp.Result) == 0 {
		return nil, false
	}
	var detail stelloriaResultDetail
	if err := common.Unmarshal(resp.Result, &detail); err != nil {
		return nil, false
	}
	return &detail, true
}

// stelloriaErrorMessage 从顶层或 result 内部取错误信息。
// error 字段既可能是字符串也可能是 {"message":...} 对象，两种都要认。
func stelloriaErrorMessage(resp *stelloriaTaskResponse) string {
	if msg := flexibleErrorMessage(resp.Error); msg != "" {
		return service.ScrubUpstreamText(msg)
	}
	if detail, ok := parseStelloriaResultDetail(resp); ok {
		return service.ScrubUpstreamText(flexibleErrorMessage(detail.Error))
	}
	return ""
}

func flexibleErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := common.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asObject struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	if err := common.Unmarshal(raw, &asObject); err == nil {
		if asObject.Message != "" {
			return asObject.Message
		}
		return asObject.Reason
	}
	return ""
}

// kkidc 响应结构体（OpenAI 兼容格式）
type kkidcSubmitResponse struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	CreatedAt int64  `json:"created_at"`
}

type kkidcTaskData struct {
	ID         string `json:"task_id"`
	Status     string `json:"status"`
	ResultURL  string `json:"result_url"`
	FailReason string `json:"fail_reason"`
	Progress   string `json:"progress"`
	Usage      struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
}

type kkidcQueryResponse struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Data    kkidcTaskData `json:"data"`
}

type responseTask struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	// Content 的形态在不同上游之间不一致，必须延迟解析：
	//   火山原生 Ark: {"content": {"video_url": "https://..."}}
	//   seegen.ai:    {"content": [{"type":"video_url","video_url":{"url":"https://..."}}]}
	// 如果声明成固定结构，遇到另一种形态会导致整个响应 unmarshal 失败，
	// 表现为任务能提交但永远轮询不出结果。用 extractVideoURL 统一兼容。
	Content         json.RawMessage `json:"content"`
	Seed            int             `json:"seed"`
	Resolution      string          `json:"resolution"`
	Duration        int             `json:"duration"`
	Ratio           string          `json:"ratio"`
	FramesPerSecond int             `json:"framespersecond"`
	ServiceTier     string          `json:"service_tier"`
	// Tools 在不同上游之间形态不一致：火山 Ark 返回数组，润元返回空对象 {}。
	// 声明为 json.RawMessage 避免 strict unmarshal 失败导致整个响应解析挂掉。
	Tools json.RawMessage `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// isKKIDCBaseURL 判断上游是否为 kkidc 网关（OpenAI 兼容协议）。
func isKKIDCBaseURL(baseURL string) bool {
	return strings.Contains(baseURL, "kkidc")
}

// isSeegenBaseURL 判断上游是否为 seegen.ai。
//
// seegen 的请求/响应参数与火山 Ark 一致，但路径前缀是 /v1 而不是 /api/v3，
// 所以只需要单独处理 URL，请求体构造仍走 Ark 分支。
func isSeegenBaseURL(baseURL string) bool {
	return strings.Contains(baseURL, "seegen")
}

// isStelloriaBaseURL 判断上游是否为星瞳 Stelloria。
//
// Stelloria 与前三家在每一层都不同，是完全独立的一套协议：
//   - 提交 POST /v1/videos/generations（复数 videos，注意不是 kkidc 的单数 video）
//   - 查询 GET  /v1/tasks/{id}（换了前缀，不在提交路径下）
//   - 请求体是扁平结构（prompt / image_url / duration:"5s" / aspect_ratio），
//     不是 Ark 的 content[] 数组
//   - 提交返回 {"task_id":...}，查询返回 {"status":"completed","result":{"video_url":...}}
//   - 状态取值是 processing / completed / failed
func isStelloriaBaseURL(baseURL string) bool {
	return strings.Contains(baseURL, "stelloria")
}

// isRunyuanBaseURL 判断上游是否为润元（runy.yitd.cn）。
//
// 润元与火山 Ark 只差路径：
//   - 提交 POST /v1/video/tasks（单数 video + tasks，四家里独一份）
//   - 查询 GET  /v1/video/tasks/{task_id}
//   - 请求体就是 Ark 的 content[] 结构（text / image_url / video_url / audio_url + role），
//     ratio / resolution / duration(int) 全部同名同义，因此复用 Ark 分支构造
//   - 提交返回 {"status":"submitted","task_id":...}，走 DoResponse 的 task_id 分支
//   - 查询返回 Ark 原生形态 {"id":...,"status":"succeeded","content":{"video_url":...}}，
//     走 ParseTaskResult 的 Ark 分支
//
// 所以只需要在 URL 这一层单独开一支。
func isRunyuanBaseURL(baseURL string) bool {
	return strings.Contains(baseURL, "yitd")
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	// kkidc 使用 OpenAI 兼容路径 /v1/video/generations
	if isKKIDCBaseURL(a.baseURL) {
		return fmt.Sprintf("%s/v1/video/generations", a.baseURL), nil
	}
	// seegen.ai 使用 /v1/contents/generations/tasks
	if isSeegenBaseURL(a.baseURL) {
		return fmt.Sprintf("%s/v1/contents/generations/tasks", a.baseURL), nil
	}
	// Stelloria 使用 /v1/videos/generations（复数 videos）
	if isStelloriaBaseURL(a.baseURL) {
		return fmt.Sprintf("%s/v1/videos/generations", a.baseURL), nil
	}
	// 润元使用 /v1/video/tasks
	if isRunyuanBaseURL(a.baseURL) {
		return fmt.Sprintf("%s/v1/video/tasks", a.baseURL), nil
	}
	// 火山引擎原生 Ark
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 根据请求参数计算 OtherRatios：视频输入折扣、时长倍率、分辨率倍率。
// 后台应将模型价格设为 5秒 720p 的基准价，系统会自动乘以 seconds × size。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	ratios := make(map[string]float64)

	// 1. 视频输入折扣（参考视频）
	if hasVideoInMetadata(req.Metadata) {
		if ratio, ok := GetVideoInputRatio(info.OriginModelName); ok {
			ratios["video_input"] = ratio
		}
	}

	// 2. 时长倍率：每 5 秒为一个单位，向上取整
	seconds := 5 // 默认 5 秒
	if req.Duration > 0 {
		seconds = req.Duration
	} else if s, _ := strconv.Atoi(req.Seconds); s > 0 {
		seconds = s
	} else if meta, ok := req.Metadata["seconds"].(string); ok {
		if s, _ := strconv.Atoi(meta); s > 0 {
			seconds = s
		}
	} else if meta, ok := req.Metadata["duration"].(float64); ok {
		seconds = int(meta)
	}
	ratios["seconds"] = math.Max(1, math.Ceil(float64(seconds)/5))

	// 3. 分辨率倍率
	size := req.Size
	if size == "" {
		if meta, ok := req.Metadata["size"].(string); ok {
			size = meta
		}
	}
	ratios["size"] = 1.0
	switch size {
	case "480x480", "480x854", "854x480":
		ratios["size"] = 0.5
	case "1024x1024", "1024x576", "576x1024", "720x1280", "1280x720":
		ratios["size"] = 1.0
	case "1280x1280", "1920x1080", "1080x1920":
		ratios["size"] = 1.5
	case "2048x2048":
		ratios["size"] = 2.0
	}

	return ratios
}

// hasVideoInMetadata 直接检查 metadata 的 content 数组是否包含 video_url 条目，
// 避免构建完整的上游 requestPayload。
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	// Stelloria 的请求体是完全不同的扁平结构，单独构造
	if isStelloriaBaseURL(a.baseURL) {
		// 检测是否原生协议：客户端带了 content 数组
		if content, hasContent := req.Metadata["content"]; hasContent && content != nil {
			body := a.convertToStelloriaNativePayload(&req)
			if info.IsModelMapped {
				body.Model = info.UpstreamModelName
			} else {
				info.UpstreamModelName = body.Model
			}
			data, err := common.Marshal(body)
			if err != nil {
				return nil, err
			}
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("stelloria native upstream request: %s", string(data)))
			return bytes.NewReader(data), nil
		}

		body := a.convertToStelloriaPayload(&req)
		if info.IsModelMapped {
			body.Model = info.UpstreamModelName
		} else {
			info.UpstreamModelName = body.Model
		}
		data, err := common.Marshal(body)
		if err != nil {
			return nil, err
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("stelloria upstream request: %s", string(data)))
		return bytes.NewReader(data), nil
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("doubao upstream request: %s", string(data)))

	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// extractUpstreamErrorMsg 尝试从各种上游错误格式中提取可读错误信息。
// 返回前统一清洗掉上游的品牌与域名痕迹，避免把真实供应商暴露给客户。
func extractUpstreamErrorMsg(body []byte) string {
	return service.ScrubUpstreamText(extractUpstreamErrorMsgRaw(body))
}

func extractUpstreamErrorMsgRaw(body []byte) string {
	var m map[string]interface{}
	if err := common.Unmarshal(body, &m); err != nil {
		return string(body)
	}
	// 优先 message
	if msg, ok := m["message"].(string); ok && msg != "" {
		return msg
	}
	// 其次 error.message
	if errObj, ok := m["error"].(map[string]interface{}); ok {
		if msg, ok := errObj["message"].(string); ok && msg != "" {
			return msg
		}
	}
	// 再试 msg
	if msg, ok := m["msg"].(string); ok && msg != "" {
		return msg
	}
	// 最后 code
	if code, ok := m["code"].(string); ok && code != "" {
		return code
	}
	return string(body)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 上游返回非 2xx 时，直接透传错误
	if resp.StatusCode >= 400 {
		errMsg := extractUpstreamErrorMsg(responseBody)
		taskErr = service.TaskErrorWrapper(fmt.Errorf("upstream error: %s", errMsg), "upstream_error", resp.StatusCode)
		return
	}

	// Parse Doubao response（先尝试官方 Ark 格式）
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err == nil && dResp.ID != "" {
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return dResp.ID, responseBody, nil
	}

	// 再尝试顶层 task_id 的格式。
	// kkidc 与 Stelloria 的提交响应都是 {"task_id":...,"status":...}，共用这一支即可。
	var kkidcResp kkidcSubmitResponse
	if err := common.Unmarshal(responseBody, &kkidcResp); err == nil && kkidcResp.TaskID != "" {
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return kkidcResp.TaskID, responseBody, nil
	}

	taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty, body: %s", string(responseBody)), "invalid_response", http.StatusInternalServerError)
	return
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var uri string
	switch {
	case isKKIDCBaseURL(baseUrl):
		uri = fmt.Sprintf("%s/v1/video/generations/%s", baseUrl, taskID)
	case isSeegenBaseURL(baseUrl):
		uri = fmt.Sprintf("%s/v1/contents/generations/tasks/%s", baseUrl, taskID)
	case isStelloriaBaseURL(baseUrl):
		// Stelloria 的查询不在提交路径下，而是独立的 /v1/tasks/{id}。
		//
		// 注意：官方文档在这里自相矛盾——Python 示例写的是 /tasks/{id}（无 /v1），
		// cURL 示例和「响应格式」小节写的都是 /v1/tasks/{id}。这里按二比一取
		// /v1/tasks/{id}，若上游 404 会自动回退到 /tasks/{id}（见下方重试逻辑）。
		uri = fmt.Sprintf("%s/v1/tasks/%s", baseUrl, taskID)
	case isRunyuanBaseURL(baseUrl):
		uri = fmt.Sprintf("%s/v1/video/tasks/%s", baseUrl, taskID)
	default:
		uri = fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)
	}

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}

	resp, err := doFetchTaskGet(client, uri, key)
	if err != nil {
		return nil, err
	}

	// Stelloria 文档对轮询路径的写法自相矛盾，这里做一次容错回退：
	// /v1/tasks/{id} 拿到 404 时改试 /tasks/{id}。只在 Stelloria 上生效，
	// 其余上游的 404 保持原样交给上层判断。
	if resp.StatusCode == http.StatusNotFound && isStelloriaBaseURL(baseUrl) {
		_ = resp.Body.Close()
		fallback := fmt.Sprintf("%s/tasks/%s", baseUrl, taskID)
		return doFetchTaskGet(client, fallback, key)
	}
	return resp, nil
}

// doFetchTaskGet 发一个带鉴权的 GET，供 FetchTask 与其回退路径共用。
func doFetchTaskGet(client *http.Client, uri, key string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	// seegen 与火山原生 Ark 共用同一套请求体格式，只有 URL 不同，
	// 因此这里只需要区分 kkidc（OpenAI 兼容格式）与其余。
	isKKIDC := isKKIDCBaseURL(a.baseURL)

	r := requestPayload{
		Model: req.Model,
	}

	// 只有非 kkidc 才构建 content 数组（官方 Ark 格式）
	if !isKKIDC {
		r.Content = []ContentItem{}
		// Add images if present
		if req.HasImage() {
			for _, imgURL := range req.Images {
				r.Content = append(r.Content, ContentItem{
					Type: "image_url",
					ImageURL: &MediaURL{
						URL: imgURL,
					},
				})
			}
		}
		// Add videos if present
		if req.HasVideo() {
			for _, videoURL := range req.Videos {
				r.Content = append(r.Content, ContentItem{
					Type: "video_url",
					VideoURL: &MediaURL{
						URL: videoURL,
					},
				})
			}
		}
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	} else if req.Duration > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(req.Duration))
	}

	// 官方 Ark 用 content 数组：文本放在最前面，移除旧的 text 条目
	if !isKKIDC {
		r.Prompt = "" // Ark API 不需要 prompt 字段，清空以避免泄漏
		r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
		r.Content = append([]ContentItem{{
			Type: "text",
			Text: req.Prompt,
		}}, r.Content...)
	}

	// 只有 kkidc 才需要 top-level prompt 字段和 metadata 字段
	// 官方 Ark API 不需要这些字段，发送它们会干扰上游行为
	if isKKIDC {
		r.Prompt = req.Prompt

		meta := make(map[string]interface{})
		// 先复制用户传入的 metadata
		if metadata != nil {
			for k, v := range metadata {
				meta[k] = v
			}
		}
		// 多模态资源（字符串数组）
		if len(req.Images) > 0 {
			meta["reference_images"] = req.Images
		}
		if len(req.Videos) > 0 {
			meta["reference_videos"] = req.Videos
		}
		if len(req.Audios) > 0 {
			meta["reference_audios"] = req.Audios
		}
		if req.Image != "" {
			meta["first_frame_image"] = req.Image
		}
		if len(meta) > 0 {
			r.Metadata = meta
		}
	}

	return &r, nil
}

// convertToStelloriaPayload 把统一的任务请求转换成 Stelloria 的扁平请求体。
//
// 关键差异：duration 是字符串（"5s"），宽高比字段叫 aspect_ratio。
// 参考图走 image_url，可以填 asset://<id> 素材引用。
func (a *TaskAdaptor) convertToStelloriaPayload(req *relaycommon.TaskSubmitReq) *stelloriaRequestPayload {
	r := &stelloriaRequestPayload{
		Model:  req.Model,
		Prompt: req.Prompt,
	}

	// 先从 metadata 取通用参数（resolution / seed / fps 等同名字段）
	meta := req.Metadata
	r.Resolution = metadataString(meta, "resolution")
	// 宽高比：优先 aspect_ratio，其次沿用 Ark 风格的 ratio
	r.AspectRatio = metadataString(meta, "aspect_ratio")
	if r.AspectRatio == "" {
		r.AspectRatio = metadataString(meta, "ratio")
	}
	if v, ok := metadataInt(meta, "fps"); ok {
		r.FPS = lo.ToPtr(dto.IntValue(v))
	}
	if v, ok := metadataInt(meta, "seed"); ok {
		r.Seed = lo.ToPtr(dto.IntValue(v))
	}

	// duration：上游要 "5s" 这样的字符串
	seconds := 0
	if s, _ := strconv.Atoi(req.Seconds); s > 0 {
		seconds = s
	} else if req.Duration > 0 {
		seconds = req.Duration
	} else if v, ok := metadataInt(meta, "duration"); ok {
		seconds = v
	}
	if seconds > 0 {
		r.Duration = fmt.Sprintf("%ds", seconds)
	}

	// 参考图：首帧优先，其次多图里的第一张
	if req.Image != "" {
		r.ImageURL = req.Image
	} else if len(req.Images) > 0 {
		r.ImageURL = req.Images[0]
	} else {
		r.ImageURL = metadataString(meta, "image_url")
	}

	// 无 content 时走扁平模式（仅 image_url 单图参考）。
	return r
}

// convertToStelloriaNativePayload 把统一的任务请求转换成 Stelloria 原生协议请求体。
//
// 原生协议透传 content 数组，支持首帧/尾帧/多参考图/参考视频/参考音频。
// duration 是整数秒（不是字符串），宽高比字段叫 ratio（不是 aspect_ratio）。
func (a *TaskAdaptor) convertToStelloriaNativePayload(req *relaycommon.TaskSubmitReq) *stelloriaNativePayload {
	meta := req.Metadata

	// content 原样透传
	contentRaw, _ := common.Marshal(meta["content"])

	r := &stelloriaNativePayload{
		Model:   req.Model,
		Content: contentRaw,
	}

	r.Resolution = metadataString(meta, "resolution")

	// 宽高比：原生协议用 ratio
	r.Ratio = metadataString(meta, "ratio")
	if r.Ratio == "" {
		r.Ratio = metadataString(meta, "aspect_ratio")
	}

	// duration：原生协议用整数秒
	if v, ok := metadataInt(meta, "duration"); ok {
		r.Duration = v
	} else if req.Duration > 0 {
		r.Duration = req.Duration
	} else if s, _ := strconv.Atoi(req.Seconds); s > 0 {
		r.Duration = s
	}

	// 布尔参数透传
	if v, ok := metadataBool(meta, "generate_audio"); ok {
		r.GenerateAudio = lo.ToPtr(v)
	}
	if v, ok := metadataBool(meta, "watermark"); ok {
		r.Watermark = lo.ToPtr(v)
	}
	if v, ok := metadataInt(meta, "seed"); ok {
		r.Seed = lo.ToPtr(dto.IntValue(v))
	}

	return r
}

func metadataString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key].(string); ok {
		return v
	}
	return ""
}

func metadataInt(meta map[string]interface{}, key string) (int, bool) {
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

func metadataBool(meta map[string]interface{}, key string) (bool, bool) {
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

// parseStelloriaTaskResult 解析 Stelloria 的查询响应。
// 状态取值是 processing / completed / failed，视频地址在 result.video_url。
func parseStelloriaTaskResult(resp *stelloriaTaskResponse) *relaycommon.TaskInfo {
	taskResult := relaycommon.TaskInfo{Code: 0}

	switch strings.ToLower(resp.Status) {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "completed", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = stelloriaVideoURL(resp)
		if detail, ok := parseStelloriaResultDetail(resp); ok {
			taskResult.CompletionTokens = detail.Usage.CompletionTokens
			taskResult.TotalTokens = detail.Usage.TotalTokens
		}
	case "failed", "error", "cancelled", "canceled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = stelloriaErrorMessage(resp)
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// 先尝试官方 Ark 格式
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err == nil && resTask.ID != "" {
		return parseArkTaskResult(&resTask)
	}

	// 再尝试 Stelloria 格式：顶层 task_id + status，视频地址在 result.video_url。
	// 按响应形态识别而不是按 baseURL 判断，这样轮询与单测都不依赖适配器状态。
	var stelloriaResp stelloriaTaskResponse
	if err := common.Unmarshal(respBody, &stelloriaResp); err == nil &&
		stelloriaResp.TaskID != "" && stelloriaResp.Status != "" {
		return parseStelloriaTaskResult(&stelloriaResp), nil
	}

	// 再尝试 kkidc 格式
	var kkidcResp kkidcQueryResponse
	if err := common.Unmarshal(respBody, &kkidcResp); err == nil && kkidcResp.Code == "success" && kkidcResp.Data.ID != "" {
		return parseKKIDCTaskResult(&kkidcResp.Data)
	}

	return nil, errors.New("unmarshal task result failed, unknown format")
}

// extractVideoURL 从 content 字段中取出视频地址，兼容三种已知形态：
//
//  1. 火山原生 Ark:  {"video_url": "https://..."}
//  2. seegen.ai:     [{"type":"video_url","video_url":{"url":"https://..."}}]
//  3. 对象嵌套形态:  {"video_url": {"url": "https://..."}}
//
// 解析不出来时返回空串，由调用方按"任务未完成"处理，而不是让整个响应解析失败。
func extractVideoURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// 形态 2：数组
	var items []map[string]any
	if err := common.Unmarshal(raw, &items); err == nil {
		for _, item := range items {
			if url := videoURLFromMap(item); url != "" {
				return url
			}
		}
		return ""
	}

	// 形态 1 / 3：对象
	var obj map[string]any
	if err := common.Unmarshal(raw, &obj); err == nil {
		return videoURLFromMap(obj)
	}

	return ""
}

// videoURLFromMap 从单个 content 元素里取 video_url，值可能是字符串或 {"url": "..."}。
func videoURLFromMap(item map[string]any) string {
	if item == nil {
		return ""
	}
	for _, key := range []string{"video_url", "url"} {
		switch v := item[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]any:
			if nested, ok := v["url"].(string); ok && nested != "" {
				return nested
			}
		}
	}
	return ""
}

func parseArkTaskResult(resTask *responseTask) (*relaycommon.TaskInfo, error) {
	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = extractVideoURL(resTask.Content)
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed", "cancelled", "canceled", "expired":
		// cancelled / expired 是终态，不能落到 default 当作"还在跑"，
		// 否则任务会被无限轮询、配额也永远不结算。
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
		if taskResult.Reason == "" {
			taskResult.Reason = "task " + resTask.Status
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func parseKKIDCTaskResult(data *kkidcTaskData) (*relaycommon.TaskInfo, error) {
	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch data.Status {
	case "PENDING", "QUEUED":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "IN_PROGRESS", "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "SUCCESS":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		if data.ResultURL != "" {
			taskResult.Url = data.ResultURL
		} else if data.Content.VideoURL != "" {
			taskResult.Url = data.Content.VideoURL
		}
		taskResult.CompletionTokens = data.Usage.CompletionTokens
		taskResult.TotalTokens = data.Usage.TotalTokens
	case "FAILED", "FAILURE":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = data.FailReason
	default:
		taskResult.Status = model.TaskStatusInProgress
		if data.Progress != "" {
			taskResult.Progress = data.Progress
		} else {
			taskResult.Progress = "30%"
		}
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	// 先尝试官方 Ark 格式
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err == nil && dResp.ID != "" {
		openAIVideo.SetMetadata("url", extractVideoURL(dResp.Content))
		if dResp.Status == "failed" {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: dResp.Error.Message,
				Code:    dResp.Error.Code,
			}
		}
		return common.Marshal(openAIVideo)
	}

	// 再尝试 Stelloria 格式
	var stelloriaResp stelloriaTaskResponse
	if err := common.Unmarshal(originTask.Data, &stelloriaResp); err == nil &&
		stelloriaResp.TaskID != "" && stelloriaResp.Status != "" {
		if url := stelloriaVideoURL(&stelloriaResp); url != "" {
			openAIVideo.SetMetadata("url", url)
		}
		if strings.EqualFold(stelloriaResp.Status, "failed") {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: stelloriaErrorMessage(&stelloriaResp),
				Code:    "upstream_error",
			}
		}
		return common.Marshal(openAIVideo)
	}

	// 再尝试 kkidc 格式
	var kkidcResp kkidcQueryResponse
	if err := common.Unmarshal(originTask.Data, &kkidcResp); err == nil && kkidcResp.Code == "success" {
		if kkidcResp.Data.ResultURL != "" {
			openAIVideo.SetMetadata("url", kkidcResp.Data.ResultURL)
		} else if kkidcResp.Data.Content.VideoURL != "" {
			openAIVideo.SetMetadata("url", kkidcResp.Data.Content.VideoURL)
		}
		if kkidcResp.Data.Status == "FAILED" || kkidcResp.Data.Status == "FAILURE" {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: kkidcResp.Data.FailReason,
				Code:    "upstream_error",
			}
		}
		return common.Marshal(openAIVideo)
	}

	// 兜底：如果都解析不了，至少返回基本结构
	return common.Marshal(openAIVideo)
}
