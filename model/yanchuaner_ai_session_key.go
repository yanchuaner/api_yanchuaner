/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

const aiWebSessionTokenNamePrefix = "yancore:ai-web:session:"

var ErrAiWebSessionKeyPolicy = errors.New("ai-web session key policy is invalid")

type YanCoreApplicationSession struct {
	Id          int64  `json:"id"`
	UserId      int    `json:"user_id" gorm:"uniqueIndex:idx_yancore_user_application;not null"`
	Application string `json:"application" gorm:"type:varchar(64);uniqueIndex:idx_yancore_user_application;not null"`
	TokenId     int    `json:"token_id" gorm:"index;not null"`
	GrantId     int64  `json:"grant_id" gorm:"index;not null"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime;index"`
}

func IsReservedYanCoreTokenName(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "yancore:")
}

func NormalizeAiWebSessionModels(models []string) ([]string, error) {
	if len(models) == 0 || len(models) > 32 {
		return nil, ErrAiWebSessionKeyPolicy
	}
	seen := make(map[string]bool, len(models))
	normalized := make([]string, 0, len(models))
	for _, candidate := range models {
		modelName := strings.TrimSpace(candidate)
		if modelName == "" || len(modelName) > 128 || strings.ContainsAny(modelName, ",\r\n\t") || seen[modelName] {
			return nil, ErrAiWebSessionKeyPolicy
		}
		seen[modelName] = true
		normalized = append(normalized, modelName)
	}
	return normalized, nil
}

// IssueAiWebSessionKey creates the only bearer credential used by the
// autonomous AI Web BFF. A successful login replaces older ai-web session
// keys while preserving their rows for usage-log and incident correlation.
func IssueAiWebSessionKey(userID int, grantID int64, expiresAt int64, quota int, models []string) (string, *Token, error) {
	if userID <= 0 || grantID <= 0 || expiresAt <= common.GetTimestamp() || quota <= 0 || quota > int(common.QuotaPerUnit) {
		return "", nil, ErrAiWebSessionKeyPolicy
	}
	normalizedModels, err := NormalizeAiWebSessionModels(models)
	if err != nil {
		return "", nil, err
	}
	presented, storedHash, prefix, suffix, err := GenerateVirtualKey()
	if err != nil {
		return "", nil, err
	}
	now := common.GetTimestamp()
	token := &Token{
		UserId:             userID,
		Key:                storedHash,
		KeyHashEnabled:     true,
		KeyDisplayPrefix:   prefix,
		KeyDisplaySuffix:   suffix,
		Status:             common.TokenStatusEnabled,
		Name:               fmt.Sprintf("%s%d", aiWebSessionTokenNamePrefix, grantID),
		CreatedTime:        now,
		AccessedTime:       now,
		ExpiredTime:        expiresAt,
		RemainQuota:        quota,
		UnlimitedQuota:     false,
		ModelLimitsEnabled: true,
		ModelLimits:        strings.Join(normalizedModels, ","),
	}
	tx := DB.Begin()
	if tx.Error != nil {
		return "", nil, tx.Error
	}
	if err := tx.Create(token).Error; err != nil {
		tx.Rollback()
		return "", nil, err
	}
	if YanCoreVirtualKeyPolicyEnabled() {
		policy, policyErr := BuildYanCoreVirtualKeyPolicy(token, nil)
		if policyErr != nil {
			tx.Rollback()
			return "", nil, policyErr
		}
		if policyErr = createYanCoreVirtualKeyPolicyWithTx(tx, token, policy, userID, "initial ai-web session policy"); policyErr != nil {
			tx.Rollback()
			return "", nil, policyErr
		}
	}
	session := &YanCoreApplicationSession{
		UserId:      userID,
		Application: "ai-web",
		TokenId:     token.Id,
		GrantId:     grantID,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "application"}},
		DoUpdates: clause.AssignmentColumns([]string{"token_id", "grant_id", "updated_at"}),
	}).Create(session).Error; err != nil {
		tx.Rollback()
		return "", nil, err
	}
	if err := tx.Where("user_id = ? AND id <> ? AND name LIKE ?", userID, token.Id, aiWebSessionTokenNamePrefix+"%").Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return "", nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return "", nil, err
	}
	if err := InvalidateUserTokensCache(userID); err != nil {
		_ = DB.Model(&Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error
		_ = InvalidateUserTokensCache(userID)
		return "", nil, err
	}
	return "sk-" + presented, token, nil
}
