/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOAuthStyleUserInsertRecordsInitialPublicBenefitGrant(t *testing.T) {
	truncateTables(t)
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	previousQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 500000
	t.Cleanup(func() { common.QuotaForNewUser = previousQuota })

	user := &User{
		Username: "oauth-ledger-user",
		Password: "password",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 0)
	}))
	assert.Equal(t, 500000, user.Quota)

	var entry QuotaLedgerEntry
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&entry).Error)
	assert.Equal(t, QuotaLedgerTypeGrant, entry.EntryType)
	assert.Equal(t, QuotaFundingPublicBenefit, entry.FundingSource)
	assert.Equal(t, 500000, entry.Amount)
	assert.Equal(t, 500000, entry.BalanceAfter)
}

func TestQuotaLedgerChangeIsAtomicAndIdempotent(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&QuotaLedgerEntry{}))
	user := &User{Username: "ledger-user", Password: "password", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, DB.Create(user).Error)

	change := QuotaLedgerChange{
		UserId:         user.Id,
		TokenId:        9,
		RequestId:      "request-1",
		IdempotencyKey: "request-1:wallet:reserve",
		EntryType:      QuotaLedgerTypeReserve,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         -30,
		Reason:         "model request reserve",
	}
	entry, err := ApplyQuotaLedgerChange(change)
	require.NoError(t, err)
	assert.Equal(t, 70, entry.BalanceAfter)

	duplicate, err := ApplyQuotaLedgerChange(change)
	require.NoError(t, err)
	assert.Equal(t, entry.Id, duplicate.Id)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 70, reloaded.Quota)
	var count int64
	require.NoError(t, DB.Model(&QuotaLedgerEntry{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestBackfillQuotaLedgerOpeningBalanceIsIdempotent(t *testing.T) {
	truncateTables(t)
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	user := &User{Username: "ledger-backfill", Password: "password", Status: common.UserStatusEnabled, Quota: 42}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, BackfillQuotaLedgerOpeningBalances())
	require.NoError(t, BackfillQuotaLedgerOpeningBalances())

	var entries []QuotaLedgerEntry
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&entries).Error)
	require.Len(t, entries, 1)
	assert.Equal(t, QuotaLedgerTypeOpeningBalance, entries[0].EntryType)
	assert.Equal(t, QuotaFundingLegacy, entries[0].FundingSource)
	assert.Equal(t, 42, entries[0].Amount)
	assert.Equal(t, 42, entries[0].BalanceAfter)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 42, reloaded.Quota)
}

func TestQuotaLedgerRejectsOverdrawAndConflictingReplay(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&QuotaLedgerEntry{}))
	user := &User{Username: "ledger-overdraw", Password: "password", Status: common.UserStatusEnabled, Quota: 20}
	require.NoError(t, DB.Create(user).Error)

	base := QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: "request-2:wallet:reserve",
		EntryType:      QuotaLedgerTypeReserve,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         -10,
	}
	_, err := ApplyQuotaLedgerChange(base)
	require.NoError(t, err)

	conflict := base
	conflict.Amount = -11
	_, err = ApplyQuotaLedgerChange(conflict)
	assert.ErrorIs(t, err, ErrQuotaLedgerConflict)

	overdraw := base
	overdraw.IdempotencyKey = "request-3:wallet:reserve"
	overdraw.Amount = -100
	_, err = ApplyQuotaLedgerChange(overdraw)
	assert.ErrorIs(t, err, ErrQuotaLedgerOverdraw)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 10, reloaded.Quota)
}

func TestWalletFundingBalanceExcludesCampaignQuota(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "ledger-source-isolation", Password: "password", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, DB.Create(user).Error)

	_, err := ApplyQuotaLedgerChange(QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: "campaign-isolation:grant",
		EntryType:      QuotaLedgerTypeGrant,
		FundingSource:  QuotaFundingCampaign,
		Amount:         50,
	})
	require.NoError(t, err)

	walletBalance, err := GetWalletFundingBalance(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 100, walletBalance)
	_, err = ApplyQuotaLedgerChange(QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: "campaign-isolation:wallet-overdraw",
		EntryType:      QuotaLedgerTypeReserve,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         -101,
	})
	assert.ErrorIs(t, err, ErrQuotaLedgerOverdraw)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 150, reloaded.Quota)
}

func TestQuotaLedgerRejectsOutOfRangeAmountBeforeArithmetic(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "ledger-range", Password: "password", Status: common.UserStatusEnabled, Quota: 20}
	require.NoError(t, DB.Create(user).Error)

	_, err := ApplyQuotaLedgerChange(QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: "request-range:wallet:reserve",
		EntryType:      QuotaLedgerTypeReserve,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         int(^uint(0) >> 1),
	})
	assert.ErrorIs(t, err, ErrQuotaLedgerOutOfRange)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 20, reloaded.Quota)
}

func TestQuotaLedgerPostgresMigrationCompatibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres migration compatibility test")
	}

	postgresDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := postgresDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if postgresDB.Migrator().HasTable(&User{}) || postgresDB.Migrator().HasTable(&QuotaLedgerEntry{}) {
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
		_ = postgresDB.Migrator().DropTable(&QuotaLedgerEntry{})
		_ = postgresDB.Migrator().DropTable(&User{})
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})

	require.NoError(t, postgresDB.AutoMigrate(&User{}, &QuotaLedgerEntry{}))
	user := &User{Username: "postgres-ledger-user", Password: "password", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, postgresDB.Create(user).Error)

	entry, err := ApplyQuotaLedgerChange(QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: "postgres-migration:grant:v1",
		EntryType:      QuotaLedgerTypeGrant,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         25,
		Reason:         "postgres migration compatibility",
	})
	require.NoError(t, err)
	assert.Equal(t, 125, entry.BalanceAfter)

	var reloaded User
	require.NoError(t, postgresDB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 125, reloaded.Quota)
}

func TestGetQuotaLedgerEntriesByRequestIdScopesUser(t *testing.T) {
	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:quota_ledger_request?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&User{}, &QuotaLedgerEntry{}))
	userA := &User{Username: "request-a", Password: "x", Status: common.UserStatusEnabled, Quota: 1000, AffCode: "request-a-aff"}
	userB := &User{Username: "request-b", Password: "x", Status: common.UserStatusEnabled, Quota: 0, AffCode: "request-b-aff"}
	require.NoError(t, db.Create(userA).Error)
	require.NoError(t, db.Create(userB).Error)

	_, err = ApplyQuotaLedgerChange(QuotaLedgerChange{
		UserId:         userA.Id,
		RequestId:      "req-abc",
		IdempotencyKey: "req-abc:user-a:reserve",
		EntryType:      QuotaLedgerTypeReserve,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         -100,
		Reason:         "reserve",
	})
	require.NoError(t, err)
	_, err = ApplyQuotaLedgerChange(QuotaLedgerChange{
		UserId:         userA.Id,
		RequestId:      "req-abc",
		IdempotencyKey: "req-abc:user-a:settle",
		EntryType:      QuotaLedgerTypeSettlement,
		FundingSource:  QuotaFundingPublicBenefit,
		Amount:         40,
		Reason:         "settlement",
	})
	require.NoError(t, err)

	rowsA, err := GetQuotaLedgerEntriesByRequestId("req-abc", userA.Id)
	require.NoError(t, err)
	assert.Len(t, rowsA, 2)
	rowsB, err := GetQuotaLedgerEntriesByRequestId("req-abc", userB.Id)
	require.NoError(t, err)
	assert.Len(t, rowsB, 0)
	rowsAll, err := GetQuotaLedgerEntriesByRequestId("req-abc", 0)
	require.NoError(t, err)
	assert.Len(t, rowsAll, 2)
}
