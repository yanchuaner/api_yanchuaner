/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// YanCoreIdentityEvent records identity events pushed by the main site so
// account disable / role changes can revoke existing grants and tokens
// immediately. EventId is unique so a webhook retry cannot be applied twice.
type YanCoreIdentityEvent struct {
	Id            int64  `json:"id" gorm:"primaryKey"`
	EventId       string `json:"event_id" gorm:"type:varchar(128);uniqueIndex;not null"`
	Subject       string `json:"subject" gorm:"type:varchar(128);index;not null"`
	Event         string `json:"event" gorm:"type:varchar(64);index;not null"`
	Role          string `json:"role" gorm:"type:varchar(32)"`
	AccountStatus string `json:"account_status" gorm:"type:varchar(32)"`
	Status        string `json:"status" gorm:"type:varchar(32)"`
	ReceivedAt    int64  `json:"received_at" gorm:"autoCreateTime;index"`
}

type YanCoreIdentityEventData struct {
	EventId       string
	Subject       string
	Event         string
	Role          string
	AccountStatus string
	Status        string
}

type YanCoreIdentityEventResult struct {
	AlreadyProcessed  bool  `json:"already_processed"`
	Bound             bool  `json:"bound"`
	UserId            int   `json:"user_id"`
	GrantsRevoked     int64 `json:"grants_revoked"`
	TokensDisabled    int64 `json:"tokens_disabled"`
	RoleUpdated       bool  `json:"role_updated"`
	UserStatusChanged bool  `json:"user_status_changed"`
}

var (
	ErrYanCoreIdentityEventIncomplete = errors.New("yan core identity event is incomplete")
	ErrYanCoreIdentityEventNotFound   = errors.New("yan core identity event not found")
)

func validYanCoreIdentityEventData(data YanCoreIdentityEventData) bool {
	if len(data.EventId) < 16 || len(data.EventId) > 128 || strings.ContainsAny(data.EventId, "\r\n") {
		return false
	}
	if strings.TrimSpace(data.Subject) == "" || len(data.Subject) > 128 || strings.ContainsAny(data.Subject, "\r\n") {
		return false
	}
	switch data.Event {
	case "account.disabled", "account.enabled", "sessions.revoked", "role.changed", "account.verified", "account.rejected":
		return true
	default:
		return false
	}
}

// ApplyYanCoreIdentityEvent records the event idempotently and, when the
// subject is bound to a local API user, applies the matching revocation /
// role sync inside the same transaction. Cache invalidation happens after
// commit so the database remains the source of truth even if Redis is down.
func ApplyYanCoreIdentityEvent(providerId int, data YanCoreIdentityEventData) (*YanCoreIdentityEventResult, error) {
	if !validYanCoreIdentityEventData(data) {
		return nil, ErrYanCoreIdentityEventIncomplete
	}
	result := &YanCoreIdentityEventResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&YanCoreIdentityEvent{}).Where("event_id = ?", data.EventId).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			result.AlreadyProcessed = true
			return nil
		}
		event := &YanCoreIdentityEvent{
			EventId:       data.EventId,
			Subject:       data.Subject,
			Event:         data.Event,
			Role:          strings.TrimSpace(data.Role),
			AccountStatus: strings.TrimSpace(data.AccountStatus),
			Status:        strings.TrimSpace(data.Status),
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		var binding UserOAuthBinding
		if err := tx.Where("provider_id = ? AND provider_user_id = ?", providerId, data.Subject).First(&binding).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				result.Bound = false
				return nil
			}
			return err
		}
		var user User
		if err := tx.First(&user, binding.UserId).Error; err != nil {
			return err
		}
		result.Bound = true
		result.UserId = user.Id
		now := time.Now().Unix()
		switch data.Event {
		case "account.disabled":
			if user.Status != common.UserStatusDisabled {
				if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error; err != nil {
					return err
				}
				result.UserStatusChanged = true
			}
			tokenResult := tx.Model(&Token{}).
				Where("user_id = ? AND status = ? AND deleted_at IS NULL", user.Id, common.TokenStatusEnabled).
				Update("status", common.TokenStatusDisabled)
			if err := tokenResult.Error; err != nil {
				return err
			}
			result.TokensDisabled = tokenResult.RowsAffected
			grantResult := tx.Model(&YanCoreSubjectGrant{}).
				Where("user_id = ? AND revoked_at = 0", user.Id).
				Update("revoked_at", now)
			if err := grantResult.Error; err != nil {
				return err
			}
			result.GrantsRevoked = grantResult.RowsAffected
		case "sessions.revoked":
			grantResult := tx.Model(&YanCoreSubjectGrant{}).
				Where("user_id = ? AND revoked_at = 0", user.Id).
				Update("revoked_at", now)
			if err := grantResult.Error; err != nil {
				return err
			}
			result.GrantsRevoked = grantResult.RowsAffected
			tokenResult := tx.Model(&Token{}).
				Where("user_id = ? AND name LIKE ? AND status = ? AND deleted_at IS NULL", user.Id, aiWebSessionTokenNamePrefix+"%", common.TokenStatusEnabled).
				Update("status", common.TokenStatusDisabled)
			if err := tokenResult.Error; err != nil {
				return err
			}
			result.TokensDisabled = tokenResult.RowsAffected
		case "account.enabled":
			if user.Status != common.UserStatusEnabled {
				if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("status", common.UserStatusEnabled).Error; err != nil {
					return err
				}
				result.UserStatusChanged = true
			}
		case "role.changed", "account.verified", "account.rejected":
			desiredRole := common.RoleCommonUser
			if strings.EqualFold(strings.TrimSpace(data.Role), "admin") {
				desiredRole = common.RoleRootUser
			}
			if user.Role != desiredRole {
				if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("role", desiredRole).Error; err != nil {
					return err
				}
				result.RoleUpdated = true
			}
			grantResult := tx.Model(&YanCoreSubjectGrant{}).
				Where("user_id = ? AND revoked_at = 0", user.Id).
				Update("revoked_at", now)
			if err := grantResult.Error; err != nil {
				return err
			}
			result.GrantsRevoked = grantResult.RowsAffected
			tokenResult := tx.Model(&Token{}).
				Where("user_id = ? AND name LIKE ? AND status = ? AND deleted_at IS NULL", user.Id, aiWebSessionTokenNamePrefix+"%", common.TokenStatusEnabled).
				Update("status", common.TokenStatusDisabled)
			if err := tokenResult.Error; err != nil {
				return err
			}
			result.TokensDisabled = tokenResult.RowsAffected
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.UserId > 0 {
		if cacheErr := InvalidateUserCache(result.UserId); cacheErr != nil {
			common.SysLog("failed to invalidate user cache for identity event: " + cacheErr.Error())
		}
		if cacheErr := InvalidateUserTokensCache(result.UserId); cacheErr != nil {
			common.SysLog("failed to invalidate token cache for identity event: " + cacheErr.Error())
		}
	}
	return result, nil
}

func GetYanCoreIdentityEvent(eventId string) (*YanCoreIdentityEvent, error) {
	var event YanCoreIdentityEvent
	if err := DB.Where("event_id = ?", eventId).First(&event).Error; err != nil {
		return nil, err
	}
	return &event, nil
}
