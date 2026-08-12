/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const yanCoreIdentityEventSignaturePrefix = "sha256="

type yanCoreIdentityEventRequest struct {
	EventId       string `json:"event_id"`
	Subject       string `json:"subject"`
	Event         string `json:"event"`
	Role          string `json:"role"`
	AccountStatus string `json:"account_status"`
	Status        string `json:"status"`
	OccurredAt    int64  `json:"occurred_at"`
}

func yanCoreIdentityEventSecret() ([]byte, bool) {
	secret := strings.TrimSpace(common.GetEnvOrDefaultString("YANCHUANER_IDENTITY_EVENT_SECRET", ""))
	if len(secret) < 32 {
		return nil, false
	}
	return []byte(secret), true
}

func yanCoreIdentityEventSignature(secret []byte, body []byte) string {
	return common.GenerateHMACWithKey(secret, string(body))
}

func verifyYanCoreIdentityEventSignature(secret []byte, body []byte, header string) bool {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, yanCoreIdentityEventSignaturePrefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, yanCoreIdentityEventSignaturePrefix))
	expected := yanCoreIdentityEventSignature(secret, body)
	return provided != "" && hmac.Equal([]byte(provided), []byte(expected))
}

func identityEventTimeDelta(now int64, value int64) int64 {
	delta := now - value
	if delta < 0 {
		return -delta
	}
	return delta
}

// HandleYanCoreIdentityEvent receives signed identity events from the main
// site (account disable/enable, session revocation and role changes) and
// applies them idempotently to grants, tokens and the local user role/status.
func HandleYanCoreIdentityEvent(c *gin.Context) {
	secret, enabled := yanCoreIdentityEventSecret()
	if !enabled {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore identity events are disabled."})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 32*1024+1))
	if err != nil || len(body) == 0 || len(body) > 32*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "identity event body is invalid."})
		return
	}
	eventTimeHeader := strings.TrimSpace(c.GetHeader("X-Yanchuaner-Event-Time"))
	eventTime, err := strconv.ParseInt(eventTimeHeader, 10, 64)
	if err != nil || eventTime <= 0 || identityEventTimeDelta(time.Now().Unix(), eventTime) > 300 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "identity event time is invalid or expired."})
		return
	}
	if !verifyYanCoreIdentityEventSignature(secret, body, c.GetHeader("X-Yanchuaner-Signature")) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "identity event signature is invalid."})
		return
	}
	var request yanCoreIdentityEventRequest
	if err := common.Unmarshal(body, &request); err != nil || request.OccurredAt != eventTime {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "identity event payload is invalid."})
		return
	}
	provider, err := model.GetCustomOAuthProviderBySlug("yanchuaner")
	if err != nil || provider == nil || !provider.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "YanCore identity binding is unavailable."})
		return
	}
	data := model.YanCoreIdentityEventData{
		EventId:       request.EventId,
		Subject:       request.Subject,
		Event:         request.Event,
		Role:          request.Role,
		AccountStatus: request.AccountStatus,
		Status:        request.Status,
	}
	result, err := model.ApplyYanCoreIdentityEvent(provider.Id, data)
	if err != nil {
		if errors.Is(err, model.ErrYanCoreIdentityEventIncomplete) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "identity event payload is invalid."})
			return
		}
		common.SysLog(fmt.Sprintf("failed to apply main-site identity event %s: %s", request.EventId, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "identity event could not be applied."})
		return
	}
	if result.AlreadyProcessed {
		c.JSON(http.StatusOK, gin.H{"success": true, "already_processed": true})
		return
	}
	if result.Bound && result.UserId > 0 {
		model.RecordOperationAuditLog(
			result.UserId,
			fmt.Sprintf("Applied main-site identity event %s for subject %s", request.Event, request.Subject),
			c.ClientIP(),
			"yancore.identity-event.received",
			map[string]interface{}{
				"event_id":       request.EventId,
				"event":          request.Event,
				"subject":        request.Subject,
				"target_user_id": result.UserId,
			},
			nil,
			map[string]interface{}{
				"identity_event_id": request.EventId,
				"grants_revoked":    result.GrantsRevoked,
				"tokens_disabled":   result.TokensDisabled,
				"role_updated":      result.RoleUpdated,
			},
		)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
