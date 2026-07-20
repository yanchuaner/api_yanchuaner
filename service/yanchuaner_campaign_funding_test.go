/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func seedCampaignFundingUser(t *testing.T, quota int) (*model.User, *model.YanCoreEntitlement) {
	t.Helper()
	user := &model.User{Username: fmt.Sprintf("campaign-funding-%d", time.Now().UnixNano()), AffCode: fmt.Sprintf("campaign-%d", time.Now().UnixNano()), Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, model.DB.Create(user).Error)
	now := time.Now().Unix()
	campaign := &model.YanCoreCampaign{Name: "provider-scoped-campaign", FundingSource: model.YanCoreEntitlementSourceCampaign, Quota: quota, ProviderScope: "deepseek", ModelScope: "deepseek-chat", StartsAt: now - 10, ExpiresAt: now + 3600, MaxClaims: 1, Status: model.YanCoreCampaignStatusEnabled, CreatedBy: user.Id}
	require.NoError(t, model.CreateYanCoreCampaign(campaign))
	codes, err := model.CreateYanCoreRedeemCodes(campaign.Id, 1, 1)
	require.NoError(t, err)
	entitlement, replayed, err := model.ClaimYanCoreRedeemCode(user.Id, codes[0])
	require.NoError(t, err)
	require.False(t, replayed)
	return user, entitlement
}

func campaignRelayInfo(userID int, modelName string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UserId: userID, OriginModelName: modelName, RequestId: fmt.Sprintf("campaign-request-%d", time.Now().UnixNano()), IsPlayground: true,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}
}

func TestNewBillingSessionUsesCampaignEntitlementAndSettlesIt(t *testing.T) {
	t.Setenv("YANCHUANER_CAMPAIGN_FUNDING_ENABLED", "true")
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	user, entitlement := seedCampaignFundingUser(t, 100)
	defer func() {
		model.DB.Where("user_id = ?", user.Id).Delete(&model.QuotaLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementClaim{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlement{})
		model.DB.Delete(&model.YanCoreRedeemCode{}, entitlement.RedeemCodeId)
		model.DB.Delete(&model.YanCoreCampaign{}, entitlement.CampaignId)
		model.DB.Delete(&model.User{}, user.Id)
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := campaignRelayInfo(user.Id, "deepseek-chat")
	session, apiErr := NewBillingSession(c, relayInfo, 60)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	require.Equal(t, BillingSourceCampaign, relayInfo.BillingSource)
	require.NoError(t, session.Settle(40))

	var updated model.YanCoreEntitlement
	require.NoError(t, model.DB.First(&updated, entitlement.Id).Error)
	require.Equal(t, 60, updated.RemainingQuota)
	var campaignBalance int64
	campaignBalance, err := model.GetQuotaLedgerBalanceByFundingSource(user.Id, model.QuotaFundingCampaign)
	require.NoError(t, err)
	require.Equal(t, int64(60), campaignBalance)
	publicBalance, err := model.GetQuotaLedgerBalanceByFundingSource(user.Id, model.QuotaFundingPublicBenefit)
	require.NoError(t, err)
	require.Equal(t, int64(0), publicBalance)
}

func TestCampaignFundingRefundRestoresEntitlement(t *testing.T) {
	t.Setenv("YANCHUANER_CAMPAIGN_FUNDING_ENABLED", "true")
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	user, entitlement := seedCampaignFundingUser(t, 100)
	defer func() {
		model.DB.Where("user_id = ?", user.Id).Delete(&model.QuotaLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementClaim{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlement{})
		model.DB.Delete(&model.YanCoreRedeemCode{}, entitlement.RedeemCodeId)
		model.DB.Delete(&model.YanCoreCampaign{}, entitlement.CampaignId)
		model.DB.Delete(&model.User{}, user.Id)
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := campaignRelayInfo(user.Id, "deepseek-chat")
	session, apiErr := NewBillingSession(c, relayInfo, 50)
	require.Nil(t, apiErr)
	session.Refund(c)
	// Refund is asynchronous by design; wait briefly for the local test pool.
	require.Eventually(t, func() bool {
		var updated model.YanCoreEntitlement
		if err := model.DB.First(&updated, entitlement.Id).Error; err != nil {
			return false
		}
		return updated.RemainingQuota == 100
	}, time.Second, 10*time.Millisecond)
}

func TestCampaignFundingDoesNotFallbackWhenMatchedEntitlementIsInsufficient(t *testing.T) {
	t.Setenv("YANCHUANER_CAMPAIGN_FUNDING_ENABLED", "true")
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	user, entitlement := seedCampaignFundingUser(t, 30)
	defer func() {
		model.DB.Where("user_id = ?", user.Id).Delete(&model.QuotaLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementClaim{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlement{})
		model.DB.Delete(&model.YanCoreRedeemCode{}, entitlement.RedeemCodeId)
		model.DB.Delete(&model.YanCoreCampaign{}, entitlement.CampaignId)
		model.DB.Delete(&model.User{}, user.Id)
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := campaignRelayInfo(user.Id, "deepseek-chat")
	_, apiErr := NewBillingSession(c, relayInfo, 40)
	require.NotNil(t, apiErr)
	require.Equal(t, BillingSourceCampaign, relayInfo.BillingSource)
}

func TestCampaignQuotaCannotFundOutOfScopeModel(t *testing.T) {
	t.Setenv("YANCHUANER_CAMPAIGN_FUNDING_ENABLED", "true")
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	user, entitlement := seedCampaignFundingUser(t, 100)
	defer func() {
		model.DB.Where("user_id = ?", user.Id).Delete(&model.QuotaLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementClaim{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlement{})
		model.DB.Delete(&model.YanCoreRedeemCode{}, entitlement.RedeemCodeId)
		model.DB.Delete(&model.YanCoreCampaign{}, entitlement.CampaignId)
		model.DB.Delete(&model.User{}, user.Id)
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, apiErr := NewBillingSession(c, campaignRelayInfo(user.Id, "gpt-4.1-mini"), 10)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
}

func TestCampaignFundingRejectsAsyncRequestWithoutConsuming(t *testing.T) {
	t.Setenv("YANCHUANER_CAMPAIGN_FUNDING_ENABLED", "true")
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	user, entitlement := seedCampaignFundingUser(t, 100)
	defer func() {
		model.DB.Where("user_id = ?", user.Id).Delete(&model.QuotaLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementLedgerEntry{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlementClaim{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.YanCoreEntitlement{})
		model.DB.Delete(&model.YanCoreRedeemCode{}, entitlement.RedeemCodeId)
		model.DB.Delete(&model.YanCoreCampaign{}, entitlement.CampaignId)
		model.DB.Delete(&model.User{}, user.Id)
	}()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := campaignRelayInfo(user.Id, "deepseek-chat")
	relayInfo.ForcePreConsume = true
	_, apiErr := NewBillingSession(c, relayInfo, 10)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())

	var updated model.YanCoreEntitlement
	require.NoError(t, model.DB.First(&updated, entitlement.Id).Error)
	require.Equal(t, 100, updated.RemainingQuota)
}
