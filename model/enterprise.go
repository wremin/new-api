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
	if err := query.Order("id desc").Offset(startIdx).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
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

// UpdateSubAccount updates basic info (display_name, password, status) of a sub-account.
func UpdateSubAccount(parentId int, user *User) error {
	// Verify ownership
	existing, err := GetSubAccount(parentId, user.Id)
	if err != nil {
		return err
	}

	updates := map[string]interface{}{
		"display_name": user.DisplayName,
		"status":       user.Status,
	}
	if user.Password != "" {
		hashed, err := common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
		updates["password"] = hashed
	}
	_ = existing // suppress unused warning
	return DB.Model(&User{}).Where("id = ? AND parent_id = ?", user.Id, parentId).Updates(updates).Error
}

// DeleteSubAccount soft-deletes a sub-account. Remaining quota is NOT reclaimed.
func DeleteSubAccount(parentId, userId int) error {
	if _, err := GetSubAccount(parentId, userId); err != nil {
		return err
	}
	return DB.Where("id = ? AND parent_id = ?", userId, parentId).Delete(&User{}).Error
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
	if err := DB.Where("parent_id = ?", parentId).Find(&users).Error; err != nil {
		return nil, err
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
		// Aggregate log stats for this user
		var agg struct {
			TotalTokens int `gorm:"column:total_tokens"`
			TotalQuota  int `gorm:"column:total_quota"`
		}
		err := LOG_DB.Table("logs").
			Select("COALESCE(SUM(prompt_tokens + completion_tokens), 0) as total_tokens, COALESCE(SUM(quota), 0) as total_quota").
			Where("user_id = ? AND type = ?", u.Id, LogTypeConsume).
			Scan(&agg).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to get usage stats for user %d: %v", u.Id, err))
		} else {
			stat.TotalTokens = agg.TotalTokens
			stat.TotalQuotaUsed = agg.TotalQuota
		}
		stats = append(stats, stat)
	}
	return stats, nil
}
