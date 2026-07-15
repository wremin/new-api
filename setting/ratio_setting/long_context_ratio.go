package ratio_setting

import (
	"github.com/QuantumNous/new-api/types"
)

const (
	DefaultLongContextThresholdTokens = 272 * 1024 // 272k tokens
)

var (
	defaultLongContextThreshold = DefaultLongContextThresholdTokens

	longContextModelRatioMap           = types.NewRWMap[string, float64]()
	longContextCompletionRatioMap      = types.NewRWMap[string, float64]()
	longContextCacheRatioMap           = types.NewRWMap[string, float64]()
	longContextCreateCacheRatioMap     = types.NewRWMap[string, float64]()
	longContextThreshold               = DefaultLongContextThresholdTokens
)

func InitLongContextRatioSettings() {
	// 默认阈值为 272k tokens，可通过 option 覆盖
	longContextThreshold = DefaultLongContextThresholdTokens
	longContextModelRatioMap.Clear()
	longContextCompletionRatioMap.Clear()
	longContextCacheRatioMap.Clear()
	longContextCreateCacheRatioMap.Clear()
}

// GetLongContextThreshold 返回当前长文本阈值（tokens）
func GetLongContextThreshold() int {
	return longContextThreshold
}

// SetLongContextThreshold 设置长文本阈值（tokens）
func SetLongContextThreshold(threshold int) {
	if threshold <= 0 {
		longContextThreshold = 0
		return
	}
	longContextThreshold = threshold
}

func DefaultLongContextThreshold() int {
	return DefaultLongContextThresholdTokens
}

// LongContextModelRatio
func LongContextModelRatio2JSONString() string {
	return longContextModelRatioMap.MarshalJSONString()
}

func UpdateLongContextModelRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(longContextModelRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetLongContextModelRatio(name string) (float64, bool) {
	name = FormatMatchingModelName(name)
	ratio, ok := longContextModelRatioMap.Get(name)
	if !ok {
		return 0, false
	}
	return ratio, true
}

func GetLongContextModelRatioCopy() map[string]float64 {
	return longContextModelRatioMap.ReadAll()
}

// LongContextCompletionRatio
func LongContextCompletionRatio2JSONString() string {
	return longContextCompletionRatioMap.MarshalJSONString()
}

func UpdateLongContextCompletionRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(longContextCompletionRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetLongContextCompletionRatio(name string) (float64, bool) {
	name = FormatMatchingModelName(name)
	ratio, ok := longContextCompletionRatioMap.Get(name)
	if !ok {
		return 0, false
	}
	return ratio, true
}

func GetLongContextCompletionRatioCopy() map[string]float64 {
	return longContextCompletionRatioMap.ReadAll()
}

// LongContextCacheRatio
func LongContextCacheRatio2JSONString() string {
	return longContextCacheRatioMap.MarshalJSONString()
}

func UpdateLongContextCacheRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(longContextCacheRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetLongContextCacheRatio(name string) (float64, bool) {
	name = FormatMatchingModelName(name)
	ratio, ok := longContextCacheRatioMap.Get(name)
	if !ok {
		return 0, false
	}
	return ratio, true
}

func GetLongContextCacheRatioCopy() map[string]float64 {
	return longContextCacheRatioMap.ReadAll()
}

// LongContextCreateCacheRatio
func LongContextCreateCacheRatio2JSONString() string {
	return longContextCreateCacheRatioMap.MarshalJSONString()
}

func UpdateLongContextCreateCacheRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(longContextCreateCacheRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetLongContextCreateCacheRatio(name string) (float64, bool) {
	name = FormatMatchingModelName(name)
	ratio, ok := longContextCreateCacheRatioMap.Get(name)
	if !ok {
		return 0, false
	}
	return ratio, true
}

func GetLongContextCreateCacheRatioCopy() map[string]float64 {
	return longContextCreateCacheRatioMap.ReadAll()
}
