/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createYanCoreCampaignRequest struct {
	Name          string `json:"name" binding:"required"`
	Quota         int    `json:"quota" binding:"required,gt=0"`
	ProviderScope string `json:"provider_scope"`
	ModelScope    string `json:"model_scope"`
	StartsAt      int64  `json:"starts_at"`
	ExpiresAt     int64  `json:"expires_at" binding:"required,gt=0"`
	MaxClaims     int    `json:"max_claims" binding:"required,gt=0"`
}

type createYanCoreRedeemCodesRequest struct {
	Count     int `json:"count" binding:"required,gt=0,max=100"`
	MaxClaims int `json:"max_claims" binding:"required,gt=0"`
}

var claimYanCoreRedeemCode = model.ClaimYanCoreRedeemCode

func yancoreEntitlementError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, model.ErrYanCoreCampaignExpired), errors.Is(err, model.ErrYanCoreRedeemCodeExpired):
		status = http.StatusGone
	case errors.Is(err, model.ErrYanCoreCampaignExhausted), errors.Is(err, model.ErrYanCoreRedeemCodeExhausted), errors.Is(err, model.ErrYanCoreEntitlementReplayed):
		status = http.StatusConflict
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, model.ErrYanCoreEntitlementTarget):
		status = http.StatusForbidden
	}
	c.JSON(status, gin.H{"success": false, "message": err.Error()})
}

func CreateYanCoreCampaign(c *gin.Context) {
	var request createYanCoreCampaignRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid campaign request"})
		return
	}
	startsAt := request.StartsAt
	if startsAt == 0 {
		startsAt = time.Now().Unix()
	}
	campaign := &model.YanCoreCampaign{
		Name: request.Name, FundingSource: model.YanCoreEntitlementSourceCampaign, Quota: request.Quota,
		ProviderScope: strings.TrimSpace(request.ProviderScope), ModelScope: strings.TrimSpace(request.ModelScope),
		StartsAt: startsAt, ExpiresAt: request.ExpiresAt, MaxClaims: request.MaxClaims,
		Status: model.YanCoreCampaignStatusEnabled, CreatedBy: c.GetInt("id"),
	}
	if err := model.CreateYanCoreCampaign(campaign); err != nil {
		yancoreEntitlementError(c, err)
		return
	}
	recordManageAudit(c, "yancore.entitlement.campaign.create", map[string]interface{}{"campaign_id": campaign.Id, "name": campaign.Name, "quota": campaign.Quota, "max_claims": campaign.MaxClaims})
	common.ApiSuccess(c, campaign)
}

func CreateYanCoreRedeemCodes(c *gin.Context) {
	campaignID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || campaignID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "campaign id is invalid"})
		return
	}
	var request createYanCoreRedeemCodesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid redeem code request"})
		return
	}
	codes, err := model.CreateYanCoreRedeemCodes(campaignID, request.Count, request.MaxClaims)
	if err != nil {
		yancoreEntitlementError(c, err)
		return
	}
	recordManageAudit(c, "yancore.entitlement.redeem-code.create", map[string]interface{}{"campaign_id": campaignID, "count": request.Count, "max_claims": request.MaxClaims})
	common.ApiSuccess(c, gin.H{"campaign_id": campaignID, "codes": codes, "warning": "codes are shown once; store them securely"})
}

func ClaimYanCoreEntitlement(c *gin.Context) {
	var request struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "code is required"})
		return
	}
	entitlement, replayed, err := claimYanCoreRedeemCode(c.GetInt("id"), request.Code)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to claim YanCore entitlement for user %d: %s", c.GetInt("id"), err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "entitlement claim failed"})
		return
	}
	common.ApiSuccess(c, gin.H{"entitlement": entitlement, "replayed": replayed})
}

func ListYanCoreEntitlements(c *gin.Context) {
	entitlements, err := model.ListYanCoreEntitlements(c.GetInt("id"))
	if err != nil {
		yancoreEntitlementError(c, err)
		return
	}
	common.ApiSuccess(c, entitlements)
}
