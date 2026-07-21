/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func newVirtualKeyLimiterContext(policy *model.YanCoreVirtualKeyPolicy) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	SetYanCoreVirtualKeyPolicyContext(c, policy)
	return c
}

func TestYanCoreVirtualKeyLimiterEnforcesRPMTPMAndConcurrency(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		resetYanCoreVirtualKeyLimiterForTest()
	})
	resetYanCoreVirtualKeyLimiterForTest()

	policy := &model.YanCoreVirtualKeyPolicy{TokenId: 9001, UserId: 1, ProviderScope: "openai", MaxRPM: 2, MaxTPM: 100, MaxConcurrency: 1, Status: model.YanCoreVirtualKeyPolicyActive, Version: 1}
	first := newVirtualKeyLimiterContext(policy)
	require.NoError(t, BeginYanCoreVirtualKeyRequest(first, 40))
	require.NoError(t, RecordYanCoreVirtualKeyTokens(first, 20))
	second := newVirtualKeyLimiterContext(policy)
	require.ErrorIs(t, BeginYanCoreVirtualKeyRequest(second, 40), ErrYanCoreVirtualKeyConcurrencyExceeded)
	require.NoError(t, FinalizeYanCoreVirtualKeyRequest(first))

	third := newVirtualKeyLimiterContext(policy)
	require.ErrorIs(t, BeginYanCoreVirtualKeyRequest(third, 90), ErrYanCoreVirtualKeyTPMExceeded)
	fourth := newVirtualKeyLimiterContext(policy)
	require.NoError(t, BeginYanCoreVirtualKeyRequest(fourth, 80))
	require.NoError(t, FinalizeYanCoreVirtualKeyRequest(fourth))

	fifth := newVirtualKeyLimiterContext(policy)
	require.ErrorIs(t, BeginYanCoreVirtualKeyRequest(fifth, 1), ErrYanCoreVirtualKeyRPMExceeded)
}

func TestYanCoreVirtualKeyLimiterFailsClosedWhenRedisUnavailable(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	policy := &model.YanCoreVirtualKeyPolicy{TokenId: 9002, UserId: 1, ProviderScope: "openai", MaxRPM: 1, MaxTPM: 100, MaxConcurrency: 1, Status: model.YanCoreVirtualKeyPolicyActive, Version: 1}
	c := newVirtualKeyLimiterContext(policy)
	require.ErrorIs(t, BeginYanCoreVirtualKeyRequest(c, 1), ErrYanCoreVirtualKeyLimiterUnavailable)
}

func TestYanCoreVirtualKeyLimiterRedisAtomicityAndMidRequestFailure(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set TEST_REDIS_URL to run Redis limiter integration test")
	}
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.Ping(context.Background()).Err())

	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	tokenID := int(time.Now().UnixNano()%1000000000) + 1000000000
	minute := time.Now().Unix() / 60
	keys := []string{
		fmt.Sprintf("yancore:vkey:rpm:%d:%d", tokenID, minute),
		fmt.Sprintf("yancore:vkey:tpm:%d:%d", tokenID, minute),
		fmt.Sprintf("yancore:vkey:concurrency:%d", tokenID),
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), keys...).Err() })

	policy := &model.YanCoreVirtualKeyPolicy{TokenId: tokenID, UserId: 1, ProviderScope: "openai", MaxRPM: 3, MaxTPM: 100, MaxConcurrency: 1, Status: model.YanCoreVirtualKeyPolicyActive, Version: 1}
	first := newVirtualKeyLimiterContext(policy)
	require.NoError(t, BeginYanCoreVirtualKeyRequest(first, 40))
	second := newVirtualKeyLimiterContext(policy)
	require.ErrorIs(t, BeginYanCoreVirtualKeyRequest(second, 40), ErrYanCoreVirtualKeyConcurrencyExceeded)
	require.NoError(t, RecordYanCoreVirtualKeyTokens(first, 20))
	require.NoError(t, FinalizeYanCoreVirtualKeyRequest(first))

	third := newVirtualKeyLimiterContext(policy)
	require.NoError(t, BeginYanCoreVirtualKeyRequest(third, 40))
	common.RDB = nil
	require.ErrorIs(t, RecordYanCoreVirtualKeyTokens(third, 20), ErrYanCoreVirtualKeyLimiterUnavailable)
	require.ErrorIs(t, FinalizeYanCoreVirtualKeyRequest(third), ErrYanCoreVirtualKeyLimiterUnavailable)
	common.RDB = client
}
