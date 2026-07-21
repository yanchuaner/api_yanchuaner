/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func applyAdminQuotaLedgerChange(c *gin.Context, user *model.User, amount int, mode string, reason string) error {
	requestId := c.GetString(common.RequestIdKey)
	if requestId == "" {
		requestId = common.NewRequestId()
	}
	entry, err := model.ApplyQuotaLedgerChange(model.QuotaLedgerChange{
		UserId:         user.Id,
		ActorUserId:    c.GetInt("id"),
		RequestId:      requestId,
		IdempotencyKey: fmt.Sprintf("%s:user:%d:%s", requestId, user.Id, mode),
		EntryType:      model.QuotaLedgerTypeAdjustment,
		FundingSource:  model.QuotaFundingPublicBenefit,
		Amount:         amount,
		Reason:         reason,
	})
	if err == nil {
		user.Quota = entry.BalanceAfter
	}
	return err
}

func GetMyQuotaLedger(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	entries, total, err := model.GetUserQuotaLedger(
		c.GetInt("id"),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(entries)
	common.ApiSuccess(c, pageInfo)
}
