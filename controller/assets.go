package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// assetError 返回 OpenAI 风格的错误体，与 relay 侧其他接口保持一致。
func assetError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
	c.Abort()
}

func assetServiceError(c *gin.Context, err *service.AssetsError) {
	status := err.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	assetError(c, status, err.Code, err.Message)
}

// resolveAssetsChannel 取本期唯一的素材渠道，失败时已写入响应。
func resolveAssetsChannel(c *gin.Context) (*model.Channel, bool) {
	channel, aErr := service.GetAssetsChannel()
	if aErr != nil {
		assetServiceError(c, aErr)
		return nil, false
	}
	return channel, true
}

// checkAssetWritePermission 校验写入类操作的前置条件。
// 素材接口本身不计费，但欠费/禁用用户不应继续占用上游素材配额。
func checkAssetWritePermission(c *gin.Context, newItems int) bool {
	// 会话鉴权（控制台页面）不会写入额度上下文，此时跳过额度校验：
	// authHelper 已经保证了用户存在且未被封禁。
	// 只有 token 鉴权（API 客户端）才带 user_quota，这里才做欠费拦截。
	if quotaVal, ok := common.GetContextKey(c, constant.ContextKeyUserQuota); ok {
		if !common.GetContextKeyBool(c, constant.ContextKeyTokenUnlimited) && assetQuotaExhausted(quotaVal) {
			assetError(c, http.StatusForbidden, "insufficient_quota",
				"user quota is exhausted, asset upload is not allowed")
			return false
		}
	}

	maxTotal := operation_setting.GetAssetsUserMaxTotal()
	if maxTotal > 0 {
		userId := c.GetInt("id")
		total, err := model.CountUserAssets(userId)
		if err == nil && total+int64(newItems) > int64(maxTotal) {
			assetError(c, http.StatusForbidden, service.AssetErrQuotaExceeded,
				fmt.Sprintf("asset count limit reached (%d/%d)", total, maxTotal))
			return false
		}
	}
	return true
}

// assetQuotaExhausted 兼容额度在上下文中可能是 int / int64 的两种情况。
func assetQuotaExhausted(quotaVal any) bool {
	switch v := quotaVal.(type) {
	case int:
		return v <= 0
	case int64:
		return v <= 0
	default:
		return false
	}
}

func assetRequestUser(c *gin.Context) (userId int, tokenId int) {
	return c.GetInt("id"), c.GetInt("token_id")
}

// isAssetAdminView 判断本次请求是否以管理员视角查询全部数据。
func isAssetAdminView(c *gin.Context) bool {
	if c.Query("all") != "true" {
		return false
	}
	return model.IsAdmin(c.GetInt("id"))
}

func assetVerbose(c *gin.Context) bool {
	return c.Query("verbose") == "true" && model.IsAdmin(c.GetInt("id"))
}

func toAssetItem(a *model.Asset, verbose bool) dto.AssetItemResponse {
	item := dto.AssetItemResponse{
		Id:         a.Id,
		OfficialId: a.OfficialId,
		GroupId:    a.GroupOfficialId,
		Name:       a.Name,
		Status:     a.Status,
		Region:     a.Region,
		AssetType:  a.AssetType,
		Url:        a.SourceUrl,
		AssetRef:   model.BuildAssetRef(a.OfficialId),
		FailReason: a.FailReason,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
	if verbose {
		item.ChannelId = a.ChannelId
	}
	return item
}

// ============================
// POST /v1/assets
// ============================

func RelayAssetCreate(c *gin.Context) {
	channel, ok := resolveAssetsChannel(c)
	if !ok {
		return
	}
	if !checkAssetWritePermission(c, 1) {
		return
	}

	var req dto.AssetCreateRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.GroupId) == "" {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest, "groupId is required")
		return
	}
	if err := service.ValidateAssetSourceURL(req.Url); err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest, err.Error())
		return
	}

	rawBody, err := readAssetRequestBody(c)
	if err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"failed to read request body: "+err.Error())
		return
	}

	resp, aErr := service.DoAssetsUpstreamRequest(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets",
		Body:        rawBody,
		ContentType: "application/json",
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if !resp.IsSuccess() {
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(resp.Body))
		return
	}

	userId, tokenId := assetRequestUser(c)
	// 素材组的 region 用于生成任务的区域一致性校验，本地有记录时带上
	region := ""
	if group, exist, gErr := model.GetAssetGroupByOfficialId(userId, req.GroupId); gErr == nil && exist {
		region = group.Region
	}

	_, err = service.RecordUploadedAsset(userId, tokenId, channel.Id, resp.Body, service.AssetFallbackFields(
		"", req.GroupId, req.GetName(), req.Url, region,
	))
	if err != nil {
		// 落库失败不影响用户拿到上游结果，但必须记日志：
		// 归属记录缺失会导致该素材后续无法被查询/引用。
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to record uploaded asset for user %d: %s", userId, err.Error()))
	}

	writeUpstreamJSON(c, resp.StatusCode, resp.Body)
}

// ============================
// GET /v1/assets（本地查询，不透传）
// ============================

func RelayAssetList(c *gin.Context) {
	userId := c.GetInt("id")

	pageNum, _ := strconv.Atoi(c.DefaultQuery("page_num", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 状态同步是同步的上游 HTTP 调用，默认不在列表接口里做，避免拖慢响应。
	// 前端轮询与用户主动刷新时带 refresh=true，此时才回源同步：
	// 这同时完成审核状态刷新与 Excel 批量上传路径下 name/groupId 等空字段的回填。
	if c.Query("refresh") == "true" {
		if channel, aErr := service.GetAssetsChannel(); aErr == nil {
			service.SyncPendingAssets(c.Request.Context(), channel, userId, 20)
		}
	}

	assets, total, err := model.GetAssets(model.AssetSearchParams{
		UserId:     userId,
		GroupId:    c.Query("groupId"),
		Status:     c.Query("status"),
		AssetType:  c.Query("assetType"),
		Keyword:    c.Query("keyword"),
		PageNum:    pageNum,
		PageSize:   pageSize,
		IncludeAll: isAssetAdminView(c),
	})
	if err != nil {
		assetError(c, http.StatusInternalServerError, "server_error", "failed to query assets: "+err.Error())
		return
	}

	verbose := assetVerbose(c)
	items := make([]dto.AssetItemResponse, 0, len(assets))
	for _, a := range assets {
		items = append(items, toAssetItem(a, verbose))
	}

	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	c.JSON(http.StatusOK, dto.AssetListResponse{
		Items:    items,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	})
}

// ============================
// ANY /v1/assets/*action
// ============================

// RelayAssetDispatch 统一分发 /v1/assets 下的子路径。
//
// 使用 catch-all 而非分别注册 /batch、/groups、/:id，
// 是为了规避 gin 在同层级混用静态段与通配段时可能出现的路由冲突 panic。
func RelayAssetDispatch(c *gin.Context) {
	action := strings.Trim(c.Param("action"), "/")

	switch {
	case action == "batch":
		if c.Request.Method != http.MethodPost {
			assetMethodNotAllowed(c)
			return
		}
		assetBatchUpload(c)
	case action == "batch/template":
		if c.Request.Method != http.MethodGet {
			assetMethodNotAllowed(c)
			return
		}
		assetBatchTemplate(c)
	case action == "groups":
		switch c.Request.Method {
		case http.MethodPost:
			assetGroupCreate(c)
		case http.MethodGet:
			assetGroupList(c)
		default:
			assetMethodNotAllowed(c)
		}
	case action == "":
		assetError(c, http.StatusNotFound, service.AssetErrInvalidRequest, "not found")
	default:
		if strings.Contains(action, "/") {
			assetError(c, http.StatusNotFound, service.AssetErrInvalidRequest, "not found: "+action)
			return
		}
		switch c.Request.Method {
		case http.MethodGet:
			assetGet(c, action)
		case http.MethodDelete:
			assetDelete(c, action)
		default:
			assetMethodNotAllowed(c)
		}
	}
}

func assetMethodNotAllowed(c *gin.Context) {
	assetError(c, http.StatusMethodNotAllowed, service.AssetErrInvalidRequest,
		"method "+c.Request.Method+" is not allowed on this path")
}

// ============================
// 批量上传
// ============================

func assetBatchUpload(c *gin.Context) {
	channel, ok := resolveAssetsChannel(c)
	if !ok {
		return
	}

	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		assetBatchUploadExcel(c, channel, contentType)
		return
	}
	assetBatchUploadJSON(c, channel)
}

// assetBatchUploadJSON 处理 JSON 数组批量上传。
// 上游按 index 回显 officialId，可与请求体数组下标对齐，因此能一次写全归属记录。
func assetBatchUploadJSON(c *gin.Context, channel *model.Channel) {
	var items []dto.AssetCreateRequest
	if err := common.UnmarshalBodyReusable(c, &items); err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"invalid request body, expect a JSON array: "+err.Error())
		return
	}
	if len(items) == 0 {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest, "empty batch")
		return
	}
	maxItems := operation_setting.GetAssetsBatchMaxItems()
	if len(items) > maxItems {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			fmt.Sprintf("batch size %d exceeds limit %d", len(items), maxItems))
		return
	}
	for i, item := range items {
		if strings.TrimSpace(item.GroupId) == "" {
			assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
				fmt.Sprintf("item %d: groupId is required", i))
			return
		}
		if err := service.ValidateAssetSourceURL(item.Url); err != nil {
			assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
				fmt.Sprintf("item %d: %s", i, err.Error()))
			return
		}
	}
	if !checkAssetWritePermission(c, len(items)) {
		return
	}

	rawBody, err := readAssetRequestBody(c)
	if err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"failed to read request body: "+err.Error())
		return
	}

	resp, aErr := service.DoAssetsUpstreamRequest(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets/batch",
		Body:        rawBody,
		ContentType: "application/json",
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if !resp.IsSuccess() {
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(resp.Body))
		return
	}

	var batchResp dto.AssetBatchResponse
	if err := common.Unmarshal(resp.Body, &batchResp); err == nil {
		recordBatchAssets(c, channel, batchResp, items)
	} else {
		logger.LogError(c.Request.Context(),
			"failed to parse batch upload response, asset ownership not recorded: "+err.Error())
	}

	writeUpstreamJSON(c, resp.StatusCode, resp.Body)
}

// recordBatchAssets 按 index 把上游结果与请求体对齐后落库。
// items 为 nil 时（Excel 路径）只写 officialId 等最小字段，其余交给异步回填。
func recordBatchAssets(c *gin.Context, channel *model.Channel, batchResp dto.AssetBatchResponse, items []dto.AssetCreateRequest) {
	userId, tokenId := assetRequestUser(c)

	// 预取素材组 region，避免逐条查库
	regionCache := make(map[string]string)
	lookupRegion := func(groupId string) string {
		if groupId == "" {
			return ""
		}
		if r, ok := regionCache[groupId]; ok {
			return r
		}
		region := ""
		if group, exist, err := model.GetAssetGroupByOfficialId(userId, groupId); err == nil && exist {
			region = group.Region
		}
		regionCache[groupId] = region
		return region
	}

	assets := make([]*model.Asset, 0, len(batchResp.Results))
	for _, result := range batchResp.Results {
		if result.Status != "ok" || result.OfficialId == "" {
			continue
		}
		asset := &model.Asset{
			OfficialId: result.OfficialId,
			UserId:     userId,
			TokenId:    tokenId,
			ChannelId:  channel.Id,
			Status:     model.AssetStatusProcessing,
			BatchId:    batchResp.BatchId,
		}
		// JSON 路径下可以按 index 对齐请求体；Excel 路径下 items 为 nil，字段留空待回填
		if items != nil && result.Index >= 0 && result.Index < len(items) {
			item := items[result.Index]
			asset.GroupOfficialId = item.GroupId
			asset.Name = item.GetName()
			asset.SourceUrl = item.Url
			asset.AssetType = model.GuessAssetType(item.Url)
			asset.Region = lookupRegion(item.GroupId)
		}
		assets = append(assets, asset)
	}

	if err := model.InsertAssetsBatch(assets); err != nil {
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to record %d batch assets for user %d: %s", len(assets), userId, err.Error()))
	}
}

// assetBatchUploadExcel 处理 Excel 批量上传。
//
// 已确认本期不解析 Excel（见 PRD §3.5）：new-api 只能从上游响应拿到 officialId，
// name / groupId / region / url 全部留空，由 SyncPendingAssets 异步回填。
func assetBatchUploadExcel(c *gin.Context, channel *model.Channel, contentType string) {
	maxBytes := operation_setting.GetAssetsUploadMaxBytes()
	if c.Request.ContentLength > maxBytes {
		assetError(c, http.StatusRequestEntityTooLarge, service.AssetErrInvalidRequest,
			fmt.Sprintf("upload size %d exceeds limit %d bytes", c.Request.ContentLength, maxBytes))
		return
	}
	if !checkAssetWritePermission(c, 0) {
		return
	}

	rawBody, err := readAssetRequestBody(c)
	if err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"failed to read request body: "+err.Error())
		return
	}
	if int64(len(rawBody)) > maxBytes {
		assetError(c, http.StatusRequestEntityTooLarge, service.AssetErrInvalidRequest,
			fmt.Sprintf("upload size %d exceeds limit %d bytes", len(rawBody), maxBytes))
		return
	}

	resp, aErr := service.DoAssetsUpstreamRequest(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets/batch",
		RawBody:     bytes.NewReader(rawBody),
		ContentType: contentType,
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if !resp.IsSuccess() {
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(resp.Body))
		return
	}

	var batchResp dto.AssetBatchResponse
	if err := common.Unmarshal(resp.Body, &batchResp); err == nil {
		recordBatchAssets(c, channel, batchResp, nil)
	} else {
		logger.LogError(c.Request.Context(),
			"failed to parse excel batch response, asset ownership not recorded: "+err.Error())
	}

	writeUpstreamJSON(c, resp.StatusCode, resp.Body)
}

// assetBatchTemplate 二进制流式透传 Excel 模板。
func assetBatchTemplate(c *gin.Context) {
	channel, ok := resolveAssetsChannel(c)
	if !ok {
		return
	}

	resp, aErr := service.StreamAssetsUpstream(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method: http.MethodGet,
		Path:   "/v1/assets/batch/template",
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(body))
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	c.Header("Content-Type", contentType)
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		c.Header("Content-Disposition", disposition)
	} else {
		c.Header("Content-Disposition", `attachment; filename="assets-template.xlsx"`)
	}
	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logger.LogError(c.Request.Context(), "failed to stream asset template: "+err.Error())
	}
}

// ============================
// 素材组
// ============================

func assetGroupCreate(c *gin.Context) {
	channel, ok := resolveAssetsChannel(c)
	if !ok {
		return
	}

	var req dto.AssetGroupCreateRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest, "name is required")
		return
	}
	region := strings.ToLower(strings.TrimSpace(req.GetRegion()))
	if region != "" && region != model.AssetRegionCN && region != model.AssetRegionINTL {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			`region must be "cn" or "intl"`)
		return
	}

	rawBody, err := readAssetRequestBody(c)
	if err != nil {
		assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
			"failed to read request body: "+err.Error())
		return
	}

	resp, aErr := service.DoAssetsUpstreamRequest(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method:      http.MethodPost,
		Path:        "/v1/assets/groups",
		Body:        rawBody,
		ContentType: "application/json",
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if !resp.IsSuccess() {
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(resp.Body))
		return
	}

	userId := c.GetInt("id")
	var raw map[string]any
	if err := common.Unmarshal(resp.Body, &raw); err == nil {
		officialId, _ := raw["officialId"].(string)
		if officialId != "" {
			respRegion, _ := raw["region"].(string)
			if respRegion == "" {
				respRegion = region
			}
			if respRegion == "" {
				respRegion = model.AssetRegionCN
			}
			group := &model.AssetGroup{
				OfficialId:  officialId,
				UserId:      userId,
				ChannelId:   channel.Id,
				Name:        req.Name,
				Description: req.GetDescription(),
				Region:      strings.ToLower(respRegion),
			}
			if idVal, ok := raw["id"].(float64); ok {
				group.UpstreamId = int64(idVal)
			}
			if err := group.Insert(); err != nil {
				logger.LogError(c.Request.Context(),
					fmt.Sprintf("failed to record asset group for user %d: %s", userId, err.Error()))
			}
		}
	}

	writeUpstreamJSON(c, resp.StatusCode, resp.Body)
}

// assetGroupList 本地查询，理由同素材列表：
// 全站共用一个上游账号，透传会把其他用户的素材组暴露出去。
func assetGroupList(c *gin.Context) {
	userId := c.GetInt("id")
	groups, err := model.GetAssetGroups(userId, isAssetAdminView(c))
	if err != nil {
		assetError(c, http.StatusInternalServerError, "server_error",
			"failed to query asset groups: "+err.Error())
		return
	}

	verbose := assetVerbose(c)
	items := make([]dto.AssetGroupItemResponse, 0, len(groups))
	for _, g := range groups {
		item := dto.AssetGroupItemResponse{
			Id:          g.Id,
			OfficialId:  g.OfficialId,
			Name:        g.Name,
			Description: g.Description,
			Region:      g.Region,
			Count:       dto.AssetGroupCount{Assets: g.AssetCount},
			CreatedAt:   g.CreatedAt,
		}
		if verbose {
			item.ChannelId = g.ChannelId
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

// ============================
// 单个素材查询 / 删除
// ============================

func assetGet(c *gin.Context, officialId string) {
	userId := c.GetInt("id")

	// 先查本地归属，查不到直接 404 且不打到上游，
	// 否则用户可以通过遍历 officialId 探测他人素材。
	asset, exist, err := model.GetAssetByOfficialId(userId, officialId)
	if err != nil {
		assetError(c, http.StatusInternalServerError, "server_error", "failed to query asset: "+err.Error())
		return
	}
	if !exist {
		assetError(c, http.StatusNotFound, service.AssetErrNotFound,
			"asset not found: "+officialId)
		return
	}

	channel, ok := resolveAssetsChannel(c)
	if !ok {
		return
	}

	resp, aErr := service.DoAssetsUpstreamRequest(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method: http.MethodGet,
		Path:   "/v1/assets/" + url.PathEscape(officialId),
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if !resp.IsSuccess() {
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(resp.Body))
		return
	}

	// 回写本地状态与 Excel 路径下缺失的字段
	if err := service.ApplyUpstreamAssetResponse(asset, resp.Body); err != nil {
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to sync asset %s: %s", officialId, err.Error()))
	}

	writeUpstreamJSON(c, resp.StatusCode, resp.Body)
}

func assetDelete(c *gin.Context, officialId string) {
	userId := c.GetInt("id")

	asset, exist, err := model.GetAssetByOfficialId(userId, officialId)
	if err != nil {
		assetError(c, http.StatusInternalServerError, "server_error", "failed to query asset: "+err.Error())
		return
	}
	if !exist {
		assetError(c, http.StatusNotFound, service.AssetErrNotFound,
			"asset not found: "+officialId)
		return
	}

	channel, ok := resolveAssetsChannel(c)
	if !ok {
		return
	}

	resp, aErr := service.DoAssetsUpstreamRequest(c.Request.Context(), channel, service.AssetsUpstreamRequest{
		Method: http.MethodDelete,
		Path:   "/v1/assets/" + url.PathEscape(officialId),
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	// 上游已经不存在时，本地也应清理掉，视为删除成功
	if !resp.IsSuccess() && resp.StatusCode != http.StatusNotFound {
		assetError(c, resp.StatusCode, service.AssetErrUpstream, service.ExtractUpstreamAssetError(resp.Body))
		return
	}

	if err := asset.SoftDelete(); err != nil {
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to soft delete asset %s: %s", officialId, err.Error()))
	}

	if len(resp.Body) > 0 && resp.IsSuccess() {
		writeUpstreamJSON(c, resp.StatusCode, resp.Body)
		return
	}
	c.JSON(http.StatusOK, gin.H{"officialId": officialId, "deleted": true})
}

// ============================
// 工具函数
// ============================

// readAssetRequestBody 取原始请求体，用于原样透传到上游。
func readAssetRequestBody(c *gin.Context) ([]byte, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

// writeUpstreamJSON 原样透传上游 JSON 响应，不做任何字段改写。
func writeUpstreamJSON(c *gin.Context, statusCode int, body []byte) {
	c.Data(statusCode, "application/json; charset=utf-8", body)
}
