package yike

import "encoding/json"

// ============================
// 提交任务
// ============================

// Media 是 Input.Medias 的元素：登记后的素材引用。
//
// Wonder 系列不接受外部 URL，参考素材必须先经 ImportMedia 登记、
// GetMedia 轮询到 ThirdPartyAssetStatus=Success，再以 MediaId 传入。
type Media struct {
	Type    string `json:"Type"` // image / video / audio
	MediaID string `json:"MediaId"`
}

// jobInput 是 SubmitVideoGenerationJob 的 input 字段**反序列化后**的形态。
//
// 注意上游要的是这个结构**序列化成 JSON 字符串**后再放进 input，
// 不是嵌套对象。直接传对象上游会拒。
type jobInput struct {
	Prompt string  `json:"Prompt"`
	Medias []Media `json:"Medias,omitempty"`
}

// jobParameters 同样是序列化成字符串后放进 jobParameters 字段。
type jobParameters struct {
	// EnableAudio 用指针：客户端显式传 false 时必须下发，
	// 非指针 + omitempty 会把 false 静默吞掉（CLAUDE.md Rule 6）。
	EnableAudio *bool `json:"EnableAudio,omitempty"`
}

// submitRequest 是 SubmitVideoGenerationJob 的业务参数。
//
// 几个格式陷阱，每个都能让上游直接拒：
//   - Duration 是**字符串**（"10"），不是整数
//   - Resolution 是**大写 P**（"720P"），不是 "720p"
//   - 宽高比字段叫 AspectRatio，不是 Ratio
//   - Input / JobParameters 是**序列化后的 JSON 字符串**
//   - 上游**不接受** watermark 参数
type submitRequest struct {
	Model         string `json:"Model"`
	JobType       string `json:"JobType"`
	Scene         string `json:"Scene,omitempty"`
	Input         string `json:"Input"`
	JobParameters string `json:"JobParameters,omitempty"`
	Resolution    string `json:"Resolution,omitempty"`
	AspectRatio   string `json:"AspectRatio,omitempty"`
	Duration      string `json:"Duration,omitempty"`
	N             int    `json:"N,omitempty"`
	ClientToken   string `json:"ClientToken,omitempty"`
}

// submitResponse 是提交响应：{"RequestId":"...","JobId":"ag_xxx"}
type submitResponse struct {
	RequestID string `json:"RequestId"`
	JobID     string `json:"JobId"`
}

// ============================
// 查询任务
// ============================

// jobDetail 是 GetVideoGenerationJob 返回的任务体。
//
// Output 是 JSON **字符串**，需要二次解析才能拿到 Medias[].OutputUrl。
type jobDetail struct {
	JobID        string `json:"JobId"`
	Status       string `json:"Status"`
	Output       string `json:"Output"`
	ErrorMessage string `json:"ErrorMessage"`
	ErrorCode    string `json:"ErrorCode"`
	Model        string `json:"Model"`
	JobType      string `json:"JobType"`
}

// getJobResponse 是 GetVideoGenerationJob 的完整响应。
//
// 字段名大小写在阿里云 RPC 响应里偶有出入（VideoGenerationJob / videoGenerationJob），
// 因此解析时两种都试，见 parseJobDetail。
type getJobResponse struct {
	RequestID          string          `json:"RequestId"`
	VideoGenerationJob json.RawMessage `json:"VideoGenerationJob"`
}

// jobOutput 是 jobDetail.Output 反序列化后的形态。
type jobOutput struct {
	Medias []struct {
		OutputURL string `json:"OutputUrl"`
		MediaID   string `json:"MediaId"`
		Type      string `json:"Type"`
	} `json:"Medias"`
}

// 任务状态取值（与豆包/润元完全不同，不能套用）。
const (
	StatusCreated   = "Created"
	StatusQueuing   = "Queuing"
	StatusExecuting = "Executing"
	StatusFinished  = "Finished"
	StatusFailed    = "Failed"
)

// ============================
// 计费
// ============================

// jobCreditResponse 是 GetYikeJobCredit 的响应。
//
// 上游给出的是这个任务实际消耗的额度，比按 token 反推准确得多。
// 字段名待联调核对 —— 参考资料里只提到了这个 Action 的用途，没给响应样例，
// 所以这里用 RawMessage 兜住，解析失败时退回按倍率计费而不是让流程失败。
type jobCreditResponse struct {
	RequestID string          `json:"RequestId"`
	Credit    json.RawMessage `json:"Credit"`
}

// ============================
// 素材登记
// ============================

// importMediaRequest 是 ImportMedia 的业务参数。
type importMediaRequest struct {
	ImportSource   string `json:"ImportSource"` // 固定 "url"
	InputURL       string `json:"InputURL"`
	MediaType      string `json:"MediaType"` // image / video / audio
	Title          string `json:"Title,omitempty"`
	RegisterConfig string `json:"RegisterConfig,omitempty"`
}

type importMediaResponse struct {
	RequestID string `json:"RequestId"`
	MediaID   string `json:"MediaId"`
}

// getMediaResponse 是 GetMedia 的响应。
//
// 关键字段是 ThirdPartyAssetStatus —— 必须等到 Success，
// 素材才能被 SubmitVideoGenerationJob 引用。
type getMediaResponse struct {
	RequestID string          `json:"RequestId"`
	Media     json.RawMessage `json:"Media"`
}

type mediaDetail struct {
	MediaID                 string `json:"MediaId"`
	Title                   string `json:"Title"`
	MediaType               string `json:"MediaType"`
	Status                  string `json:"Status"`
	ThirdPartyAssetStatus   string `json:"ThirdPartyAssetStatus"`
	URL                     string `json:"Url"`
	ErrorMessage            string `json:"ErrorMessage"`
	ThirdPartyAssetErrorMsg string `json:"ThirdPartyAssetErrorMessage"`
}

// ThirdPartyAssetStatus 取值。
const (
	AssetStatusSuccess = "Success"
	AssetStatusFailed  = "Failed"
)

// ============================
// 错误
// ============================

// errorResponse 是阿里云 RPC 的标准错误体。
type errorResponse struct {
	RequestID string `json:"RequestId"`
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	Recommend string `json:"Recommend"`
	HostID    string `json:"HostId"`
}
