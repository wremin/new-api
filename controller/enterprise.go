package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// EnterpriseListSubAccounts lists sub-accounts with pagination and optional keyword search.
func EnterpriseListSubAccounts(c *gin.Context) {
	parentId := c.GetInt("id")
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)

	users, total, err := model.GetSubAccounts(parentId, keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
}

type enterpriseCreateSubAccountRequest struct {
	Username    string `json:"username" validate:"required,min=3,max=20"`
	Password    string `json:"password" validate:"required,min=8,max=20"`
	DisplayName string `json:"display_name" validate:"max=20"`
	Email       string `json:"email" validate:"max=50"`
}

// EnterpriseCreateSubAccount creates a new sub-account under the enterprise admin.
func EnterpriseCreateSubAccount(c *gin.Context) {
	parentId := c.GetInt("id")

	var req enterpriseCreateSubAccountRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Validate.Struct(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// Check username uniqueness
	exist, err := model.CheckUserExistOrDeleted(req.Username, req.Email)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if exist {
		common.ApiErrorI18n(c, i18n.MsgUserExists)
		return
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Username
	}

	user := &model.User{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: displayName,
		Email:       req.Email,
	}

	if err := model.CreateSubAccount(parentId, user); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccessI18n(c, i18n.MsgCreateSuccess, gin.H{"id": user.Id})
}

type enterpriseUpdateSubAccountRequest struct {
	Id          int     `json:"id"`
	DisplayName *string `json:"display_name" validate:"omitempty,max=20"`
	Password    string  `json:"password" validate:"omitempty,min=8,max=20"`
	Status      *int    `json:"status" validate:"omitempty,oneof=1 2"`
}

// EnterpriseUpdateSubAccount updates a sub-account's basic info. Only the fields
// present in the request are modified; status is restricted to enabled/disabled.
func EnterpriseUpdateSubAccount(c *gin.Context) {
	parentId := c.GetInt("id")

	var req enterpriseUpdateSubAccountRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Id == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Validate.Struct(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if err := model.UpdateSubAccount(parentId, req.Id, req.DisplayName, req.Password, req.Status); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccessI18n(c, i18n.MsgUpdateSuccess, nil)
}

// EnterpriseDeleteSubAccount deletes a sub-account.
func EnterpriseDeleteSubAccount(c *gin.Context) {
	parentId := c.GetInt("id")
	subUserId, err := strconv.Atoi(c.Param("id"))
	if err != nil || subUserId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}

	reclaimed, err := model.DeleteSubAccount(parentId, subUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Quota left on the sub-account was returned to the parent inside the delete
	// transaction; sync the parent's Redis cache and log the reclaim.
	if reclaimed > 0 {
		model.CacheIncrUserQuota(parentId, int64(reclaimed))
		model.RecordLog(parentId, model.LogTypeManage, "reclaimed remaining quota from deleted sub-account")
	}

	common.ApiSuccessI18n(c, i18n.MsgDeleteSuccess, nil)
}

type enterpriseQuotaRequest struct {
	Quota int `json:"quota"` // positive = allocate, negative = reclaim
}

// EnterpriseAllocateQuota allocates or reclaims quota for a sub-account.
func EnterpriseAllocateQuota(c *gin.Context) {
	parentId := c.GetInt("id")
	subUserId, err := strconv.Atoi(c.Param("id"))
	if err != nil || subUserId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}

	var req enterpriseQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Quota == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	if req.Quota > 0 {
		// Allocate quota from parent to sub-account
		if err := model.AllocateQuota(parentId, subUserId, req.Quota); err != nil {
			common.ApiError(c, err)
			return
		}
		// Sync Redis cache
		model.CacheIncrUserQuota(subUserId, int64(req.Quota))
		model.CacheDecrUserQuota(parentId, int64(req.Quota))
		model.RecordLog(parentId, model.LogTypeManage, "allocate quota to sub-account")
		model.RecordLog(subUserId, model.LogTypeTopup, "received quota from enterprise admin")
	} else {
		// Reclaim quota from sub-account to parent
		reclaimAmount := -req.Quota
		if err := model.ReclaimQuota(parentId, subUserId, reclaimAmount); err != nil {
			common.ApiError(c, err)
			return
		}
		// Sync Redis cache
		model.CacheDecrUserQuota(subUserId, int64(reclaimAmount))
		model.CacheIncrUserQuota(parentId, int64(reclaimAmount))
		model.RecordLog(parentId, model.LogTypeManage, "reclaimed quota from sub-account")
		model.RecordLog(subUserId, model.LogTypeManage, "quota reclaimed by enterprise admin")
	}

	common.ApiSuccess(c, nil)
}

// EnterpriseUsageStats returns aggregated usage statistics for all sub-accounts.
func EnterpriseUsageStats(c *gin.Context) {
	parentId := c.GetInt("id")

	stats, err := model.GetEnterpriseUsageStats(parentId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}
