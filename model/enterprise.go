package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

var (
	ErrSubAccountNotFound = errors.New("sub-account not found")
	ErrQuotaInsufficient  = errors.New("insufficient quota")
	ErrQuotaNegative      = errors.New("quota cannot be negative")
)

// IsEnterpriseAdmin checks whether the user has the enterprise admin role.
func IsEnterpriseAdmin(userId int) (bool, error) {
	user, err := GetUserById(userId, true)
	if err != nil {
		return false, err
	}
	return user.Role == common.RoleEnterpriseAdmin, nil
}

// CreateSubAccount creates a sub-account under the given enterprise admin.
// The sub-account starts with 0 quota; quota must be allocated separately.
func CreateSubAccount(parentId int, user *User) error {
	if user.Username == "" || user.Password == "" {
		return errors.New("username and password are required")
	}
	// Hash password
	hashed, err := common.Password2Hash(user.Password)
	if err != nil {
		return err
	}
	user.Password = hashed
	user.ParentId = parentId
	user.Role = common.RoleCommonUser
	user.Status = common.UserStatusEnabled
	user.Quota = 0
	user.UsedQuota = 0
	user.RequestCount = 0
	user.AffCode = common.GetRandomString(4)

	return DB.Create(user).Error
}

// GetSubAccounts returns paginated sub-accounts belonging to the given parent.
func GetSubAccounts(parentId int, keyword string, startIdx int, pageSize int) ([]*User, int64, error) {
	var users []*User
	var total int64

	query := DB.Model(&User{}).Where("parent_id = ?", parentId)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR display_name LIKE ? OR email LIKE ?", like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// Omit the password hash from the SELECT; it must never be returned to clients.
	if err := query.Order("id desc").Offset(startIdx).Limit(pageSize).Omit("password").Find(&users).Error; err != nil {
		return nil, 0, err
	}
	// Strip the system management access token before returning.
	for _, u := range users {
		u.AccessToken = nil
	}
	return users, total, nil
}

// GetSubAccount returns a sub-account only if it belongs to the given parent.
func GetSubAccount(parentId, userId int) (*User, error) {
	var user User
	err := DB.Where("id = ? AND parent_id = ?", userId, parentId).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSubAccountNotFound
		}
		return nil, err
	}
	return &user, nil
}

// UpdateSubAccount updates basic info of a sub-account. Only the fields that are
// explicitly provided (non-nil pointers / non-empty password) are updated, so a
// partial update never clobbers untouched fields such as display_name or status.
func UpdateSubAccount(parentId, userId int, displayName *string, password string, status *int) error {
	// Verify ownership
	if _, err := GetSubAccount(parentId, userId); err != nil {
		return err
	}

	updates := map[string]interface{}{}
	if displayName != nil {
		updates["display_name"] = *displayName
	}
	if status != nil {
		updates["status"] = *status
	}
	if password != "" {
		hashed, err := common.Password2Hash(password)
		if err != nil {
			return err
		}
		updates["password"] = hashed
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ? AND parent_id = ?", userId, parentId).Updates(updates).Error
}

// DeleteSubAccount soft-deletes a sub-account and returns its remaining quota to
// the parent enterprise admin (the quota was originally allocated from the parent).
// It returns the amount of quota reclaimed so the caller can sync caches/logs.
func DeleteSubAccount(parentId, userId int) (int, error) {
	var reclaimed int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub User
		// Row lock the sub-account and verify ownership.
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND parent_id = ?", userId, parentId).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSubAccountNotFound
			}
			return err
		}

		reclaimed = sub.Quota
		if reclaimed > 0 {
			if err := tx.Model(&User{}).Where("id = ?", parentId).
				Update("quota", gorm.Expr("quota + ?", reclaimed)).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("id = ? AND parent_id = ?", userId, parentId).Delete(&User{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Invalidate the deleted sub-account's cache (best-effort).
	_ = invalidateUserCache(userId)
	return reclaimed, nil
}

// AllocateQuota transfers quota from the enterprise admin to a sub-account.
// quota must be > 0.
func AllocateQuota(parentId, subUserId int, quota int) error {
	if quota <= 0 {
		return errors.New("quota must be positive")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		// Deduct from parent
		result := tx.Model(&User{}).Where("id = ? AND quota >= ?", parentId, quota).
			Update("quota", gorm.Expr("quota - ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrQuotaInsufficient
		}

		// Add to sub-account (verify ownership)
		result = tx.Model(&User{}).Where("id = ? AND parent_id = ?", subUserId, parentId).
			Update("quota", gorm.Expr("quota + ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrSubAccountNotFound
		}
		return nil
	})
	// Note: Redis cache sync is done by the caller after successful transaction
}

// ReclaimQuota takes quota back from a sub-account and returns it to the enterprise admin.
// quota must be > 0.
func ReclaimQuota(parentId, subUserId int, quota int) error {
	if quota <= 0 {
		return errors.New("quota must be positive")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		// Deduct from sub-account
		result := tx.Model(&User{}).Where("id = ? AND parent_id = ? AND quota >= ?", subUserId, parentId, quota).
			Update("quota", gorm.Expr("quota - ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrQuotaInsufficient
		}

		// Add back to parent
		result = tx.Model(&User{}).Where("id = ?", parentId).
			Update("quota", gorm.Expr("quota + ?", quota))
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
}

// EnterpriseUsageStat holds aggregated usage data for a single sub-account.
type EnterpriseUsageStat struct {
	UserId         int    `json:"user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Quota          int    `json:"quota"`
	UsedQuota      int    `json:"used_quota"`
	RequestCount   int    `json:"request_count"`
	TotalTokens    int    `json:"total_tokens"`
	TotalQuotaUsed int    `json:"total_quota_used"`
}

// GetEnterpriseUsageStats returns usage statistics for all sub-accounts of the given parent.
func GetEnterpriseUsageStats(parentId int) ([]EnterpriseUsageStat, error) {
	var users []*User
	if err := DB.Where("parent_id = ?", parentId).Omit("password").Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return []EnterpriseUsageStat{}, nil
	}

	userIds := make([]int, 0, len(users))
	for _, u := range users {
		userIds = append(userIds, u.Id)
	}

	// Aggregate consume-log stats for all sub-accounts in a single grouped query.
	type aggRow struct {
		UserId      int `gorm:"column:user_id"`
		TotalTokens int `gorm:"column:total_tokens"`
		TotalQuota  int `gorm:"column:total_quota"`
	}
	var rows []aggRow
	err := LOG_DB.Table("logs").
		Select("user_id, COALESCE(SUM(prompt_tokens + completion_tokens), 0) as total_tokens, COALESCE(SUM(quota), 0) as total_quota").
		Where("user_id IN ? AND type = ?", userIds, LogTypeConsume).
		Group("user_id").
		Scan(&rows).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to get enterprise usage stats for parent %d: %v", parentId, err))
	}

	aggByUser := make(map[int]aggRow, len(rows))
	for _, r := range rows {
		aggByUser[r.UserId] = r
	}

	stats := make([]EnterpriseUsageStat, 0, len(users))
	for _, u := range users {
		stat := EnterpriseUsageStat{
			UserId:       u.Id,
			Username:     u.Username,
			DisplayName:  u.DisplayName,
			Quota:        u.Quota,
			UsedQuota:    u.UsedQuota,
			RequestCount: u.RequestCount,
		}
		if agg, ok := aggByUser[u.Id]; ok {
			stat.TotalTokens = agg.TotalTokens
			stat.TotalQuotaUsed = agg.TotalQuota
		}
		stats = append(stats, stat)
	}
	return stats, nil
}
