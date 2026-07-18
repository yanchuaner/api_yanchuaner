/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	QuotaLedgerTypeOpeningBalance = "opening_balance"
	QuotaLedgerTypeGrant          = "grant"
	QuotaLedgerTypeReserve        = "reserve"
	QuotaLedgerTypeSettlement     = "settlement"
	QuotaLedgerTypeRefund         = "refund"
	QuotaLedgerTypeAdjustment     = "adjustment"
	QuotaLedgerTypeLegacy         = "legacy"

	QuotaFundingPublicBenefit = "public_benefit"
	QuotaFundingLegacy        = "legacy"
)

var (
	ErrQuotaLedgerConflict   = errors.New("quota ledger idempotency conflict")
	ErrQuotaLedgerOverdraw   = errors.New("quota ledger balance would become negative")
	ErrQuotaLedgerOutOfRange = errors.New("quota ledger balance is out of range")
)

// QuotaLedgerEntry is append-only application data. No update or delete API is
// provided; users.quota remains a transactionally maintained compatibility
// projection while migrated Yanchuaner paths use this ledger as their source.
type QuotaLedgerEntry struct {
	Id             int64  `json:"id"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	TokenId        int    `json:"token_id" gorm:"index;not null;default:0"`
	ActorUserId    int    `json:"actor_user_id" gorm:"index;not null;default:0"`
	RequestId      string `json:"request_id" gorm:"type:varchar(64);index;not null;default:''"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(128);uniqueIndex;not null"`
	EntryType      string `json:"entry_type" gorm:"type:varchar(32);index;not null"`
	FundingSource  string `json:"funding_source" gorm:"type:varchar(32);index;not null"`
	Amount         int    `json:"amount" gorm:"not null"`
	BalanceAfter   int    `json:"balance_after" gorm:"not null"`
	Reason         string `json:"reason" gorm:"type:varchar(255);not null;default:''"`
	Metadata       string `json:"metadata,omitempty" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type QuotaLedgerChange struct {
	UserId         int
	TokenId        int
	ActorUserId    int
	RequestId      string
	IdempotencyKey string
	EntryType      string
	FundingSource  string
	Amount         int
	Reason         string
	Metadata       string
}

func QuotaLedgerEnabled() bool {
	return common.GetEnvOrDefaultBool("YANCHUANER_QUOTA_LEDGER_ENABLED", false)
}

func validateQuotaLedgerChange(change QuotaLedgerChange) error {
	if change.UserId <= 0 {
		return errors.New("quota ledger user id is required")
	}
	if change.Amount == 0 {
		return errors.New("quota ledger amount cannot be zero")
	}
	if change.Amount > math.MaxInt32 || change.Amount < -math.MaxInt32 {
		return ErrQuotaLedgerOutOfRange
	}
	if strings.TrimSpace(change.IdempotencyKey) == "" || len(change.IdempotencyKey) > 128 {
		return errors.New("quota ledger idempotency key is invalid")
	}
	if strings.TrimSpace(change.EntryType) == "" || len(change.EntryType) > 32 {
		return errors.New("quota ledger entry type is invalid")
	}
	if strings.TrimSpace(change.FundingSource) == "" || len(change.FundingSource) > 32 {
		return errors.New("quota ledger funding source is invalid")
	}
	if len(change.RequestId) > 64 || len(change.Reason) > 255 {
		return errors.New("quota ledger text field is too long")
	}
	return nil
}

func sameQuotaLedgerChange(existing *QuotaLedgerEntry, change QuotaLedgerChange) bool {
	return existing.UserId == change.UserId &&
		existing.TokenId == change.TokenId &&
		existing.ActorUserId == change.ActorUserId &&
		existing.RequestId == change.RequestId &&
		existing.EntryType == change.EntryType &&
		existing.FundingSource == change.FundingSource &&
		existing.Amount == change.Amount &&
		existing.Reason == change.Reason &&
		existing.Metadata == change.Metadata
}

func applyQuotaLedgerChangeWithTx(tx *gorm.DB, change QuotaLedgerChange) (*QuotaLedgerEntry, error) {
	if err := validateQuotaLedgerChange(change); err != nil {
		return nil, err
	}

	user := &User{}
	if err := lockForUpdate(tx).Select("id", "quota").First(user, change.UserId).Error; err != nil {
		return nil, err
	}

	// Serialize replays for the same user before checking the idempotency key.
	// Otherwise two concurrent transactions can both observe a missing entry.
	existing := &QuotaLedgerEntry{}
	err := tx.Where("idempotency_key = ?", change.IdempotencyKey).First(existing).Error
	if err == nil {
		if sameQuotaLedgerChange(existing, change) {
			return existing, nil
		}
		return nil, ErrQuotaLedgerConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	nextBalance := int64(user.Quota) + int64(change.Amount)
	if nextBalance < 0 {
		return nil, ErrQuotaLedgerOverdraw
	}
	if nextBalance > math.MaxInt32 {
		return nil, ErrQuotaLedgerOutOfRange
	}

	entry := &QuotaLedgerEntry{
		UserId:         change.UserId,
		TokenId:        change.TokenId,
		ActorUserId:    change.ActorUserId,
		RequestId:      change.RequestId,
		IdempotencyKey: change.IdempotencyKey,
		EntryType:      change.EntryType,
		FundingSource:  change.FundingSource,
		Amount:         change.Amount,
		BalanceAfter:   int(nextBalance),
		Reason:         change.Reason,
		Metadata:       change.Metadata,
	}
	if err := tx.Model(&User{}).Where("id = ?", change.UserId).Update("quota", entry.BalanceAfter).Error; err != nil {
		return nil, err
	}
	if err := tx.Create(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

// ApplyQuotaLedgerChange atomically appends a ledger entry and updates the
// compatibility balance projection. Repeating an identical idempotency key is
// a no-op; reusing it for different semantics is rejected.
func ApplyQuotaLedgerChange(change QuotaLedgerChange) (*QuotaLedgerEntry, error) {
	var entry *QuotaLedgerEntry
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		entry, err = applyQuotaLedgerChangeWithTx(tx, change)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := updateUserQuotaCache(change.UserId, entry.BalanceAfter); err != nil {
		common.SysLog(fmt.Sprintf("failed to update quota cache after ledger entry %d: %s", entry.Id, err.Error()))
	}
	return entry, nil
}

func recordInitialQuotaGrantWithTx(tx *gorm.DB, user *User, amount int) error {
	if amount <= 0 {
		return nil
	}
	entry, err := applyQuotaLedgerChangeWithTx(tx, QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: fmt.Sprintf("user:%d:initial-grant:v1", user.Id),
		EntryType:      QuotaLedgerTypeGrant,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         amount,
		Reason:         "initial public-benefit quota",
	})
	if err != nil {
		return err
	}
	user.Quota = entry.BalanceAfter
	return nil
}

// BackfillQuotaLedgerOpeningBalances records the pre-migration balance without
// changing it. This is deliberately idempotent and runs only when enabled.
func BackfillQuotaLedgerOpeningBalances() error {
	if !QuotaLedgerEnabled() {
		return nil
	}
	var users []User
	if err := DB.Select("id", "quota").Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		idempotencyKey := fmt.Sprintf("user:%d:opening-balance:v1", user.Id)
		if err := DB.Transaction(func(tx *gorm.DB) error {
			lockedUser := &User{}
			if err := lockForUpdate(tx).Select("id", "quota").First(lockedUser, user.Id).Error; err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&QuotaLedgerEntry{}).Where("user_id = ?", user.Id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			return tx.Create(&QuotaLedgerEntry{
				UserId:         lockedUser.Id,
				IdempotencyKey: idempotencyKey,
				EntryType:      QuotaLedgerTypeOpeningBalance,
				FundingSource:  QuotaFundingLegacy,
				Amount:         lockedUser.Quota,
				BalanceAfter:   lockedUser.Quota,
				Reason:         "pre-ledger compatibility balance",
				Metadata:       "",
			}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func GetUserQuotaLedger(userId int, startIdx int, num int) ([]*QuotaLedgerEntry, int64, error) {
	var entries []*QuotaLedgerEntry
	var total int64
	query := DB.Model(&QuotaLedgerEntry{}).Where("user_id = ?", userId)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Offset(startIdx).Limit(num).Find(&entries).Error; err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}
