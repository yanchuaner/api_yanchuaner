package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openYanchuanerOAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CustomOAuthProvider{}, &model.UserOAuthBinding{}))
	return db
}

func TestTrustedOAuthRoleOnlyAcceptsYanchuanerProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider    *oauth.GenericOAuthProvider
		role        string
		expected    int
		managesRole bool
	}{
		{
			name:        "main site admin becomes root",
			provider:    oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "yanchuaner"}),
			role:        "admin",
			expected:    common.RoleRootUser,
			managesRole: true,
		},
		{
			name:        "verified student stays common user",
			provider:    oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "yanchuaner"}),
			role:        "student",
			expected:    common.RoleCommonUser,
			managesRole: true,
		},
		{
			name:        "other generic provider cannot assign roles",
			provider:    oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "other"}),
			role:        "admin",
			expected:    common.RoleCommonUser,
			managesRole: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			role, managesRole := trustedOAuthRole(testCase.provider, &oauth.OAuthUser{
				Extra: map[string]any{"role": testCase.role},
			})
			if role != testCase.expected || managesRole != testCase.managesRole {
				t.Fatalf("trustedOAuthRole() = (%d, %v), want (%d, %v)", role, managesRole, testCase.expected, testCase.managesRole)
			}
		})
	}
}

func TestAdoptTrustedRootOAuthUserKeepsBootstrapIdentity(t *testing.T) {
	db := openYanchuanerOAuthTestDB(t)
	root := &model.User{
		Username:    "yanchuaner",
		Password:    "legacy-password-hash",
		DisplayName: "Root User",
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
		Quota:       100000000,
	}
	require.NoError(t, db.Create(root).Error)
	providerConfig := &model.CustomOAuthProvider{Name: "燕中统一身份", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(providerConfig).Error)
	provider := oauth.NewGenericOAuthProvider(providerConfig)
	oauthUser := &oauth.OAuthUser{
		ProviderUserID: "main-admin-subject",
		Username:       "yanchuaner",
		DisplayName:    "燕中超级管理员",
		Email:          "yanchuaner@yanchuaner.cn",
		Extra:          map[string]any{"role": "admin"},
	}

	adoptedRoot, adopted, err := adoptTrustedRootOAuthUser(provider, oauthUser, common.RoleRootUser, true)
	require.NoError(t, err)
	require.True(t, adopted)
	require.Equal(t, root.Id, adoptedRoot.Id)

	var stored model.User
	require.NoError(t, db.First(&stored, root.Id).Error)
	require.Equal(t, "", stored.Password)
	require.Equal(t, "yanchuaner@yanchuaner.cn", stored.Email)
	require.Equal(t, "燕中超级管理员", stored.DisplayName)
	require.Equal(t, 100000000, stored.Quota)

	var binding model.UserOAuthBinding
	require.NoError(t, db.Where("user_id = ? AND provider_id = ?", root.Id, providerConfig.Id).First(&binding).Error)
	require.Equal(t, "main-admin-subject", binding.ProviderUserId)

	var userCount int64
	require.NoError(t, db.Model(&model.User{}).Count(&userCount).Error)
	require.EqualValues(t, 1, userCount)
}

func TestAdoptTrustedRootOAuthUserRejectsUntrustedOrConflictingIdentity(t *testing.T) {
	db := openYanchuanerOAuthTestDB(t)
	root := &model.User{
		Username: "yanchuaner",
		Password: "legacy-password-hash",
		Email:    "root@example.com",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(root).Error)

	otherConfig := &model.CustomOAuthProvider{Name: "Other", Slug: "other", Enabled: true}
	require.NoError(t, db.Create(otherConfig).Error)
	_, adopted, err := adoptTrustedRootOAuthUser(
		oauth.NewGenericOAuthProvider(otherConfig),
		&oauth.OAuthUser{ProviderUserID: "other-subject", Email: "root@example.com"},
		common.RoleRootUser,
		true,
	)
	require.NoError(t, err)
	require.False(t, adopted)

	trustedConfig := &model.CustomOAuthProvider{Name: "燕中统一身份", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(trustedConfig).Error)
	_, adopted, err = adoptTrustedRootOAuthUser(
		oauth.NewGenericOAuthProvider(trustedConfig),
		&oauth.OAuthUser{ProviderUserID: "main-subject", Email: "different@example.com"},
		common.RoleRootUser,
		true,
	)
	require.NoError(t, err)
	require.False(t, adopted)

	var bindingCount int64
	require.NoError(t, db.Model(&model.UserOAuthBinding{}).Count(&bindingCount).Error)
	require.Zero(t, bindingCount)
	var stored model.User
	require.NoError(t, db.First(&stored, root.Id).Error)
	require.Equal(t, "legacy-password-hash", stored.Password)
}
