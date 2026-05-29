package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

func SeedanceRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 只对 POST 请求做 body 转换，GET 等请求直接放行
		if c.Request.Method != "POST" {
			c.Next()
			return
		}

		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// 如果原始请求已经有 metadata 字段（已包装过），直接放行避免重复包装
		if _, hasMetadata := originalReq["metadata"]; hasMetadata {
			c.Next()
			return
		}

		// Support both model_name and model fields
		model, _ := originalReq["model_name"].(string)
		if model == "" {
			model, _ = originalReq["model"].(string)
		}
		prompt, _ := originalReq["prompt"].(string)

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// 清除 BodyStorage 缓存，确保后续 handler 读到包装后的请求体
		c.Set(common.KeyBodyStorage, nil)

		// Rewrite request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		// We have to reset the request body for the next handlers
		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
	}
}
