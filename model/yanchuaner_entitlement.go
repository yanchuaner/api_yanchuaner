/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	YanCoreCampaignStatusEnabled      = "enabled"
	YanCoreCampaignStatusDisabled     = "disabled"
	YanCoreEntitlementSourceCampaign  = "campaign"
	YanCoreEntitlementStatusActive    = "active"
	YanCoreEntitlementStatusExhausted = "exhausted"
	YanCoreEntitlementStatusExpired   = "expired"
	YanCoreEntitlementLedgerGrant     = "grant"
)

var (
	ErrYanCoreCampaignInvalid     = errors.New("yancore campaign is invalid")
	ErrYanCoreCampaignExpired     = errors.New("yancore campaign is expired")
	ErrYanCoreCampaignExhausted   = errors.New("yancore campaign claim limit reached")
	ErrYanCoreRedeemCodeInvalid   = errors.New("yancore redeem code is invalid")
	ErrYanCoreRedeemCodeExpired   = errors.New("yancore redeem code is expired")
	ErrYanCoreRedeemCodeExhausted = errors.New("yancore redeem code claim limit reached")
	ErrYanCoreEntitlementTarget   = errors.New("yancore entitlement target is invalid")
	ErrYanCoreEntitlementReplayed = errors.New("yancore entitlement already claimed")
)

// YanCoreCampaign is the policy container for a separately accounted benefit.
// It intentionally does not reuse New API's Redemption table.
type YanCoreCampaign struct {
	Id            int64  `json:"id"`
	Name          string `json:"name" gorm:"type:varchar(128);not null"`
	FundingSource string `json:"funding_source" gorm:"type:varchar(32);index;not null"`
	Quota         int    `json:"quota" gorm:"not null"`
	ProviderScope string `json:"provider_scope" gorm:"type:text;not null"`
	ModelScope    string `json:"model_scope" gorm:"type:text;not null"`
	StartsAt      int64  `json:"starts_at" gorm:"index;not null"`
	ExpiresAt     int64  `json:"expires_at" gorm:"index;not null"`
	MaxClaims     int    `json:"max_claims" gorm:"not null"`
	ClaimedCount  int    `json:"claimed_count" gorm:"not null;default:0"`
	Status        string `json:"status" gorm:"type:varchar(16);index;not null"`
	CreatedBy     int    `json:"created_by" gorm:"index;not null"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

// YanCoreRedeemCode stores only a digest; plaintext is returned at creation
// time and cannot be recovered by operators or database readers.
type YanCoreRedeemCode struct {
	Id           int64  `json:"id"`
	CampaignId   int64  `json:"campaign_id" gorm:"index;not null"`
	CodeHash     string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	CodePrefix   string `json:"code_prefix" gorm:"type:varchar(16);not null"`
	CodeSuffix   string `json:"code_suffix" gorm:"type:char(4);not null"`
	MaxClaims    int    `json:"max_claims" gorm:"not null"`
	ClaimedCount int    `json:"claimed_count" gorm:"not null;default:0"`
	Status       string `json:"status" gorm:"type:varchar(16);index;not null"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type YanCoreEntitlement struct {
	Id             int64  `json:"id"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	CampaignId     int64  `json:"campaign_id" gorm:"index;not null"`
	RedeemCodeId   int64  `json:"redeem_code_id" gorm:"index;not null"`
	Source         string `json:"source" gorm:"type:varchar(32);index;not null"`
	GrantedQuota   int    `json:"granted_quota" gorm:"not null"`
	RemainingQuota int    `json:"remaining_quota" gorm:"not null"`
	ProviderScope  string `json:"provider_scope" gorm:"type:text;not null"`
	ModelScope     string `json:"model_scope" gorm:"type:text;not null"`
	ExpiresAt      int64  `json:"expires_at" gorm:"index;not null"`
	Status         string `json:"status" gorm:"type:varchar(16);index;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

// YanCoreEntitlementClaim is the immutable replay/audit record for a claim.
type YanCoreEntitlementClaim struct {
	Id             int64  `json:"id"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	CampaignId     int64  `json:"campaign_id" gorm:"uniqueIndex:idx_yancore_claim_campaign_user;not null"`
	RedeemCodeId   int64  `json:"redeem_code_id" gorm:"uniqueIndex:idx_yancore_claim_code_user;not null"`
	EntitlementId  int64  `json:"entitlement_id" gorm:"index;not null"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(160);uniqueIndex;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type YanCoreEntitlementLedgerEntry struct {
	Id             int64  `json:"id"`
	EntitlementId  int64  `json:"entitlement_id" gorm:"index;not null"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	EntryType      string `json:"entry_type" gorm:"type:varchar(32);index;not null"`
	Amount         int    `json:"amount" gorm:"not null"`
	BalanceAfter   int    `json:"balance_after" gorm:"not null"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(160);uniqueIndex;not null"`
	Metadata       string `json:"metadata" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func HashYanCoreRedeemCode(code string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(digest[:])
}

func GenerateYanCoreRedeemCode() (string, string, string, string, error) {
	secret := make([]byte, 18)
	if _, err := rand.Read(secret); err != nil {
		return "", "", "", "", err
	}
	code := "YC-" + strings.ToUpper(hex.EncodeToString(secret))
	return code, HashYanCoreRedeemCode(code), code[:11], code[len(code)-4:], nil
}

func validateYanCoreCampaign(campaign *YanCoreCampaign, now int64) error {
	if campaign == nil || strings.TrimSpace(campaign.Name) == "" || len(campaign.Name) > 128 ||
		campaign.FundingSource != YanCoreEntitlementSourceCampaign || campaign.Quota <= 0 ||
		campaign.StartsAt <= 0 || campaign.ExpiresAt <= campaign.StartsAt || campaign.MaxClaims <= 0 ||
		campaign.CreatedBy <= 0 {
		return ErrYanCoreCampaignInvalid
	}
	if campaign.Status != YanCoreCampaignStatusEnabled {
		return ErrYanCoreCampaignInvalid
	}
	if now < campaign.StartsAt || now >= campaign.ExpiresAt {
		return ErrYanCoreCampaignExpired
	}
	return nil
}

func CreateYanCoreCampaign(campaign *YanCoreCampaign) error {
	now := time.Now().Unix()
	if err := validateYanCoreCampaign(campaign, now); err != nil {
		return err
	}
	if campaign.ProviderScope == "" {
		campaign.ProviderScope = "*"
	}
	if campaign.ModelScope == "" {
		campaign.ModelScope = "*"
	}
	return DB.Create(campaign).Error
}

func CreateYanCoreRedeemCodes(campaignID int64, count, maxClaims int) ([]string, error) {
	if campaignID <= 0 || count <= 0 || count > 100 || maxClaims <= 0 {
		return nil, ErrYanCoreRedeemCodeInvalid
	}
	var campaign YanCoreCampaign
	if err := DB.First(&campaign, campaignID).Error; err != nil {
		return nil, err
	}
	if campaign.Status != YanCoreCampaignStatusEnabled {
		return nil, ErrYanCoreCampaignInvalid
	}
	if maxClaims > campaign.MaxClaims {
		return nil, ErrYanCoreRedeemCodeInvalid
	}
	result := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code, hash, prefix, suffix, err := GenerateYanCoreRedeemCode()
		if err != nil {
			return nil, err
		}
		row := &YanCoreRedeemCode{CampaignId: campaignID, CodeHash: hash, CodePrefix: prefix, CodeSuffix: suffix, MaxClaims: maxClaims, Status: YanCoreCampaignStatusEnabled}
		if err := DB.Create(row).Error; err != nil {
			return nil, err
		}
		result = append(result, code)
	}
	return result, nil
}

// ClaimYanCoreRedeemCode atomically creates an entitlement, records its claim,
// appends both ledgers, and updates the compatibility user quota projection.
// It returns the existing entitlement on an identical replay.
func ClaimYanCoreRedeemCode(userID int, presentedCode string) (*YanCoreEntitlement, bool, error) {
	if userID <= 0 || strings.TrimSpace(presentedCode) == "" || len(presentedCode) > 128 {
		return nil, false, ErrYanCoreRedeemCodeInvalid
	}
	var entitlement *YanCoreEntitlement
	replayed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var code YanCoreRedeemCode
		if err := lockForUpdate(tx).Where("code_hash = ?", HashYanCoreRedeemCode(presentedCode)).First(&code).Error; err != nil {
			return ErrYanCoreRedeemCodeInvalid
		}
		var campaign YanCoreCampaign
		if err := lockForUpdate(tx).First(&campaign, code.CampaignId).Error; err != nil {
			return ErrYanCoreCampaignInvalid
		}
		var user User
		if err := lockForUpdate(tx).Select("id", "status").First(&user, userID).Error; err != nil || user.Status != common.UserStatusEnabled {
			return ErrYanCoreEntitlementTarget
		}
		idempotencyKey := fmt.Sprintf("yancore:claim:%d:%d:%d", userID, campaign.Id, code.Id)
		var existingClaim YanCoreEntitlementClaim
		if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&existingClaim).Error; err == nil {
			if err := tx.First(&entitlement, existingClaim.EntitlementId).Error; err != nil {
				return err
			}
			replayed = true
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := validateYanCoreCampaign(&campaign, time.Now().Unix()); err != nil {
			return err
		}
		if code.Status != YanCoreCampaignStatusEnabled {
			return ErrYanCoreRedeemCodeInvalid
		}
		if code.MaxClaims <= code.ClaimedCount {
			return ErrYanCoreRedeemCodeExhausted
		}
		if campaign.MaxClaims <= campaign.ClaimedCount {
			return ErrYanCoreCampaignExhausted
		}
		var campaignClaim YanCoreEntitlementClaim
		if err := tx.Where("campaign_id = ? AND user_id = ?", campaign.Id, userID).First(&campaignClaim).Error; err == nil {
			if err := tx.First(&entitlement, campaignClaim.EntitlementId).Error; err != nil {
				return err
			}
			replayed = true
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		entitlement = &YanCoreEntitlement{UserId: userID, CampaignId: campaign.Id, RedeemCodeId: code.Id, Source: campaign.FundingSource, GrantedQuota: campaign.Quota, RemainingQuota: campaign.Quota, ProviderScope: campaign.ProviderScope, ModelScope: campaign.ModelScope, ExpiresAt: campaign.ExpiresAt, Status: YanCoreEntitlementStatusActive}
		if err := tx.Create(entitlement).Error; err != nil {
			return err
		}
		claim := &YanCoreEntitlementClaim{UserId: userID, CampaignId: campaign.Id, RedeemCodeId: code.Id, EntitlementId: entitlement.Id, IdempotencyKey: idempotencyKey}
		if err := tx.Create(claim).Error; err != nil {
			return err
		}
		metadataBytes, err := common.Marshal(map[string]any{"campaign_id": campaign.Id, "redeem_code_id": code.Id, "claim_id": claim.Id})
		if err != nil {
			return err
		}
		if _, err := applyQuotaLedgerChangeWithTx(tx, QuotaLedgerChange{UserId: userID, IdempotencyKey: idempotencyKey + ":quota", EntryType: QuotaLedgerTypeGrant, FundingSource: QuotaFundingCampaign, Amount: campaign.Quota, Reason: "YanCore campaign entitlement claim", Metadata: string(metadataBytes)}); err != nil {
			return err
		}
		if err := tx.Model(&YanCoreCampaign{}).Where("id = ?", campaign.Id).Updates(map[string]any{"claimed_count": gorm.Expr("claimed_count + 1")}).Error; err != nil {
			return err
		}
		if err := tx.Model(&YanCoreRedeemCode{}).Where("id = ?", code.Id).Updates(map[string]any{"claimed_count": gorm.Expr("claimed_count + 1")}).Error; err != nil {
			return err
		}
		return tx.Create(&YanCoreEntitlementLedgerEntry{EntitlementId: entitlement.Id, UserId: userID, EntryType: YanCoreEntitlementLedgerGrant, Amount: campaign.Quota, BalanceAfter: campaign.Quota, IdempotencyKey: idempotencyKey + ":entitlement", Metadata: string(metadataBytes)}).Error
	})
	if err != nil {
		return nil, replayed, err
	}
	return entitlement, replayed, nil
}

func ListYanCoreEntitlements(userID int) ([]*YanCoreEntitlement, error) {
	if userID <= 0 {
		return nil, ErrYanCoreEntitlementTarget
	}
	var entitlements []*YanCoreEntitlement
	err := DB.Where("user_id = ?", userID).Order("id desc").Limit(100).Find(&entitlements).Error
	return entitlements, err
}
