package ratio_setting

import (
	"github.com/QuantumNous/new-api/types"
)

// ImageSizePrice stores per-model, per-resolution fixed prices.
// Structure: {"model_name": {"size_key": price_usd, ...}, ...}
// Example: {"gemini-3-pro-image-preview": {"default": 0.1973, "4K": 0.3548}}
var imageSizePriceMap = types.NewRWMap[string, map[string]float64]()

// ImageSizePrice2JSONString serializes the current map to JSON.
func ImageSizePrice2JSONString() string {
	return imageSizePriceMap.MarshalJSONString()
}

// UpdateImageSizePriceByJSONString replaces the map from a JSON string.
func UpdateImageSizePriceByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(imageSizePriceMap, jsonStr)
}

// GetImageSizePrice returns the USD price for a specific model + image size.
// If the exact size is not found, it falls back to "default" under the same model.
// Returns (price, true) if found, otherwise (0, false).
func GetImageSizePrice(modelName, imageSize string) (float64, bool) {
	modelSizes, ok := imageSizePriceMap.Get(modelName)
	if !ok || modelSizes == nil {
		return 0, false
	}
	if price, ok := modelSizes[imageSize]; ok {
		return price, true
	}
	if price, ok := modelSizes["default"]; ok {
		return price, true
	}
	return 0, false
}

// GetImageSizePriceCopy returns a deep copy of the entire map.
func GetImageSizePriceCopy() map[string]map[string]float64 {
	return imageSizePriceMap.ReadAll()
}
