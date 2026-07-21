/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCheckYanCoreVirtualKeyProviderUsesModelPrefixBeforeChannelType(t *testing.T) {
	t.Setenv("YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED", "true")
	c, _ := gin.CreateTestContext(nil)
	SetYanCoreVirtualKeyPolicyContext(c, &model.YanCoreVirtualKeyPolicy{TokenId: 1, UserId: 1, ProviderScope: "deepseek", Status: model.YanCoreVirtualKeyPolicyActive, Version: 1})
	require.NoError(t, CheckYanCoreVirtualKeyProvider(c, "deepseek-chat", constant.ChannelTypeOpenAI))
	require.ErrorIs(t, CheckYanCoreVirtualKeyProvider(c, "gpt-4.1", constant.ChannelTypeOpenAI), ErrYanCoreVirtualKeyProviderDenied)
}

func TestCheckYanCoreVirtualKeyProviderFailsClosedWithoutPolicy(t *testing.T) {
	t.Setenv("YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED", "true")
	c, _ := gin.CreateTestContext(nil)
	require.ErrorIs(t, CheckYanCoreVirtualKeyProvider(c, "gpt-4.1", constant.ChannelTypeOpenAI), ErrYanCoreVirtualKeyPolicyMissing)
}

func TestCheckYanCoreVirtualKeyRelayFormatAllowsOnlyMeteredTextPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	SetYanCoreVirtualKeyPolicyContext(c, &model.YanCoreVirtualKeyPolicy{TokenId: 1, UserId: 1, ProviderScope: "openai", Status: model.YanCoreVirtualKeyPolicyActive, Version: 1})

	for _, allowed := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		require.NoError(t, CheckYanCoreVirtualKeyRelayFormat(c, allowed))
	}
	for _, denied := range []types.RelayFormat{types.RelayFormatOpenAIImage, types.RelayFormatOpenAIAudio, types.RelayFormatOpenAIRealtime, types.RelayFormatEmbedding, types.RelayFormatTask, types.RelayFormatMjProxy} {
		require.ErrorIs(t, CheckYanCoreVirtualKeyRelayFormat(c, denied), ErrYanCoreVirtualKeyFormatDenied)
	}
}
