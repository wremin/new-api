package dto

// 素材接口的 DTO。请求 / 响应结构与上游 seegen.ai 保持一致，
// 可选标量字段一律使用指针类型 + omitempty（见 CLAUDE.md Rule 6），
// 避免显式传入的零值在重新序列化到上游时被静默丢弃。

// AssetCreateRequest 对应上游 POST /v1/assets
type AssetCreateRequest struct {
	GroupId string  `json:"groupId"`
	Url     string  `json:"url"`
	Name    *string `json:"name,omitempty"`
}

func (r *AssetCreateRequest) GetName() string {
	if r.Name == nil {
		return ""
	}
	return *r.Name
}

// AssetGroupCreateRequest 对应上游 POST /v1/assets/groups
type AssetGroupCreateRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	// cn（默认）或 intl，创建后不可修改
	Region *string `json:"region,omitempty"`
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

// AssetBatchResultItem 对应上游批量上传响应中的单条结果。
// 上游只按 index 回显 officialId，不回显 name / groupId / url。
type AssetBatchResultItem struct {
	Index      int    `json:"index"`
	Status     string `json:"status"`
	OfficialId string `json:"officialId,omitempty"`
	Error      string `json:"error,omitempty"`
}

// AssetBatchResponse 对应上游 POST /v1/assets/batch 的响应
type AssetBatchResponse struct {
	BatchId string                 `json:"batchId"`
	Total   int                    `json:"total"`
	Results []AssetBatchResultItem `json:"results"`
}

// AssetItemResponse 是 new-api 返回给客户端的素材对象。
// 字段命名对齐上游，额外的 assetRef 是便利字段：
// 客户端可以直接把它填进生成任务的 url 字段。
type AssetItemResponse struct {
	Id         int64  `json:"id"`
	OfficialId string `json:"officialId"`
	GroupId    string `json:"groupId,omitempty"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Region     string `json:"region"`
	AssetType  string `json:"assetType,omitempty"`
	Url        string `json:"url,omitempty"`
	AssetRef   string `json:"assetRef"`
	FailReason string `json:"failReason,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`

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
	Id          int64           `json:"id"`
	OfficialId  string          `json:"officialId"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Region      string          `json:"region"`
	Count       AssetGroupCount `json:"_count"`
	CreatedAt   int64           `json:"createdAt"`
	ChannelId   int             `json:"channelId,omitempty"`
}

// AssetGroupCount 对齐上游响应中的 _count 结构。
type AssetGroupCount struct {
	Assets int `json:"assets"`
}
