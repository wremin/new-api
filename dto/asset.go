package dto

// 素材接口的 DTO。
//
// 上游可切换（seegen / Stelloria），两家的字段名与响应包装完全不同，
// 因此 new-api 对下游暴露的是**归一化契约**而非上游原样响应：
// 字段名沿用 seegen 的形态（officialId / name / status / region），
// 保证已按 seegen 文档接入的客户端在切换上游后不需要改代码。
//
// 可选标量字段一律使用指针类型 + omitempty（见 CLAUDE.md Rule 6），
// 避免显式传入的零值在重新序列化到上游时被静默丢弃。

// AssetCreateRequest 创建单个素材。
type AssetCreateRequest struct {
	GroupId string  `json:"groupId"`
	Url     string  `json:"url"`
	Name    *string `json:"name,omitempty"`
	// AssetType 可选：Image / Video / Audio。
	// seegen 不需要该字段；Stelloria 是必填项，不传时由 new-api 按 URL 后缀推断。
	AssetType *string `json:"assetType,omitempty"`
}

func (r *AssetCreateRequest) GetName() string {
	if r.Name == nil {
		return ""
	}
	return *r.Name
}

func (r *AssetCreateRequest) GetAssetType() string {
	if r.AssetType == nil {
		return ""
	}
	return *r.AssetType
}

// AssetGroupCreateRequest 创建素材组。
//
// Region 与 GroupType 分属两家上游，只会有一个生效：
//   - seegen：region = cn / intl，创建后不可修改
//   - Stelloria：groupType = AIGC / LivenessFace
//
// 传了当前上游不支持的那个字段会得到明确的 asset_unsupported_by_provider 错误，
// 而不是被静默忽略。
type AssetGroupCreateRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Region      *string `json:"region,omitempty"`
	GroupType   *string `json:"groupType,omitempty"`
}

func (r *AssetGroupCreateRequest) GetDescription() string {
	if r.Description == nil {
		return ""
	}
	return *r.Description
}

func (r *AssetGroupCreateRequest) GetRegion() string {
	if r.Region == nil {
		return ""
	}
	return *r.Region
}

func (r *AssetGroupCreateRequest) GetGroupType() string {
	if r.GroupType == nil {
		return ""
	}
	return *r.GroupType
}

// AssetBatchResultItem 是批量创建结果中的单条。
// index 对应提交数组的下标，客户端据此把 officialId 映射回自己的原始条目。
type AssetBatchResultItem struct {
	Index      int    `json:"index"`
	Status     string `json:"status"` // ok / error
	OfficialId string `json:"officialId,omitempty"`
	Error      string `json:"error,omitempty"`
}

// AssetBatchResponse 批量创建的响应。
// 上游没有原生批量接口时（Stelloria）由 new-api 循环单条实现，batchId 为空，
// 但响应形态对下游保持一致。
type AssetBatchResponse struct {
	BatchId string                 `json:"batchId,omitempty"`
	Total   int                    `json:"total"`
	Results []AssetBatchResultItem `json:"results"`
}

// AssetItemResponse 是 new-api 返回给客户端的素材对象。
// assetRef 是便利字段：可以直接填进生成任务的 url 字段。
type AssetItemResponse struct {
	Id         int64  `json:"id,omitempty"`
	OfficialId string `json:"officialId"`
	GroupId    string `json:"groupId,omitempty"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	// Region 仅 seegen 有；Stelloria 下为空
	Region     string `json:"region,omitempty"`
	AssetType  string `json:"assetType,omitempty"`
	Url        string `json:"url,omitempty"`
	AssetRef   string `json:"assetRef"`
	FailReason string `json:"failReason,omitempty"`
	// Provider 标明这条素材是在哪个上游创建的，上游切换后可据此识别失效素材
	Provider  string `json:"provider,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`

	// 仅管理员且 verbose=true 时返回，避免向普通用户泄露内部拓扑
	ChannelId int `json:"channelId,omitempty"`
}

// AssetListResponse 是素材列表的响应体。
type AssetListResponse struct {
	Items    []AssetItemResponse `json:"items"`
	Total    int64               `json:"total"`
	PageNum  int                 `json:"page_num"`
	PageSize int                 `json:"page_size"`
}

// AssetGroupItemResponse 是素材组对象。
type AssetGroupItemResponse struct {
	Id          int64  `json:"id,omitempty"`
	OfficialId  string `json:"officialId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Region 仅 seegen 有，GroupType 仅 Stelloria 有
	Region    string `json:"region,omitempty"`
	GroupType string `json:"groupType,omitempty"`
	Provider  string `json:"provider,omitempty"`
	// Count 对齐 seegen 响应中的 _count 结构
	Count     AssetGroupCount `json:"_count"`
	CreatedAt int64           `json:"createdAt,omitempty"`
	ChannelId int             `json:"channelId,omitempty"`
}

// AssetGroupCount 对齐上游响应中的 _count 结构。
type AssetGroupCount struct {
	Assets int `json:"assets"`
}

// AssetCapabilitiesResponse 声明当前上游支持哪些能力。
// 前端据此隐藏 / 禁用不可用的入口，客户端也可以先查询再决定调用哪些接口。
type AssetCapabilitiesResponse struct {
	Provider string `json:"provider"`
	// BatchCreate 是否可用批量创建（上游没有原生批量时由 new-api 循环实现，仍为 true）
	BatchCreate bool `json:"batchCreate"`
	// ExcelTemplate 是否支持 Excel 模板下载与表格批量上传
	ExcelTemplate bool `json:"excelTemplate"`
	// Regions 是否有 cn / intl 区域概念（决定素材与模型的区域一致性校验是否生效）
	Regions bool `json:"regions"`
	// GroupTypes 素材组类型枚举，为空表示当前上游没有该概念
	GroupTypes    []string `json:"groupTypes,omitempty"`
	RenameAsset   bool     `json:"renameAsset"`
	DeleteGroup   bool     `json:"deleteGroup"`
	BatchMaxItems int      `json:"batchMaxItems"`
}
