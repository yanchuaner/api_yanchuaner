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

func TestYanCoreVirtualKeyPolicyAtomicallyUpdatesTokenProjection(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "policy-projection-user", Password: "password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{UserId: user.Id, Name: "old-name", Key: "sha256:policy-projection", KeyHashEnabled: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1"}
	policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(token, policy, user.Id, "initial policy"))

	name := "agent-key"
	quota := 2500
	expiry := time.Now().Add(24 * time.Hour).Unix()
	models := []string{"deepseek-reasoner", "deepseek-chat"}
	allowIPs := "192.0.2.7, 10.0.0.0/8"
	group := "default"
	crossGroupRetry := false
	updated, err := UpdateYanCoreVirtualKeyPolicy(token.Id, user.Id, user.Id, YanCoreVirtualKeyPolicyConfig{
		MaxRPM: 20,
		Reason: "update agent access policy",
		Token: &YanCoreVirtualKeyTokenUpdate{
			Name: &name, RemainQuota: &quota, ExpiredTime: &expiry, Models: &models,
			AllowIPs: &allowIPs, Group: &group, CrossGroupRetry: &crossGroupRetry,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "deepseek", updated.ProviderScope)
	assert.Equal(t, 2, updated.Version)

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, name, stored.Name)
	assert.Equal(t, quota, stored.RemainQuota)
	assert.Equal(t, expiry, stored.ExpiredTime)
	assert.Equal(t, "deepseek-chat,deepseek-reasoner", stored.ModelLimits)
	assert.Equal(t, "10.0.0.0/8\n192.0.2.7", stored.GetIpLimitsString())

	revisions, err := ListYanCoreVirtualKeyPolicyRevisions(token.Id, user.Id)
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	assert.Equal(t, stored.ModelLimits, revisions[0].ModelScope)
	assert.Equal(t, stored.GetIpLimitsString(), revisions[0].SourceScope)
	assert.Equal(t, stored.RemainQuota, revisions[0].BudgetQuota)
	assert.Equal(t, stored.ExpiredTime, revisions[0].ExpiresAt)
	assert.Equal(t, stored.Status, revisions[0].TokenStatus)
	assert.Equal(t, "update agent access policy", revisions[0].Reason)
}

func TestYanCoreVirtualKeyPolicyRejectsInvalidProjectionWithoutPartialWrite(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "policy-rollback-user", Password: "password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{UserId: user.Id, Name: "rollback-key", Key: "sha256:policy-rollback", KeyHashEnabled: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1"}
	policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(token, policy, user.Id, "initial policy"))

	invalidIPs := "not-an-ip"
	_, err = UpdateYanCoreVirtualKeyPolicy(token.Id, user.Id, user.Id, YanCoreVirtualKeyPolicyConfig{
		MaxRPM: 99, Reason: "invalid source scope",
		Token: &YanCoreVirtualKeyTokenUpdate{AllowIPs: &invalidIPs},
	})
	require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyInvalid)

	storedPolicy, err := GetYanCoreVirtualKeyPolicy(token.Id, user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, storedPolicy.Version)
	assert.Equal(t, YanCoreVirtualKeyPolicyDefaultRPM, storedPolicy.MaxRPM)
	revisions, err := ListYanCoreVirtualKeyPolicyRevisions(token.Id, user.Id)
	require.NoError(t, err)
	assert.Len(t, revisions, 1)
}

func TestYanCoreVirtualKeyPolicySynchronizesDisableState(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "policy-status-user", Password: "password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{UserId: user.Id, Name: "status-key", Key: "sha256:policy-status", KeyHashEnabled: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1"}
	policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(token, policy, user.Id, "initial policy"))

	disabled := common.TokenStatusDisabled
	updated, err := UpdateYanCoreVirtualKeyPolicy(token.Id, user.Id, user.Id, YanCoreVirtualKeyPolicyConfig{
		Reason: "disable compromised key",
		Token:  &YanCoreVirtualKeyTokenUpdate{Status: &disabled},
	})
	require.NoError(t, err)
	assert.Equal(t, YanCoreVirtualKeyPolicyDisabled, updated.Status)
	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, stored.Status)
}

func TestBuildYanCoreVirtualKeyPolicyRejectsAmbiguousModelProvider(t *testing.T) {
	token := &Token{UserId: 1, Name: "ambiguous", Key: "sha256:ambiguous", KeyHashEnabled: true, ModelLimitsEnabled: true, ModelLimits: "claude-3-7-sonnet"}
	_, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyInvalid)
}

func TestBuildYanCoreVirtualKeyPolicyRejectsUnsupportedOrWildcardActiveProvider(t *testing.T) {
	token := &Token{UserId: 1, Name: "provider-scope", Key: "sha256:provider-scope", KeyHashEnabled: true, RemainQuota: 100, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1-mini"}
	for _, providers := range [][]string{{"anthropic"}, {"*"}, {"openai", "*"}} {
		_, err := BuildYanCoreVirtualKeyPolicy(token, &YanCoreVirtualKeyPolicyConfig{Providers: providers})
		require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyInvalid)
	}
}

func TestBuildYanCoreVirtualKeyPolicyRejectsInvalidSourceScope(t *testing.T) {
	invalidIPs := "192.0.2.1\nnot-an-ip"
	token := &Token{UserId: 1, Name: "invalid-source", Key: "sha256:invalid-source", KeyHashEnabled: true, RemainQuota: 100, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1-mini", AllowIps: &invalidIPs}
	_, err := BuildYanCoreVirtualKeyPolicy(token, nil)
	require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyInvalid)
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

func TestYanCoreVirtualKeyPolicyRolloutRequiresExplicitReviewedTargets(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "policy-rollout-user", Password: "password", Status: common.UserStatusEnabled, Role: common.RoleAdminUser}
	require.NoError(t, DB.Create(user).Error)
	ready := &Token{UserId: user.Id, Name: "ready-key", Key: "sha256:ready-key", KeyHashEnabled: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "gpt-4.1-mini"}
	unsafe := &Token{UserId: user.Id, Name: "unsafe-key", Key: "sha256:unsafe-key", KeyHashEnabled: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "third-party-model"}
	managed := &Token{UserId: user.Id, Name: "managed-key", Key: "sha256:managed-key", KeyHashEnabled: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000, ModelLimitsEnabled: true, ModelLimits: "deepseek-chat"}
	require.NoError(t, DB.Create(ready).Error)
	require.NoError(t, DB.Create(unsafe).Error)
	managedPolicy, err := BuildYanCoreVirtualKeyPolicy(managed, nil)
	require.NoError(t, err)
	require.NoError(t, CreateYanCoreVirtualKeyWithPolicy(managed, managedPolicy, user.Id, "existing managed policy"))

	report, err := GetYanCoreVirtualKeyPolicyRolloutReport(10)
	require.NoError(t, err)
	assert.Equal(t, 3, report.TotalHashedKeys)
	assert.Equal(t, 1, report.ManagedActive)
	assert.Equal(t, 1, report.PendingReady)
	assert.Equal(t, 1, report.PendingReview)
	require.Len(t, report.Items, 2)
	for _, item := range report.Items {
		assert.NotContains(t, item.ModelScope, "sha256:")
	}
	_, err = ApplyYanCoreVirtualKeyPolicyRollout([]int{managed.Id, ready.Id}, user.Id, "stale reviewed rollout")
	require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyRolloutNotPending)
	pendingReady, err := GetYanCoreVirtualKeyPolicy(ready.Id, user.Id)
	require.NoError(t, err)
	assert.Nil(t, pendingReady)

	result, err := ApplyYanCoreVirtualKeyPolicyRollout([]int{unsafe.Id, ready.Id}, user.Id, "reviewed before local rollout")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Applied)
	assert.Equal(t, 1, result.Activated)
	assert.Equal(t, 1, result.Disabled)

	readyPolicy, err := GetYanCoreVirtualKeyPolicy(ready.Id, user.Id)
	require.NoError(t, err)
	require.NotNil(t, readyPolicy)
	assert.Equal(t, YanCoreVirtualKeyPolicyActive, readyPolicy.Status)
	unsafePolicy, err := GetYanCoreVirtualKeyPolicy(unsafe.Id, user.Id)
	require.NoError(t, err)
	require.NotNil(t, unsafePolicy)
	assert.Equal(t, YanCoreVirtualKeyPolicyDisabled, unsafePolicy.Status)
	var storedUnsafe Token
	require.NoError(t, DB.First(&storedUnsafe, unsafe.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, storedUnsafe.Status)
	unsafeRevisions, err := ListYanCoreVirtualKeyPolicyRevisions(unsafe.Id, user.Id)
	require.NoError(t, err)
	require.Len(t, unsafeRevisions, 1)
	assert.Contains(t, unsafeRevisions[0].Reason, "model_or_source_scope_ambiguous")
	assert.Contains(t, unsafeRevisions[0].Reason, "reviewed before local rollout")

	_, err = ApplyYanCoreVirtualKeyPolicyRollout([]int{ready.Id}, user.Id, "repeat reviewed rollout")
	require.ErrorIs(t, err, ErrYanCoreVirtualKeyPolicyRolloutNotPending)
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
