/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type issueSubjectGrantRequest struct {
	Application string `json:"application"`
	Audience    string `json:"audience"`
	Scopes      string `json:"scopes"`
	TTL         int64  `json:"ttl"`
}

type introspectSubjectGrantRequest struct {
	Audience string `json:"audience"`
}

func IssueYanCoreSubjectGrant(c *gin.Context) {
	if !common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_GRANTS_ENABLED", false) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore subject grants are disabled."})
		return
	}
	var request issueSubjectGrantRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	grant, token, err := issueYanCoreSubjectGrant(c, request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"grant": token,
		"subject": gin.H{
			"user_id":     c.GetInt("id"),
			"application": grant.Application,
			"audience":    grant.Audience,
			"scopes":      grant.Scopes,
			"expires_at":  grant.ExpiresAt,
		},
	})
}

func issueYanCoreSubjectGrant(c *gin.Context, request issueSubjectGrantRequest) (*model.YanCoreSubjectGrant, string, error) {
	token, grant, err := model.IssueSubjectGrant(c.GetInt("id"), request.Application, request.Audience, request.Scopes, request.TTL)
	return grant, token, err
}

func ListYanCoreSubjectGrants(c *gin.Context) {
	if !common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_GRANTS_ENABLED", false) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore subject grants are disabled."})
		return
	}
	grants, err := model.GetSubjectGrants(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, grants)
}

func RevokeYanCoreSubjectGrant(c *gin.Context) {
	if !common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_GRANTS_ENABLED", false) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore subject grants are disabled."})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RevokeSubjectGrant(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func IntrospectYanCoreSubjectGrant(c *gin.Context) {
	if !common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_GRANTS_ENABLED", false) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore subject grants are disabled."})
		return
	}
	var request introspectSubjectGrantRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	claims, err := model.ParseSubjectGrantForAudience(bearerValue(c.GetHeader("Authorization")), request.Audience)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "YanCore subject grant is invalid."})
		return
	}
	userId := 0
	if strings.HasPrefix(claims.Subject, "yc_user_") {
		userId, _ = strconv.Atoi(strings.TrimPrefix(claims.Subject, "yc_user_"))
	}
	balance := 0
	if userId > 0 {
		balance, err = model.GetWalletFundingBalance(userId)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to load wallet balance for grant subject %d: %s", userId, err.Error()))
		}
	}
	common.ApiSuccess(c, gin.H{
		"subject":     claims.Subject,
		"application": claims.Application,
		"audience":    claims.Audience,
		"scopes":      claims.Scopes,
		"issued_at":   claims.IssuedAt.Unix(),
		"expires_at":  claims.ExpiresAt.Unix(),
		"grant_id":    claims.ID,
		"account": gin.H{
			"user_id":       userId,
			"balance_units": balance,
		},
	})
}

func bearerValue(value string) string {
	if len(value) >= 7 && (value[:7] == "Bearer " || value[:7] == "bearer ") {
		return value[7:]
	}
	return ""
}
