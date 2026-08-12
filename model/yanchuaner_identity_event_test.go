package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyYanCoreIdentityEventDisablesUserAndRevokesGrantsAndTokens(t *testing.T) {
	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_identity_event_disable?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&User{},
		&Token{},
		&YanCoreSubjectGrant{},
		&YanCoreIdentityEvent{},
	))

	provider := &CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)
	user := &User{Username: "member-disable", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "main-subject-disable"}).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "sha256:session-1", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: aiWebSessionTokenNamePrefix + "1", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "sha256:normal-1", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: "normal-token", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&YanCoreSubjectGrant{UserId: user.Id, Application: "ai-web", Audience: "yanchuaner-ai", Scopes: "chat:read chat:write", JTIHash: strings.Repeat("d", 64), ExpiresAt: now + 900}).Error)

	result, err := ApplyYanCoreIdentityEvent(provider.Id, YanCoreIdentityEventData{
		EventId:       "event-disable-000000000001",
		Subject:       "main-subject-disable",
		Event:         "account.disabled",
		AccountStatus: "DISABLED",
	})
	require.NoError(t, err)
	assert.False(t, result.AlreadyProcessed)
	assert.True(t, result.Bound)
	assert.True(t, result.UserStatusChanged)
	assert.EqualValues(t, 2, result.TokensDisabled)
	assert.EqualValues(t, 1, result.GrantsRevoked)

	var storedUser User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, storedUser.Status)
	var enabledTokens int64
	require.NoError(t, db.Model(&Token{}).Where("user_id = ? AND status = ?", user.Id, common.TokenStatusEnabled).Count(&enabledTokens).Error)
	assert.Zero(t, enabledTokens)
	var activeGrants int64
	require.NoError(t, db.Model(&YanCoreSubjectGrant{}).Where("user_id = ? AND revoked_at = 0", user.Id).Count(&activeGrants).Error)
	assert.Zero(t, activeGrants)

	// Idempotent replay does not disable anything a second time.
	replay, err := ApplyYanCoreIdentityEvent(provider.Id, YanCoreIdentityEventData{
		EventId:       "event-disable-000000000001",
		Subject:       "main-subject-disable",
		Event:         "account.disabled",
		AccountStatus: "DISABLED",
	})
	require.NoError(t, err)
	assert.True(t, replay.AlreadyProcessed)
	assert.False(t, replay.Bound)
}

func TestApplyYanCoreIdentityEventSyncsRoleAndRevokesAiWebSession(t *testing.T) {
	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_identity_event_role?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&User{},
		&Token{},
		&YanCoreSubjectGrant{},
		&YanCoreIdentityEvent{},
	))

	provider := &CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)
	user := &User{Username: "member-role", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "main-subject-role"}).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "sha256:session-2", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: aiWebSessionTokenNamePrefix + "2", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "sha256:normal-2", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: "normal-token", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&YanCoreSubjectGrant{UserId: user.Id, Application: "ai-web", Audience: "yanchuaner-ai", Scopes: "chat:read chat:write", JTIHash: strings.Repeat("e", 64), ExpiresAt: now + 900}).Error)

	result, err := ApplyYanCoreIdentityEvent(provider.Id, YanCoreIdentityEventData{
		EventId:       "event-role-00000000000001",
		Subject:       "main-subject-role",
		Event:         "role.changed",
		Role:          "admin",
		AccountStatus: "ACTIVE",
	})
	require.NoError(t, err)
	assert.True(t, result.RoleUpdated)
	assert.EqualValues(t, 1, result.TokensDisabled)
	assert.EqualValues(t, 1, result.GrantsRevoked)

	var storedUser User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, common.RoleRootUser, storedUser.Role)
	var enabledTokens int64
	require.NoError(t, db.Model(&Token{}).Where("user_id = ? AND status = ?", user.Id, common.TokenStatusEnabled).Count(&enabledTokens).Error)
	assert.EqualValues(t, 1, enabledTokens) // normal token remains enabled
}

func TestApplyYanCoreIdentityEventRecordsUnboundSubject(t *testing.T) {
	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_identity_event_unbound?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&User{},
		&YanCoreIdentityEvent{},
	))
	provider := &CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)

	result, err := ApplyYanCoreIdentityEvent(provider.Id, YanCoreIdentityEventData{
		EventId:       "event-unbound-00000000001",
		Subject:       "main-subject-unknown",
		Event:         "sessions.revoked",
		AccountStatus: "ACTIVE",
	})
	require.NoError(t, err)
	assert.False(t, result.Bound)
	assert.False(t, result.AlreadyProcessed)
	var count int64
	require.NoError(t, db.Model(&YanCoreIdentityEvent{}).Where("event_id = ?", "event-unbound-00000000001").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestApplyYanCoreIdentityEventSessionsRevokedKeepsUserAndRegularTokens(t *testing.T) {
	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_identity_event_sessions?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&User{},
		&Token{},
		&YanCoreSubjectGrant{},
		&YanCoreIdentityEvent{},
	))

	provider := &CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)
	user := &User{Username: "member-sessions", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "main-subject-sessions"}).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "sha256:session-3", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: aiWebSessionTokenNamePrefix + "3", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "sha256:normal-3", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: "normal-token", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&YanCoreSubjectGrant{UserId: user.Id, Application: "ai-web", Audience: "yanchuaner-ai", Scopes: "chat:read chat:write", JTIHash: strings.Repeat("a", 64), ExpiresAt: now + 900}).Error)

	result, err := ApplyYanCoreIdentityEvent(provider.Id, YanCoreIdentityEventData{
		EventId:       "event-sessions-0000000001",
		Subject:       "main-subject-sessions",
		Event:         "sessions.revoked",
		AccountStatus: "ACTIVE",
	})
	require.NoError(t, err)
	assert.False(t, result.UserStatusChanged)
	assert.EqualValues(t, 1, result.TokensDisabled)
	assert.EqualValues(t, 1, result.GrantsRevoked)

	var storedUser User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, storedUser.Status)
	var enabledTokens int64
	require.NoError(t, db.Model(&Token{}).Where("user_id = ? AND status = ?", user.Id, common.TokenStatusEnabled).Count(&enabledTokens).Error)
	assert.EqualValues(t, 1, enabledTokens)
}
