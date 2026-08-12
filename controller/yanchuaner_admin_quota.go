/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type adminQuotaRequest struct {
	UserId         int    `json:"user_id"`
	Action         string `json:"action"`
	Amount         int    `json:"amount"`
	Reason         string `json:"reason"`
	Reference      string `json:"reference"`
	IdempotencyKey string `json:"idempotency_key"`
}

func adminQuotaError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "message": message})
}

// AdminQuotaAdjust issues or adjusts public-benefit quota after offline
// payment. It requires a client-controlled idempotency key or a payment
// reference so a retried request cannot double-grant quota.
func AdminQuotaAdjust(c *gin.Context) {
	var request adminQuotaRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		adminQuotaError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	request.Action = strings.TrimSpace(strings.ToLower(request.Action))
	request.Reason = strings.TrimSpace(request.Reason)
	request.Reference = strings.TrimSpace(request.Reference)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)

	if request.UserId <= 0 {
		adminQuotaError(c, http.StatusBadRequest, "user_id is required")
		return
	}
	if request.Action != "grant" && request.Action != "adjust" {
		adminQuotaError(c, http.StatusBadRequest, "action must be grant or adjust")
		return
	}
	if request.Amount == 0 || request.Amount > math.MaxInt32 || request.Amount < -math.MaxInt32 {
		adminQuotaError(c, http.StatusBadRequest, "amount is out of range")
		return
	}
	if request.Action == "grant" && request.Amount <= 0 {
		adminQuotaError(c, http.StatusBadRequest, "grant amount must be positive")
		return
	}
	if request.Reason == "" || len([]rune(request.Reason)) > 200 {
		adminQuotaError(c, http.StatusBadRequest, "reason is required and must not exceed 200 characters")
		return
	}
	if len(request.Reference) > 128 {
		adminQuotaError(c, http.StatusBadRequest, "reference must not exceed 128 characters")
		return
	}
	if request.IdempotencyKey == "" {
		if request.Reference == "" {
			adminQuotaError(c, http.StatusBadRequest, "reference or idempotency_key is required")
			return
		}
		request.IdempotencyKey = "quota:ref:" + request.Reference
	}
	if len(request.IdempotencyKey) > 128 {
		adminQuotaError(c, http.StatusBadRequest, "idempotency_key must not exceed 128 characters")
		return
	}

	user, err := model.GetUserById(request.UserId, false)
	if err != nil || user == nil {
		adminQuotaError(c, http.StatusNotFound, "user does not exist")
		return
	}
	entryType := model.QuotaLedgerTypeAdjustment
	if request.Action == "grant" {
		entryType = model.QuotaLedgerTypeGrant
	}
	metadata, _ := json.Marshal(map[string]string{
		"action":    request.Action,
		"reference": request.Reference,
	})
	entry, err := model.ApplyQuotaLedgerChange(model.QuotaLedgerChange{
		UserId:         user.Id,
		ActorUserId:    c.GetInt("id"),
		RequestId:      c.GetString(common.RequestIdKey),
		IdempotencyKey: request.IdempotencyKey,
		EntryType:      entryType,
		FundingSource:  model.QuotaFundingPublicBenefit,
		Amount:         request.Amount,
		Reason:         request.Reason,
		Metadata:       string(metadata),
	})
	if err != nil {
		switch {
		case errors.Is(err, model.ErrQuotaLedgerConflict):
			adminQuotaError(c, http.StatusConflict, "idempotency key was already used with different quota semantics")
			return
		case errors.Is(err, model.ErrQuotaLedgerOverdraw), errors.Is(err, model.ErrQuotaLedgerOutOfRange):
			adminQuotaError(c, http.StatusBadRequest, "quota change would make the balance invalid")
			return
		default:
			common.SysLog(fmt.Sprintf("admin quota change failed for user %d: %s", user.Id, err.Error()))
			adminQuotaError(c, http.StatusInternalServerError, "quota change failed")
			return
		}
	}

	auditAction := "user.quota_adjust"
	if request.Action == "grant" {
		auditAction = "user.quota_grant"
	}
	recordManageAuditFor(c, user.Id, auditAction, map[string]interface{}{
		"quota":         logger.LogQuota(request.Amount),
		"reason":        request.Reason,
		"reference":     request.Reference,
		"balance_after": entry.BalanceAfter,
	})
	common.ApiSuccess(c, gin.H{
		"entry_id":        entry.Id,
		"balance_after":   entry.BalanceAfter,
		"idempotency_key": entry.IdempotencyKey,
	})
}
