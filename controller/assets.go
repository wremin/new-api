package controller

import (
	"fmt"
	"io"
	"net/http"
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

// 素材接口对下游暴露**归一化契约**，不再原样透传上游响应。
//
// 原因：上游可切换（seegen / Stelloria），两家的路径、字段名、响应包装完全不同。
// 如果透传，客户端换个上游就得改代码，网关就失去意义了。
// 字段名沿用 seegen 的形态（officialId / name / status / region），
// 保证已按 seegen 文档接入的客户端不受影响。

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

// resolveAssetsProvider 取本期唯一的素材渠道及其上游实现，失败时已写入响应。
func resolveAssetsProvider(c *gin.Context) (*model.Channel, service.AssetsProvider, bool) {
	channel, aErr := service.GetAssetsChannel()
	if aErr != nil {
		assetServiceError(c, aErr)
		return nil, nil, false
	}
	return channel, service.GetAssetsProvider(channel), true
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
		Provider:   service.PublicProviderName(a.Provider),
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
	if verbose {
		item.ChannelId = a.ChannelId
	}
	return item
}

// ============================
// GET /v1/assets/capabilities
// ============================

// assetCapabilities 让前端与客户端知道当前上游支持哪些能力，
// 从而隐藏 / 禁用不可用的入口，而不是发一个注定 501 的请求。
func assetCapabilities(c *gin.Context) {
	channel, aErr := service.GetAssetsChannel()
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	provider := service.GetAssetsProvider(channel)
	caps := provider.Capabilities()
	c.JSON(http.StatusOK, dto.AssetCapabilitiesResponse{
		Provider:      service.PublicProviderName(provider.Name()),
		BatchCreate:   caps.BatchCreate,
		ExcelTemplate: caps.ExcelTemplate,
		Regions:       caps.Regions,
		GroupTypes:    caps.GroupTypes,
		RenameAsset:   caps.RenameAsset,
		DeleteGroup:   caps.DeleteGroup,
		BatchMaxItems: operation_setting.GetAssetsBatchMaxItems(),
	})
}

// ============================
// POST /v1/assets
// ============================

func RelayAssetCreate(c *gin.Context) {
	channel, provider, ok := resolveAssetsProvider(c)
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

	userId, tokenId := assetRequestUser(c)
	// 只需要 region：它要落进素材记录供后续的区域一致性校验使用。
	// groupType 是素材组级属性，创建素材时上游不接收该字段，所以这里用不到。
	region, _ := lookupGroupMeta(userId, req.GroupId)

	fields, aErr := provider.CreateAsset(c.Request.Context(), channel, service.CreateAssetInput{
		GroupId:   req.GroupId,
		Url:       req.Url,
		Name:      req.GetName(),
		AssetType: req.GetAssetType(),
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	asset, err := service.RecordUploadedAsset(userId, tokenId, channel.Id, provider.Name(), fields,
		service.AssetFallbackFields("", req.GroupId, req.GetName(), req.Url, region))
	if err != nil {
		// 落库失败不影响用户拿到上游结果，但必须记日志：
		// 归属记录缺失会导致该素材后续无法被查询/引用。
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to record uploaded asset for user %d: %s", userId, err.Error()))
		c.JSON(http.StatusOK, assetItemFromFields(fields, provider.Name()))
		return
	}
	c.JSON(http.StatusOK, toAssetItem(asset, false))
}

// lookupGroupMeta 取素材组在本地记录的 region / groupType，用于补齐上游未回显的字段。
func lookupGroupMeta(userId int, groupOfficialId string) (region string, groupType string) {
	if groupOfficialId == "" {
		return "", ""
	}
	group, exist, err := model.GetAssetGroupByOfficialId(userId, groupOfficialId)
	if err != nil || !exist {
		return "", ""
	}
	return group.Region, group.GroupType
}

func assetItemFromFields(f service.AssetFields, provider string) dto.AssetItemResponse {
	return dto.AssetItemResponse{
		OfficialId: f.OfficialId,
		GroupId:    f.GroupId,
		Name:       f.Name,
		Status:     f.Status,
		Region:     f.Region,
		AssetType:  f.AssetType,
		Url:        f.Url,
		AssetRef:   model.BuildAssetRef(f.OfficialId),
		FailReason: f.FailReason,
		Provider:   service.PublicProviderName(provider),
	}
}

// ============================
// GET /v1/assets（本地查询，不透传）
// ============================

func RelayAssetList(c *gin.Context) {
	userId := c.GetInt("id")

	pageNum, _ := strconv.Atoi(c.DefaultQuery("page_num", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 状态同步是同步的上游 HTTP 调用，默认不在列表接口里做，避免拖慢响应。
	// 前端轮询与用户主动刷新时带 refresh=true，此时才回源同步。
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
	case action == "capabilities":
		if c.Request.Method != http.MethodGet {
			assetMethodNotAllowed(c)
			return
		}
		assetCapabilities(c)
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
	channel, provider, ok := resolveAssetsProvider(c)
	if !ok {
		return
	}

	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		assetBatchUploadExcel(c, channel, provider, contentType)
		return
	}
	assetBatchUploadJSON(c, channel, provider)
}

// assetBatchUploadJSON 处理 JSON 数组批量上传。
//
// 结果按 index 与请求体数组对齐，因此能一次写全归属记录。
// 上游没有原生批量接口时（Stelloria），provider 会退化成循环单条创建，
// 对下游的响应形态完全一致。
func assetBatchUploadJSON(c *gin.Context, channel *model.Channel, provider service.AssetsProvider) {
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

	ins := make([]service.CreateAssetInput, 0, len(items))
	for _, item := range items {
		ins = append(ins, service.CreateAssetInput{
			GroupId:   item.GroupId,
			Url:       item.Url,
			Name:      item.GetName(),
			AssetType: item.GetAssetType(),
		})
	}

	batchId, results, aErr := provider.BatchCreateAssets(c.Request.Context(), channel, ins)
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}

	recordBatchAssets(c, channel, provider, batchId, results, items)
	c.JSON(http.StatusOK, dto.AssetBatchResponse{
		BatchId: batchId,
		Total:   len(results),
		Results: toBatchResultDTO(results),
	})
}

func toBatchResultDTO(results []service.BatchItemResult) []dto.AssetBatchResultItem {
	out := make([]dto.AssetBatchResultItem, 0, len(results))
	for _, r := range results {
		out = append(out, dto.AssetBatchResultItem{
			Index:      r.Index,
			Status:     r.Status,
			OfficialId: r.OfficialId,
			Error:      r.Error,
		})
	}
	return out
}

// recordBatchAssets 按 index 把上游结果与请求体对齐后落库。
// items 为 nil 时（Excel 路径）只写 officialId 等最小字段，其余交给异步回填。
func recordBatchAssets(
	c *gin.Context,
	channel *model.Channel,
	provider service.AssetsProvider,
	batchId string,
	results []service.BatchItemResult,
	items []dto.AssetCreateRequest,
) {
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
		region, _ := lookupGroupMeta(userId, groupId)
		regionCache[groupId] = region
		return region
	}

	assets := make([]*model.Asset, 0, len(results))
	for _, result := range results {
		if result.Status != "ok" || result.OfficialId == "" {
			continue
		}
		asset := &model.Asset{
			OfficialId: result.OfficialId,
			UserId:     userId,
			TokenId:    tokenId,
			ChannelId:  channel.Id,
			Provider:   provider.Name(),
			Status:     model.AssetStatusProcessing,
			BatchId:    batchId,
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
// 仅 seegen 支持；Stelloria 没有表格上传接口，provider 会返回 501。
// seegen 路径下 new-api 不解析表格，只能从响应拿到 officialId，
// name / groupId / region / url 全部留空，由 SyncPendingAssets 异步回填。
func assetBatchUploadExcel(c *gin.Context, channel *model.Channel, provider service.AssetsProvider, contentType string) {
	if !provider.Capabilities().ExcelTemplate {
		assetServiceError(c, service.ErrCapabilityUnsupported(provider.Name(), "excel batch upload"))
		return
	}

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

	batchId, results, aErr := provider.BatchCreateFromExcel(c.Request.Context(), channel, contentType, rawBody)
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}

	recordBatchAssets(c, channel, provider, batchId, results, nil)
	c.JSON(http.StatusOK, dto.AssetBatchResponse{
		BatchId: batchId,
		Total:   len(results),
		Results: toBatchResultDTO(results),
	})
}

// assetBatchTemplate 二进制流式透传 Excel 模板。
func assetBatchTemplate(c *gin.Context) {
	channel, provider, ok := resolveAssetsProvider(c)
	if !ok {
		return
	}
	if !provider.Capabilities().ExcelTemplate {
		assetServiceError(c, service.ErrCapabilityUnsupported(provider.Name(), "excel template download"))
		return
	}

	resp, aErr := provider.ExcelTemplate(c.Request.Context(), channel)
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
	channel, provider, ok := resolveAssetsProvider(c)
	if !ok {
		return
	}
	caps := provider.Capabilities()

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

	// region 与 groupType 分属两家上游，按能力集校验，传错的那个直接报明确错误
	region := strings.ToLower(strings.TrimSpace(req.GetRegion()))
	if caps.Regions {
		if region != "" && region != model.AssetRegionCN && region != model.AssetRegionINTL {
			assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
				`region must be "cn" or "intl"`)
			return
		}
	} else if region != "" {
		assetError(c, http.StatusBadRequest, service.AssetErrUnsupported,
			"current upstream provider "+service.PublicProviderName(provider.Name())+" has no region concept, please use groupType instead")
		return
	}

	groupType := strings.TrimSpace(req.GetGroupType())
	if len(caps.GroupTypes) > 0 {
		if groupType != "" && !containsString(caps.GroupTypes, groupType) {
			assetError(c, http.StatusBadRequest, service.AssetErrInvalidRequest,
				"groupType must be one of: "+strings.Join(caps.GroupTypes, ", "))
			return
		}
	} else if groupType != "" {
		assetError(c, http.StatusBadRequest, service.AssetErrUnsupported,
			"current upstream provider "+service.PublicProviderName(provider.Name())+" has no groupType concept")
		return
	}

	fields, aErr := provider.CreateGroup(c.Request.Context(), channel, service.CreateGroupInput{
		Name:        req.Name,
		Description: req.GetDescription(),
		Region:      region,
		GroupType:   groupType,
	})
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if fields.OfficialId == "" {
		assetError(c, http.StatusBadGateway, service.AssetErrUpstream,
			"upstream response has no group id")
		return
	}

	userId := c.GetInt("id")
	group := &model.AssetGroup{
		OfficialId:  fields.OfficialId,
		UserId:      userId,
		ChannelId:   channel.Id,
		Provider:    provider.Name(),
		Name:        fields.Name,
		Description: fields.Description,
		Region:      strings.ToLower(fields.Region),
		GroupType:   fields.GroupType,
		UpstreamId:  fields.UpstreamId,
	}
	if err := group.Insert(); err != nil {
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to record asset group for user %d: %s", userId, err.Error()))
	}

	c.JSON(http.StatusOK, toAssetGroupItem(group, false))
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func toAssetGroupItem(g *model.AssetGroup, verbose bool) dto.AssetGroupItemResponse {
	item := dto.AssetGroupItemResponse{
		Id:          g.Id,
		OfficialId:  g.OfficialId,
		Name:        g.Name,
		Description: g.Description,
		Region:      g.Region,
		GroupType:   g.GroupType,
		Provider:    service.PublicProviderName(g.Provider),
		Count:       dto.AssetGroupCount{Assets: g.AssetCount},
		CreatedAt:   g.CreatedAt,
	}
	if verbose {
		item.ChannelId = g.ChannelId
	}
	return item
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
		items = append(items, toAssetGroupItem(g, verbose))
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

	channel, provider, ok := resolveAssetsProvider(c)
	if !ok {
		return
	}

	// 上游已切换时，旧素材在新上游不存在，直接返回本地记录并提示，
	// 避免把一个必然失败的请求打到上游。
	if asset.Provider != "" && asset.Provider != provider.Name() {
		assetError(c, http.StatusConflict, service.AssetErrProviderMismatch,
			fmt.Sprintf("asset %s was created on provider %q but current provider is %q",
				officialId, service.PublicProviderName(asset.Provider), service.PublicProviderName(provider.Name())))
		return
	}

	fields, aErr := provider.GetAsset(c.Request.Context(), channel, officialId)
	if aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if err := service.ApplyUpstreamAssetFields(asset, fields); err != nil {
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to sync asset %s: %s", officialId, err.Error()))
	}

	c.JSON(http.StatusOK, toAssetItem(asset, assetVerbose(c)))
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

	channel, provider, ok := resolveAssetsProvider(c)
	if !ok {
		return
	}

	// 上游已切换时不去删新上游的同名素材，只清理本地记录
	if asset.Provider != "" && asset.Provider != provider.Name() {
		if err := asset.SoftDelete(); err != nil {
			logger.LogError(c.Request.Context(),
				fmt.Sprintf("failed to soft delete asset %s: %s", officialId, err.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"officialId": officialId, "deleted": true, "upstreamSkipped": true})
		return
	}

	if aErr := provider.DeleteAsset(c.Request.Context(), channel, officialId); aErr != nil {
		assetServiceError(c, aErr)
		return
	}
	if err := asset.SoftDelete(); err != nil {
		logger.LogError(c.Request.Context(),
			fmt.Sprintf("failed to soft delete asset %s: %s", officialId, err.Error()))
	}
	c.JSON(http.StatusOK, gin.H{"officialId": officialId, "deleted": true})
}

// ============================
// 工具函数
// ============================

// readAssetRequestBody 取原始请求体，用于 multipart 原样透传到上游。
func readAssetRequestBody(c *gin.Context) ([]byte, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}
