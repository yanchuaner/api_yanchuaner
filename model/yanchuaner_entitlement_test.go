/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClaimYanCoreRedeemCodeIsIdempotentAndSeparatelyAccounted(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	db, err := gorm.Open(sqlite.Open("file:yancore_entitlement_claim?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&User{}, &QuotaLedgerEntry{}, &YanCoreCampaign{}, &YanCoreRedeemCode{}, &YanCoreEntitlement{}, &YanCoreEntitlementClaim{}, &YanCoreEntitlementLedgerEntry{}))
	user := &User{Username: "entitlement-user", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	now := time.Now().Unix()
	campaign := &YanCoreCampaign{Name: "校友暑期公益", FundingSource: YanCoreEntitlementSourceCampaign, Quota: 700, ProviderScope: "deepseek", ModelScope: "deepseek-chat", StartsAt: now - 10, ExpiresAt: now + 3600, MaxClaims: 1, Status: YanCoreCampaignStatusEnabled, CreatedBy: user.Id}
	require.NoError(t, CreateYanCoreCampaign(campaign))
	codes, err := CreateYanCoreRedeemCodes(campaign.Id, 1, 1)
	require.NoError(t, err)
	require.Len(t, codes, 1)

	entitlement, replayed, err := ClaimYanCoreRedeemCode(user.Id, codes[0])
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, 700, entitlement.RemainingQuota)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 700, stored.Quota)
	var entry QuotaLedgerEntry
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&entry).Error)
	require.Equal(t, QuotaFundingCampaign, entry.FundingSource)
	var entitlementEntry YanCoreEntitlementLedgerEntry
	require.NoError(t, db.First(&entitlementEntry).Error)
	require.Equal(t, 700, entitlementEntry.BalanceAfter)

	replayedEntitlement, replayed, err := ClaimYanCoreRedeemCode(user.Id, codes[0])
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, entitlement.Id, replayedEntitlement.Id)
	var ledgerCount int64
	require.NoError(t, db.Model(&QuotaLedgerEntry{}).Count(&ledgerCount).Error)
	require.Equal(t, int64(1), ledgerCount)
}

func TestClaimYanCoreRedeemCodeRejectsSecondUserAfterCodeLimit(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	db, err := gorm.Open(sqlite.Open("file:yancore_entitlement_exhausted?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&User{}, &QuotaLedgerEntry{}, &YanCoreCampaign{}, &YanCoreRedeemCode{}, &YanCoreEntitlement{}, &YanCoreEntitlementClaim{}, &YanCoreEntitlementLedgerEntry{}))
	users := []*User{{Username: "entitlement-user-1", AffCode: "entitlement-1", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}, {Username: "entitlement-user-2", AffCode: "entitlement-2", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}}
	for _, user := range users {
		require.NoError(t, db.Create(user).Error)
	}
	now := time.Now().Unix()
	campaign := &YanCoreCampaign{Name: "一次性活动", FundingSource: YanCoreEntitlementSourceCampaign, Quota: 100, StartsAt: now - 10, ExpiresAt: now + 3600, MaxClaims: 1, Status: YanCoreCampaignStatusEnabled, CreatedBy: users[0].Id}
	require.NoError(t, CreateYanCoreCampaign(campaign))
	codes, err := CreateYanCoreRedeemCodes(campaign.Id, 1, 1)
	require.NoError(t, err)
	_, _, err = ClaimYanCoreRedeemCode(users[0].Id, codes[0])
	require.NoError(t, err)
	_, _, err = ClaimYanCoreRedeemCode(users[1].Id, codes[0])
	require.ErrorIs(t, err, ErrYanCoreRedeemCodeExhausted)
}
