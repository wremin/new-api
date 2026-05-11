package doubao

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

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
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
	Resolution      string         `json:"resolution,omitempty"`
	Ratio           string         `json:"ratio,omitempty"`
	Duration        *dto.IntValue  `json:"duration,omitempty"`
	Frames          *dto.IntValue  `json:"frames,omitempty"`
	Seed            *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed     *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark       *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"` // task_id
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
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
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

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	// kkidc 使用 OpenAI 兼容路径 /v1/video/generations
	if strings.Contains(a.baseURL, "kkidc") {
		return fmt.Sprintf("%s/v1/video/generations", a.baseURL), nil
	}
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
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// extractUpstreamErrorMsg 尝试从各种上游错误格式中提取可读错误信息
func extractUpstreamErrorMsg(body []byte) string {
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

	// 再尝试 kkidc 格式
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
	if strings.Contains(baseUrl, "kkidc") {
		uri = fmt.Sprintf("%s/v1/video/generations/%s", baseUrl, taskID)
	} else {
		uri = fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	isKKIDC := strings.Contains(a.baseURL, "kkidc")

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
	}

	// 顶层 prompt（kkidc 需要）
	r.Prompt = req.Prompt

	// 官方 Ark 用 content 数组
	if !isKKIDC {
		r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
		r.Content = append(r.Content, ContentItem{
			Type: "text",
			Text: req.Prompt,
		})
	}

	// 兼容 kkidc 格式：metadata 中放 reference_* 和其他参数
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

	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	// 先尝试官方 Ark 格式
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err == nil && resTask.ID != "" {
		return parseArkTaskResult(&resTask)
	}

	// 再尝试 kkidc 格式
	var kkidcResp kkidcQueryResponse
	if err := common.Unmarshal(respBody, &kkidcResp); err == nil && kkidcResp.Code == "success" && kkidcResp.Data.ID != "" {
		return parseKKIDCTaskResult(&kkidcResp.Data)
	}

	return nil, errors.New("unmarshal task result failed, unknown format")
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
		taskResult.Url = resTask.Content.VideoURL
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
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
		openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
		if dResp.Status == "failed" {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: dResp.Error.Message,
				Code:    dResp.Error.Code,
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
