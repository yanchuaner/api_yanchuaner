/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type applyYanCoreVirtualKeyPolicyRolloutRequest struct {
	TokenIDs []int  `json:"token_ids" binding:"required,min=1,max=100"`
	Reason   string `json:"reason" binding:"required,min=3,max=160"`
}

func yanCoreVirtualKeyPolicyTokenID(c *gin.Context) (int, bool) {
	tokenID, err := strconv.Atoi(c.Param("token_id"))
	if err != nil || tokenID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid token id"})
		return 0, false
	}
	return tokenID, true
}

func GetYanCoreVirtualKeyPolicy(c *gin.Context) {
	tokenID, ok := yanCoreVirtualKeyPolicyTokenID(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	if _, err := model.GetTokenByIds(tokenID, userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": model.ErrYanCoreVirtualKeyPolicyNotFound.Error()})
		return
	}
	policy, err := model.GetYanCoreVirtualKeyPolicy(tokenID, userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if policy == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": model.ErrYanCoreVirtualKeyPolicyNotFound.Error()})
		return
	}
	common.ApiSuccess(c, policy)
}

func UpdateYanCoreVirtualKeyPolicy(c *gin.Context) {
	tokenID, ok := yanCoreVirtualKeyPolicyTokenID(c)
	if !ok {
		return
	}
	var config model.YanCoreVirtualKeyPolicyConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		common.ApiError(c, err)
		return
	}
	userID := c.GetInt("id")
	policy, err := model.UpdateYanCoreVirtualKeyPolicy(tokenID, userID, userID, config)
	if err != nil {
		if errors.Is(err, model.ErrYanCoreVirtualKeyPolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": err.Error()})
			return
		}
		if errors.Is(err, model.ErrYanCoreVirtualKeyPolicyInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, policy)
}

func ListYanCoreVirtualKeyPolicyRevisions(c *gin.Context) {
	tokenID, ok := yanCoreVirtualKeyPolicyTokenID(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	if _, err := model.GetTokenByIds(tokenID, userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": model.ErrYanCoreVirtualKeyPolicyNotFound.Error()})
		return
	}
	revisions, err := model.ListYanCoreVirtualKeyPolicyRevisions(tokenID, userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, revisions)
}

func GetYanCoreVirtualKeyPolicyRolloutReport(c *gin.Context) {
	report, err := model.GetYanCoreVirtualKeyPolicyRolloutReport(200)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

func ApplyYanCoreVirtualKeyPolicyRollout(c *gin.Context) {
	var request applyYanCoreVirtualKeyPolicyRolloutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid virtual key policy rollout request"})
		return
	}
	result, err := model.ApplyYanCoreVirtualKeyPolicyRollout(request.TokenIDs, c.GetInt("id"), request.Reason)
	if err != nil {
		if errors.Is(err, model.ErrYanCoreVirtualKeyPolicyInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
		if errors.Is(err, model.ErrYanCoreVirtualKeyPolicyRolloutNotPending) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "yancore.virtual-key-policy.rollout.apply", map[string]interface{}{"count": result.Applied, "activated": result.Activated, "disabled": result.Disabled, "reason": request.Reason})
	common.ApiSuccess(c, result)
}
