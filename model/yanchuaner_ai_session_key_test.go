package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIssueAiWebSessionKeyRotatesAndStoresOnlyHash(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open("file:yancore_ai_session_key?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Token{}, &YanCoreApplicationSession{}))

	expiresAt := time.Now().Add(15 * time.Minute).Unix()
	firstKey, first, err := IssueAiWebSessionKey(7, 101, expiresAt, 50000, []string{"gpt-4.1-mini", "deepseek-chat"})
	require.NoError(t, err)
	assert.NotContains(t, first.Key, firstKey)
	assert.Equal(t, HashVirtualKey(firstKey[3:]), first.Key)

	secondKey, second, err := IssueAiWebSessionKey(7, 102, expiresAt, 25000, []string{"deepseek-chat"})
	require.NoError(t, err)
	assert.NotEqual(t, firstKey, secondKey)
	assert.Equal(t, 25000, second.RemainQuota)
	assert.Equal(t, []string{"deepseek-chat"}, second.GetModelLimits())

	var active int64
	require.NoError(t, db.Model(&Token{}).Where("user_id = ?", 7).Count(&active).Error)
	assert.Equal(t, int64(1), active)
	var total int64
	require.NoError(t, db.Unscoped().Model(&Token{}).Where("user_id = ?", 7).Count(&total).Error)
	assert.Equal(t, int64(2), total)
	var applicationSession YanCoreApplicationSession
	require.NoError(t, db.Where("user_id = ? AND application = ?", 7, "ai-web").First(&applicationSession).Error)
	assert.Equal(t, second.Id, applicationSession.TokenId)
	assert.Equal(t, int64(102), applicationSession.GrantId)
	_, err = GetTokenByPresentedKey(firstKey[3:], true)
	assert.Error(t, err)
	resolved, err := GetTokenByPresentedKey(secondKey[3:], true)
	require.NoError(t, err)
	assert.Equal(t, second.Id, resolved.Id)
	validated, err := ValidateUserToken(secondKey[3:])
	require.NoError(t, err)
	assert.Equal(t, second.Id, validated.Id)
}

func TestIssueAiWebSessionKeyRejectsUnboundedPolicy(t *testing.T) {
	expiresAt := time.Now().Add(15 * time.Minute).Unix()
	_, _, err := IssueAiWebSessionKey(7, 101, expiresAt, int(common.QuotaPerUnit)+1, []string{"gpt-4.1-mini"})
	assert.True(t, errors.Is(err, ErrAiWebSessionKeyPolicy))
	_, _, err = IssueAiWebSessionKey(7, 101, expiresAt, 50000, []string{"gpt-4.1-mini", "gpt-4.1-mini"})
	assert.True(t, errors.Is(err, ErrAiWebSessionKeyPolicy))
	assert.True(t, IsReservedYanCoreTokenName(" YanCore:ai-web:manual"))
}
