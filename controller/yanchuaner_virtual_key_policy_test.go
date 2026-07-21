/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createPolicyManagedControllerToken(t *testing.T) (*model.Token, *model.YanCoreVirtualKeyPolicy) {
	t.Helper()
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.YanCoreVirtualKeyPolicy{}, &model.YanCoreVirtualKeyPolicyRevision{}))
	token := &model.Token{
		UserId: 7, Name: "controller-policy-key", Key: "sha256:controller-policy", KeyHashEnabled: true,
		Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000,
		ModelLimitsEnabled: true, ModelLimits: "gpt-4.1", Group: "default",
	}
	policy, err := model.BuildYanCoreVirtualKeyPolicy(token, nil)
	require.NoError(t, err)
	require.NoError(t, model.CreateYanCoreVirtualKeyWithPolicy(token, policy, token.UserId, "initial policy"))
	return token, policy
}

func TestUpdateYanCoreVirtualKeyPolicyUpdatesProjectionAndRevision(t *testing.T) {
	t.Setenv("YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED", "true")
	token, _ := createPolicyManagedControllerToken(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/yancore/virtual-key-policies/"+strconv.Itoa(token.Id), map[string]any{
		"max_rpm": 15,
		"reason":  "configure local agent key",
		"token": map[string]any{
			"name":         "local-agent",
			"remain_quota": 2500,
			"models":       []string{"deepseek-chat"},
			"allow_ips":    "127.0.0.1\n10.0.0.0/8",
		},
	}, token.UserId)
	ctx.Params = gin.Params{{Key: "token_id", Value: strconv.Itoa(token.Id)}}

	UpdateYanCoreVirtualKeyPolicy(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	assert.Equal(t, "local-agent", stored.Name)
	assert.Equal(t, 2500, stored.RemainQuota)
	assert.Equal(t, "deepseek-chat", stored.ModelLimits)
	policy, err := model.GetYanCoreVirtualKeyPolicy(token.Id, token.UserId)
	require.NoError(t, err)
	assert.Equal(t, "deepseek", policy.ProviderScope)
	assert.Equal(t, 2, policy.Version)
}

func TestUpdateTokenRejectsPolicyManagedVirtualKeyBypass(t *testing.T) {
	t.Setenv("YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED", "true")
	token, _ := createPolicyManagedControllerToken(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "bypass", "remain_quota": 9999,
	}, token.UserId)

	UpdateToken(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	assert.Equal(t, token.Name, stored.Name)
	assert.Equal(t, token.RemainQuota, stored.RemainQuota)
	revisions, err := model.ListYanCoreVirtualKeyPolicyRevisions(token.Id, token.UserId)
	require.NoError(t, err)
	assert.Len(t, revisions, 1)
}
