/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func requestLedgerResponse(c *gin.Context, requestId string, userId int) {
	requestId = strings.TrimSpace(requestId)
	entries, err := model.GetQuotaLedgerEntriesByRequestId(requestId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summary := map[string]int{}
	for _, entry := range entries {
		summary[entry.EntryType] += entry.Amount
	}
	common.ApiSuccess(c, gin.H{
		"request_id": requestId,
		"entries":    entries,
		"summary":    summary,
	})
}

// GetMyRequestLedger returns ledger rows for the caller's own request id.
func GetMyRequestLedger(c *gin.Context) {
	requestLedgerResponse(c, c.Param("request_id"), c.GetInt("id"))
}

// GetAdminRequestLedger returns ledger rows for any user's request id.
func GetAdminRequestLedger(c *gin.Context) {
	if strings.TrimSpace(c.Param("request_id")) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "request_id is required"})
		return
	}
	requestLedgerResponse(c, c.Param("request_id"), 0)
}
