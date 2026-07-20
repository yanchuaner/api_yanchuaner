/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
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

	matched, err := FindYanCoreEntitlement(user.Id, "DeepSeek", "deepseek-chat", 300)
	require.NoError(t, err)
	require.Equal(t, entitlement.Id, matched.Id)
	change := YanCoreEntitlementChange{EntitlementId: entitlement.Id, UserId: user.Id, RequestId: "request-entitlement-1", IdempotencyKey: "request:entitlement:1:reserve", EntryType: YanCoreEntitlementLedgerReserve, Amount: -300, Reason: "test campaign reserve"}
	reserved, err := ApplyYanCoreEntitlementChange(change)
	require.NoError(t, err)
	require.Equal(t, change.RequestId, reserved.RequestId)
	duplicate, err := ApplyYanCoreEntitlementChange(change)
	require.NoError(t, err)
	require.Equal(t, reserved.Id, duplicate.Id)
	require.Equal(t, 400, reserved.BalanceAfter)
	require.NoError(t, db.Model(&QuotaLedgerEntry{}).Count(&ledgerCount).Error)
	require.Equal(t, int64(2), ledgerCount)
	campaignBalance, err := GetQuotaLedgerBalanceByFundingSource(user.Id, QuotaFundingCampaign)
	require.NoError(t, err)
	require.Equal(t, int64(400), campaignBalance)
	_, err = FindYanCoreEntitlement(user.Id, "DeepSeek", "deepseek-chat", 500)
	require.ErrorIs(t, err, ErrYanCoreEntitlementInsufficient)
	_, err = FindYanCoreEntitlement(user.Id, "OpenAI", "deepseek-chat", 1)
	require.ErrorIs(t, err, ErrYanCoreEntitlementNotFound)

	require.NoError(t, db.Model(&YanCoreEntitlement{}).Where("id = ?", entitlement.Id).Update("expires_at", time.Now().Unix()-1).Error)
	refunded, err := ApplyYanCoreEntitlementChange(YanCoreEntitlementChange{EntitlementId: entitlement.Id, UserId: user.Id, RequestId: "request-entitlement-refund", IdempotencyKey: "request:entitlement:1:refund", EntryType: YanCoreEntitlementLedgerRefund, Amount: 300, Reason: "test expired campaign refund"})
	require.NoError(t, err)
	require.Equal(t, 700, refunded.BalanceAfter)
	var expired YanCoreEntitlement
	require.NoError(t, db.First(&expired, entitlement.Id).Error)
	require.Equal(t, YanCoreEntitlementStatusExpired, expired.Status)
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

func TestYanCoreEntitlementPostgresMigrationCompatibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL migration compatibility test")
	}

	postgresDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := postgresDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if postgresDB.Migrator().HasTable(&User{}) || postgresDB.Migrator().HasTable(&YanCoreEntitlement{}) {
		t.Skip("refusing to run migration compatibility test against a non-empty database")
	}

	originalDB := DB
	originalLogDB := LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	DB = postgresDB
	LOG_DB = postgresDB
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	t.Cleanup(func() {
		_ = postgresDB.Migrator().DropTable(
			&YanCoreEntitlementLedgerEntry{},
			&YanCoreEntitlementClaim{},
			&YanCoreEntitlement{},
			&YanCoreRedeemCode{},
			&YanCoreCampaign{},
			&QuotaLedgerEntry{},
			&User{},
		)
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})

	require.NoError(t, postgresDB.AutoMigrate(
		&User{},
		&QuotaLedgerEntry{},
		&YanCoreCampaign{},
		&YanCoreRedeemCode{},
		&YanCoreEntitlement{},
		&YanCoreEntitlementClaim{},
		&YanCoreEntitlementLedgerEntry{},
	))
	user := &User{Username: "entitlement-postgres", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, postgresDB.Create(user).Error)
	now := time.Now().Unix()
	campaign := &YanCoreCampaign{Name: "PostgreSQL migration", FundingSource: YanCoreEntitlementSourceCampaign, Quota: 100, ProviderScope: "deepseek", ModelScope: "deepseek-chat", StartsAt: now - 10, ExpiresAt: now + 3600, MaxClaims: 1, Status: YanCoreCampaignStatusEnabled, CreatedBy: user.Id}
	require.NoError(t, CreateYanCoreCampaign(campaign))
	codes, err := CreateYanCoreRedeemCodes(campaign.Id, 1, 1)
	require.NoError(t, err)
	entitlement, _, err := ClaimYanCoreRedeemCode(user.Id, codes[0])
	require.NoError(t, err)

	entry, err := ApplyYanCoreEntitlementChange(YanCoreEntitlementChange{
		EntitlementId:  entitlement.Id,
		UserId:         user.Id,
		RequestId:      "postgres-entitlement-request",
		IdempotencyKey: "postgres-entitlement:reserve:v1",
		EntryType:      YanCoreEntitlementLedgerReserve,
		Amount:         -25,
		Reason:         "PostgreSQL migration compatibility",
	})
	require.NoError(t, err)
	require.Equal(t, "postgres-entitlement-request", entry.RequestId)
	require.Equal(t, 75, entry.BalanceAfter)
}
