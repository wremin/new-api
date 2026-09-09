package volcengine

// BytePlus Seed Speech（Seed-Audio 系列）适配。
//
// 上游接口：POST {voice-host}/api/v3/tts/create
//   - 鉴权：X-Api-Key 头（渠道密钥直接填 Seed Speech 的 API Key，不是 appid|token 格式）
//   - 请求：model + text_prompt + audio_config，同步返回 JSON（base64 音频或临时 URL）
//   - 默认域名 voice.ap-southeast-1.bytepluses.com；渠道 base_url 填 voice.* 域名时以渠道配置为准
//
// 对下游暴露 OpenAI /v1/audio/speech 格式：
//   - input        -> 待合成文本
//   - instructions -> 音色/语气/场景的自然语言描述，与 input 拼成 text_prompt
//   - response_format / speed 分别映射 audio_config 的 format / speech_rate
//   - metadata     -> 原样合并进上游请求体，可全量控制 text_prompt、audio_config 等原生字段

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const defaultSeedSpeechBaseURL = "https://voice.ap-southeast-1.bytepluses.com"

// isSeedAudioModel 判断是否为 Seed Speech 音频模型（如 seed-audio-1.0）
func isSeedAudioModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "seed-audio")
}

func isSeedAudioRelay(info *relaycommon.RelayInfo) bool {
	return isSeedAudioModel(info.UpstreamModelName) || isSeedAudioModel(info.OriginModelName)
}

// seedSpeechRequestURL 渠道 base_url 为 voice.* 域名时使用渠道配置，否则退到默认 Seed Speech 域名。
// （同一渠道的 base_url 通常填 ark 域名给图像/对话模型用，Seed Speech 是独立域名。）
func seedSpeechRequestURL(baseUrl string) string {
	if u, err := url.Parse(baseUrl); err == nil && strings.HasPrefix(u.Host, "voice.") {
		return strings.TrimSuffix(baseUrl, "/") + "/api/v3/tts/create"
	}
	return defaultSeedSpeechBaseURL + "/api/v3/tts/create"
}

type SeedSpeechAudioConfig struct {
	Format         string `json:"format,omitempty"`          // mp3 / wav / pcm / ogg_opus
	SampleRate     int    `json:"sample_rate,omitempty"`     // 默认 24000
	SpeechRate     *int   `json:"speech_rate,omitempty"`     // 语速偏移百分比 [-50,100]，0 为正常
	LoudnessRate   *int   `json:"loudness_rate,omitempty"`   // 音量偏移
	PitchRate      *int   `json:"pitch_rate,omitempty"`      // 音调偏移
	EnableSubtitle *bool  `json:"enable_subtitle,omitempty"` // 是否返回字幕/时间戳
}

type SeedSpeechRequest struct {
	Model       string                 `json:"model"`
	TextPrompt  string                 `json:"text_prompt"`
	AudioConfig *SeedSpeechAudioConfig `json:"audio_config,omitempty"`
	Watermark   map[string]any         `json:"watermark,omitempty"`
}

type SeedSpeechResponse struct {
	Code    *int   `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Audio   string `json:"audio,omitempty"` // base64 音频
	URL     string `json:"url,omitempty"`   // 临时下载地址（约 2 小时有效）
}

func seedSpeechCodeOK(code *int) bool {
	if code == nil {
		return true
	}
	switch *code {
	case 0, 200, 20000000:
		return true
	}
	return false
}

// convertSeedSpeechRequest 把 OpenAI audio/speech 请求转换为 Seed Speech 原生请求
func convertSeedSpeechRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	format := mapEncoding(request.ResponseFormat) // 复用 mp3/wav/pcm/ogg_opus 映射
	c.Set(contextKeyResponseFormat, format)

	textPrompt := request.Input
	if request.Instructions != "" {
		// instructions 描述音色/语气/场景，与合成文本拼成 Seed Speech 的自然语言 text_prompt，
		// 例如「一位温柔的女声，用平静的语气说：“……”」
		textPrompt = fmt.Sprintf("%s说：“%s”", strings.TrimRight(request.Instructions, "说：:"), request.Input)
	}

	upstreamModel := info.UpstreamModelName
	if upstreamModel == "" {
		upstreamModel = info.OriginModelName
	}

	seedReq := SeedSpeechRequest{
		Model:      upstreamModel,
		TextPrompt: textPrompt,
		AudioConfig: &SeedSpeechAudioConfig{
			Format:     format,
			SampleRate: 24000,
		},
	}

	if request.Speed != nil {
		// OpenAI speed 0.25~4.0（1 为正常）-> speech_rate 百分比偏移，钳制到 [-50,100]
		rate := int((*request.Speed - 1) * 100)
		if rate < -50 {
			rate = -50
		}
		if rate > 100 {
			rate = 100
		}
		seedReq.AudioConfig.SpeechRate = &rate
	}

	// metadata 原样覆盖上游原生字段（与万相/老版火山 TTS 的 metadata 语义一致）
	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &seedReq); err != nil {
			return nil, fmt.Errorf("error unmarshalling metadata to seed speech request: %w", err)
		}
	}

	jsonData, err := common.Marshal(seedReq)
	if err != nil {
		return nil, fmt.Errorf("error marshalling seed speech request: %w", err)
	}
	return bytes.NewReader(jsonData), nil
}

// handleSeedSpeechResponse 解析 Seed Speech 同步响应，把音频以二进制流返回给下游
func handleSeedSpeechResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read seed speech response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	logID := resp.Header.Get("X-Tt-Logid")
	if resp.StatusCode != http.StatusOK {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("seed speech upstream returned status %d (logid: %s): %s", resp.StatusCode, logID, truncateForError(body)),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}

	var seedResp SeedSpeechResponse
	if unmarshalErr := common.Unmarshal(body, &seedResp); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to parse seed speech response"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}

	if !seedSpeechCodeOK(seedResp.Code) {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("seed speech error code=%d message=%s (logid: %s)", *seedResp.Code, seedResp.Message, logID),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}

	var audioData []byte
	switch {
	case seedResp.Audio != "":
		decoded, decodeErr := base64.StdEncoding.DecodeString(seedResp.Audio)
		if decodeErr != nil {
			decoded, decodeErr = base64.RawStdEncoding.DecodeString(seedResp.Audio)
		}
		if decodeErr != nil {
			return nil, types.NewErrorWithStatusCode(
				errors.New("failed to decode seed speech audio data"),
				types.ErrorCodeBadResponseBody,
				http.StatusInternalServerError,
			)
		}
		audioData = decoded
	case seedResp.URL != "":
		downloaded, dlErr := downloadSeedSpeechAudio(seedResp.URL)
		if dlErr != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to download seed speech audio: %w", dlErr),
				types.ErrorCodeBadResponse,
				http.StatusBadGateway,
			)
		}
		audioData = downloaded
	default:
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("seed speech response contains no audio (logid: %s)", logID),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
		)
	}

	contentType := getContentTypeByEncoding(c.GetString(contextKeyResponseFormat))
	c.Data(http.StatusOK, contentType, audioData)

	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}
	return usage, nil
}

func downloadSeedSpeechAudio(audioURL string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(audioURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audio url returned status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func truncateForError(body []byte) string {
	const maxLen = 512
	s := string(body)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
