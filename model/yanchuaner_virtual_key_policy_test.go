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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestYanCoreVirtualKeyPolicyCreationUpdateAndRevisionAudit(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "policy-user", Password: "password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{UserId: user.Id, Name: "policy-key", Key: "sha256:policy-key", KeyHashEnabled: true, Status: common.TokenStatusEnabled, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1,deepseek-chat"}
	policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.Equal(t, "deepseek,openai", policy.ProviderScope)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(token, policy, user.Id, "initial policy"))

	stored, err := GetYanCoreVirtualKeyPolicy(token.Id, user.Id)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.True(t, stored.AllowsProvider("OpenAI"))
	assert.True(t, stored.AllowsProvider("deepseek"))
	assert.False(t, stored.AllowsProvider("anthropic"))

	updated, err := UpdateYanCoreVirtualKeyPolicy(token.Id, user.Id, user.Id, YanCoreVirtualKeyPolicyConfig{Providers: []string{"openai", "deepseek"}, MaxRPM: 12, MaxTPM: 2000, MaxConcurrency: 1, Reason: "tighten rate limits"})
	require.NoError(t, err)
	require.Equal(t, 2, updated.Version)
	require.Equal(t, "deepseek,openai", updated.ProviderScope)
	assert.Equal(t, 12, updated.MaxRPM)

	revisions, err := ListYanCoreVirtualKeyPolicyRevisions(token.Id, user.Id)
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	assert.Equal(t, "initial policy", revisions[1].Reason)
	assert.Equal(t, "tighten rate limits", revisions[0].Reason)
	assert.Equal(t, token.ModelLimits, revisions[0].ModelScope)
}

func TestBuildYanCoreVirtualKeyPolicyRejectsAmbiguousModelProvider(t *testing.T) {
	token := &Token{UserId: 1, Name: "ambiguous", Key: "sha256:ambiguous", KeyHashEnabled: true, ModelLimitsEnabled: true, ModelLimits: "claude-3-7-sonnet"}
	_, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyInvalid)
}

func TestBuildYanCoreVirtualKeyPolicyRejectsUnsupportedOrWildcardActiveProvider(t *testing.T) {
	token := &Token{UserId: 1, Name: "provider-scope", Key: "sha256:provider-scope", KeyHashEnabled: true, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1-mini"}
	for _, providers := range [][]string{{"anthropic"}, {"*"}, {"openai", "*"}} {
		_, err := BuildYanCoreVirtualKeyPolicy(token, &YanCoreVirtualKeyPolicyConfig{Providers: providers})
		require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyInvalid)
	}
}

func TestBackfillYanCoreVirtualKeyPoliciesDisablesAmbiguousLegacyKey(t *testing.T) {
	truncateTables(t)
	t.Setenv("YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED", "true")
	user := &User{Username: "legacy-policy-user", Password: "password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{UserId: user.Id, Name: "legacy-policy-key", Key: "sha256:legacy-policy", KeyHashEnabled: true, Status: common.TokenStatusEnabled, RemainQuota: 100, ModelLimitsEnabled: true, ModelLimits: "third-party-model"}
	require.NoError(t, DB.Create(token).Error)
	require.NoError(t, BackfillYanCoreVirtualKeyPolicies())
	policy, err := GetYanCoreVirtualKeyPolicy(token.Id, user.Id)
	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, YanCoreVirtualKeyPolicyDisabled, policy.Status)
	assert.Equal(t, "*", policy.ProviderScope)
}

func TestYanCoreVirtualKeyPolicyPostgresMigrationCompatibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL migration compatibility test")
	}

	postgresDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := postgresDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if postgresDB.Migrator().HasTable(&Token{}) || postgresDB.Migrator().HasTable(&YanCoreVirtualKeyPolicy{}) {
		t.Skip("refusing to run migration compatibility test against a non-empty database")
	}

	originalDB := DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	DB = postgresDB
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, originalLogType)
	t.Cleanup(func() {
		_ = postgresDB.Migrator().DropTable(&YanCoreVirtualKeyPolicyRevision{}, &YanCoreVirtualKeyPolicy{}, &Token{})
		DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})

	require.NoError(t, postgresDB.AutoMigrate(&Token{}, &YanCoreVirtualKeyPolicy{}, &YanCoreVirtualKeyPolicyRevision{}))
	token := &Token{UserId: 9021, Name: "postgres-policy", Key: "sha256:postgres-policy", KeyHashEnabled: true, Status: common.TokenStatusEnabled, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "deepseek-chat"}
	policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(token, policy, token.UserId, "postgres initial policy"))
	updated, err := UpdateYanCoreVirtualKeyPolicy(token.Id, token.UserId, token.UserId, YanCoreVirtualKeyPolicyConfig{MaxRPM: 10, Reason: "postgres policy update"})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
	revisions, err := ListYanCoreVirtualKeyPolicyRevisions(token.Id, token.UserId)
	require.NoError(t, err)
	assert.Len(t, revisions, 2)
}

func TestYanCoreVirtualKeyPolicyMySQLMigrationCompatibility(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run MySQL migration compatibility test")
	}

	mysqlDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := mysqlDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if mysqlDB.Migrator().HasTable(&Token{}) || mysqlDB.Migrator().HasTable(&YanCoreVirtualKeyPolicy{}) {
		t.Skip("refusing to run migration compatibility test against a non-empty database")
	}

	originalDB := DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	DB = mysqlDB
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, originalLogType)
	t.Cleanup(func() {
		_ = mysqlDB.Migrator().DropTable(&YanCoreVirtualKeyPolicyRevision{}, &YanCoreVirtualKeyPolicy{}, &Token{})
		DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})

	require.NoError(t, mysqlDB.AutoMigrate(&Token{}, &YanCoreVirtualKeyPolicy{}, &YanCoreVirtualKeyPolicyRevision{}))
	token := &Token{UserId: 9022, Name: "mysql-policy", Key: "sha256:mysql-policy", KeyHashEnabled: true, Status: common.TokenStatusEnabled, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1-mini"}
	policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(token, policy, token.UserId, "mysql initial policy"))
	updated, err := UpdateYanCoreVirtualKeyPolicy(token.Id, token.UserId, token.UserId, YanCoreVirtualKeyPolicyConfig{MaxConcurrency: 1, Reason: "mysql policy update"})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
	revisions, err := ListYanCoreVirtualKeyPolicyRevisions(token.Id, token.UserId)
	require.NoError(t, err)
	assert.Len(t, revisions, 2)
}
