package middleware

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// assetRefPattern 匹配请求体中出现的 asset://<officialId> 引用。
//
// 对整个请求体字符串做扫描，而不是按已知字段路径遍历 content[].image_url.url
// 之类的结构——素材引用可能出现在 metadata 的任意嵌套位置，宁可多匹配不可漏匹配。
var assetRefPattern = regexp.MustCompile(`asset://([A-Za-z0-9\-_]+)`)

type assetRefModelRequest struct {
	Model string `json:"model"`
}

// AssetRefCheck 校验生成任务中引用的 asset:// 素材。
//
// 单渠道模式下不需要锁渠道（素材与生成任务必然落在同一个上游账号），
// 本中间件只做三类必然失败请求的前置拦截，让用户拿到明确错误而不是上游的一句
// "invalid asset"：
//
//  1. 引用了不存在或不属于自己的素材 -> asset_not_found
//  2. 引用了审核未通过的素材        -> asset_not_active
//  3. cn 素材配 intl 模型（或反之） -> asset_region_mismatch
//
// 请求体中不含 asset:// 时立即放行，对现有纯 URL 用法零影响。
func AssetRefCheck() func(c *gin.Context) {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		storage, err := common.GetBodyStorage(c)
		if err != nil {
			// 读不到请求体时交给后续 handler 去报错，这里不拦截
			c.Next()
			return
		}
		body, err := storage.Bytes()
		if err != nil || len(body) == 0 {
			c.Next()
			return
		}

		officialIds := extractAssetRefs(body)
		if len(officialIds) == 0 {
			c.Next()
			return
		}

		userId := c.GetInt("id")
		if userId == 0 {
			c.Next()
			return
		}

		assets, err := model.GetAssetsByOfficialIds(userId, officialIds)
		if err != nil {
			abortWithAssetError(c, http.StatusInternalServerError, "server_error",
				"failed to verify asset references: "+err.Error())
			return
		}

		assetMap := make(map[string]*model.Asset, len(assets))
		for _, a := range assets {
			assetMap[a.OfficialId] = a
		}

		// 1. 存在性与归属
		var missing []string
		for _, id := range officialIds {
			if _, ok := assetMap[id]; !ok {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			abortWithAssetError(c, http.StatusNotFound, service.AssetErrNotFound,
				fmt.Sprintf("asset not found or not owned by you: %s", strings.Join(missing, ", ")))
			return
		}

		// 2. 审核状态
		var notActive []string
		for _, id := range officialIds {
			asset := assetMap[id]
			if asset.Status != model.AssetStatusActive {
				notActive = append(notActive, fmt.Sprintf("%s(%s)", id, asset.Status))
			}
		}
		if len(notActive) > 0 {
			abortWithAssetError(c, http.StatusBadRequest, service.AssetErrNotActive,
				fmt.Sprintf("asset is not ready for use, only Active assets can be referenced: %s",
					strings.Join(notActive, ", ")))
			return
		}

		// 3. 区域一致性
		var modelReq assetRefModelRequest
		if err := common.UnmarshalBodyReusable(c, &modelReq); err == nil {
			modelRegion := constant.GetSeedanceModelRegion(modelReq.Model)
			// 未知模型不做区域校验，避免误伤模型重定向场景
			if modelRegion != "" {
				var mismatched []string
				for _, id := range officialIds {
					assetRegion := assetMap[id].Region
					// 本地没有记录到 region 时跳过，交给上游判定
					if assetRegion != "" && assetRegion != modelRegion {
						mismatched = append(mismatched, fmt.Sprintf("%s(%s)", id, assetRegion))
					}
				}
				if len(mismatched) > 0 {
					abortWithAssetError(c, http.StatusBadRequest, service.AssetErrRegionMismatch,
						fmt.Sprintf("model %s requires region=%s assets, but got: %s",
							modelReq.Model, modelRegion, strings.Join(mismatched, ", ")))
					return
				}
			}
		}

		c.Next()
	}
}

// extractAssetRefs 提取请求体中全部去重后的 officialId，顺序稳定便于测试与日志。
func extractAssetRefs(body []byte) []string {
	matches := assetRefPattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id := string(m[1])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func abortWithAssetError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
	c.Abort()
}
