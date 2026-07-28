package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// 素材审核状态，与上游 seegen.ai 保持一致
const (
	AssetStatusProcessing = "Processing"
	AssetStatusActive     = "Active"
	AssetStatusFailed     = "Failed"
)

// 素材区域，与上游素材组 region 保持一致
const (
	AssetRegionCN   = "cn"
	AssetRegionINTL = "intl"
)

// 素材类型
const (
	AssetTypeImage = "Image"
	AssetTypeVideo = "Video"
	AssetTypeAudio = "Audio"
)

// Asset 记录素材在 new-api 侧的归属关系。
// 上游账号没有 new-api 的用户概念，同一个上游账号下所有素材对上游是平的，
// 因此归属隔离必须由本表承担：所有查询强制带 user_id 条件。
type Asset struct {
	Id              int64  `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	OfficialId      string `json:"official_id" gorm:"type:varchar(191);uniqueIndex"`
	GroupOfficialId string `json:"group_official_id" gorm:"type:varchar(191);index"`
	UserId          int    `json:"user_id" gorm:"index"`
	// 单渠道模式下恒为同一值，保留以便后续多渠道演进（见 PRD §10）
	ChannelId   int             `json:"channel_id" gorm:"index"`
	TokenId     int             `json:"token_id"`
	Region      string          `json:"region" gorm:"type:varchar(10);index"`
	Name        string          `json:"name" gorm:"type:varchar(191)"`
	AssetType   string          `json:"asset_type" gorm:"type:varchar(20);index"`
	SourceUrl   string          `json:"source_url" gorm:"type:text"`
	Status      string          `json:"status" gorm:"type:varchar(20);index"`
	UpstreamId  int64           `json:"upstream_id"`
	UpstreamRaw json.RawMessage `json:"-" gorm:"type:json"`
	BatchId     string          `json:"batch_id" gorm:"type:varchar(64);index"`
	FailReason  string          `json:"fail_reason" gorm:"type:text"`
	CreatedAt   int64           `json:"created_at" gorm:"index"`
	UpdatedAt   int64           `json:"updated_at"`
	DeletedAt   int64           `json:"deleted_at" gorm:"index"` // 0 = 未删除
}

func (Asset) TableName() string {
	return "assets"
}

// AssetGroup 记录素材组在 new-api 侧的归属关系。
type AssetGroup struct {
	Id          int64  `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	OfficialId  string `json:"official_id" gorm:"type:varchar(191);uniqueIndex"`
	UserId      int    `json:"user_id" gorm:"index"`
	ChannelId   int    `json:"channel_id" gorm:"index"`
	Name        string `json:"name" gorm:"type:varchar(191)"`
	Description string `json:"description" gorm:"type:text"`
	Region      string `json:"region" gorm:"type:varchar(10);index"`
	UpstreamId  int64  `json:"upstream_id"`
	CreatedAt   int64  `json:"created_at" gorm:"index"`
	UpdatedAt   int64  `json:"updated_at"`
	DeletedAt   int64  `json:"deleted_at" gorm:"index"`

	// 查询时聚合，不落库
	AssetCount int `json:"asset_count" gorm:"-"`
}

func (AssetGroup) TableName() string {
	return "asset_groups"
}

// ============================
// Asset DAO
// ============================

func (a *Asset) Insert() error {
	now := common.GetTimestamp()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	return DB.Create(a).Error
}

// InsertAssetsBatch 批量写入，逐条 Create 以保证跨库兼容（SQLite/MySQL/PG）。
// 单条失败不中断其余记录，返回第一个错误供上层记日志。
func InsertAssetsBatch(assets []*Asset) error {
	if len(assets) == 0 {
		return nil
	}
	var firstErr error
	for _, a := range assets {
		if err := a.Insert(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// GetAssetByOfficialId 按 officialId + userId 查询，是归属校验的唯一入口。
// 查不到即视为不存在，调用方应直接返回 404 而不要透传到上游，
// 否则用户可以通过遍历 officialId 探测他人素材。
func GetAssetByOfficialId(userId int, officialId string) (*Asset, bool, error) {
	if officialId == "" {
		return nil, false, nil
	}
	var asset Asset
	err := DB.Where("user_id = ? and official_id = ? and deleted_at = ?", userId, officialId, 0).
		First(&asset).Error
	exist, err := RecordExist(err)
	if err != nil || !exist {
		return nil, exist, err
	}
	return &asset, true, nil
}

// GetAssetsByOfficialIds 批量查询，供 AssetRefCheck 中间件一次性校验多个 asset:// 引用。
func GetAssetsByOfficialIds(userId int, officialIds []string) ([]*Asset, error) {
	if len(officialIds) == 0 {
		return nil, nil
	}
	var assets []*Asset
	err := DB.Where("user_id = ? and official_id in (?) and deleted_at = ?", userId, officialIds, 0).
		Find(&assets).Error
	if err != nil {
		return nil, err
	}
	return assets, nil
}

type AssetSearchParams struct {
	UserId     int
	GroupId    string
	Status     string
	AssetType  string
	Keyword    string
	PageNum    int
	PageSize   int
	IncludeAll bool // 管理员视角，忽略 UserId 过滤
}

func GetAssets(params AssetSearchParams) ([]*Asset, int64, error) {
	if params.PageNum < 1 {
		params.PageNum = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}

	// Count 与 Find 各自构建一次查询：
	// GORM 复用同一个 *gorm.DB 做 Count 再 Find 会把 ORDER BY 带进 COUNT，
	// 部分数据库会因此报错，也容易踩到 Selects 被改写的坑。
	buildQuery := func() *gorm.DB {
		query := DB.Model(&Asset{}).Where("deleted_at = ?", 0)
		if !params.IncludeAll {
			query = query.Where("user_id = ?", params.UserId)
		}
		if params.GroupId != "" {
			query = query.Where("group_official_id = ?", params.GroupId)
		}
		if params.Status != "" {
			query = query.Where("status = ?", params.Status)
		}
		if params.AssetType != "" {
			query = query.Where("asset_type = ?", params.AssetType)
		}
		if params.Keyword != "" {
			kw := "%" + strings.TrimSpace(params.Keyword) + "%"
			query = query.Where("name LIKE ? or official_id LIKE ?", kw, kw)
		}
		return query
	}

	var total int64
	if err := buildQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var assets []*Asset
	err := buildQuery().Order("id desc").
		Limit(params.PageSize).
		Offset((params.PageNum - 1) * params.PageSize).
		Find(&assets).Error
	if err != nil {
		return nil, 0, err
	}
	return assets, total, nil
}

// CountUserAssets 统计用户未删除的素材总数，用于 assets_user_max_total 上限校验。
func CountUserAssets(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Asset{}).Where("user_id = ? and deleted_at = ?", userId, 0).Count(&total).Error
	return total, err
}

// GetAssetsNeedSync 返回需要向上游同步的记录：
// 1. status 仍为 Processing（审核未结束）；
// 2. name 为空（Excel 批量上传路径下 name/groupId 等字段无法从批量响应取得，需回填）。
func GetAssetsNeedSync(userId int, limit int) ([]*Asset, error) {
	if limit <= 0 {
		limit = 50
	}
	var assets []*Asset
	err := DB.Where("user_id = ? and deleted_at = ? and (status = ? or name = ?)",
		userId, 0, AssetStatusProcessing, "").
		Order("id desc").
		Limit(limit).
		Find(&assets).Error
	if err != nil {
		return nil, err
	}
	return assets, nil
}

func (a *Asset) Update() error {
	// 防御性检查：Id 为 0 时 GORM 会退化成全表更新
	if a.Id == 0 {
		return errors.New("cannot update asset without id")
	}
	a.UpdatedAt = common.GetTimestamp()
	return DB.Model(a).Select(
		"group_official_id", "region", "name", "asset_type", "source_url",
		"status", "upstream_id", "upstream_raw", "fail_reason", "updated_at",
	).Updates(a).Error
}

func (a *Asset) SoftDelete() error {
	now := common.GetTimestamp()
	return DB.Model(&Asset{}).Where("id = ?", a.Id).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
}

// ============================
// AssetGroup DAO
// ============================

func (g *AssetGroup) Insert() error {
	now := common.GetTimestamp()
	if g.CreatedAt == 0 {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	return DB.Create(g).Error
}

func GetAssetGroupByOfficialId(userId int, officialId string) (*AssetGroup, bool, error) {
	if officialId == "" {
		return nil, false, nil
	}
	var group AssetGroup
	err := DB.Where("user_id = ? and official_id = ? and deleted_at = ?", userId, officialId, 0).
		First(&group).Error
	exist, err := RecordExist(err)
	if err != nil || !exist {
		return nil, exist, err
	}
	return &group, true, nil
}

// GetAssetGroups 返回用户的素材组列表，并补齐每组的素材数量。
func GetAssetGroups(userId int, includeAll bool) ([]*AssetGroup, error) {
	query := DB.Model(&AssetGroup{}).Where("deleted_at = ?", 0)
	if !includeAll {
		query = query.Where("user_id = ?", userId)
	}
	var groups []*AssetGroup
	if err := query.Order("id desc").Find(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	officialIds := make([]string, 0, len(groups))
	for _, g := range groups {
		officialIds = append(officialIds, g.OfficialId)
	}

	// 用一次 group by 拿到全部计数，避免 N+1。
	// 只用 GORM 抽象与标准 SQL，保证 SQLite/MySQL/PG 三库一致。
	type groupCount struct {
		GroupOfficialId string `gorm:"column:group_official_id"`
		Total           int64  `gorm:"column:total"`
	}
	var counts []groupCount
	countQuery := DB.Model(&Asset{}).
		Select("group_official_id, count(*) as total").
		Where("deleted_at = ? and group_official_id in (?)", 0, officialIds)
	if !includeAll {
		countQuery = countQuery.Where("user_id = ?", userId)
	}
	if err := countQuery.Group("group_official_id").Scan(&counts).Error; err != nil {
		return nil, err
	}

	countMap := make(map[string]int64, len(counts))
	for _, c := range counts {
		countMap[c.GroupOfficialId] = c.Total
	}
	for _, g := range groups {
		g.AssetCount = int(countMap[g.OfficialId])
	}
	return groups, nil
}

func (g *AssetGroup) SoftDelete() error {
	now := common.GetTimestamp()
	return DB.Model(&AssetGroup{}).Where("id = ?", g.Id).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
}

// GuessAssetType 根据素材 URL 推断类型，仅用于上游未回显类型时的兜底展示。
func GuessAssetType(rawUrl string) string {
	u := strings.ToLower(rawUrl)
	// 去掉 query string，避免 ?x=.mp4 之类的干扰
	if idx := strings.IndexAny(u, "?#"); idx >= 0 {
		u = u[:idx]
	}
	switch {
	case hasAnySuffix(u, ".mp4", ".mov", ".webm", ".mkv", ".avi"):
		return AssetTypeVideo
	case hasAnySuffix(u, ".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg"):
		return AssetTypeAudio
	case hasAnySuffix(u, ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tiff", ".heic", ".heif"):
		return AssetTypeImage
	default:
		return ""
	}
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// AssetRefPrefix 是生成任务中引用素材的前缀。
const AssetRefPrefix = "asset://"

// BuildAssetRef 返回可直接填入生成任务 url 字段的引用字符串。
func BuildAssetRef(officialId string) string {
	return fmt.Sprintf("%s%s", AssetRefPrefix, officialId)
}
