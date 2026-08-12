/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func budgetUnitsThreshold(name string) int64 {
	return int64(common.GetEnvOrDefault(name, 0))
}

// GetAdminBudgetSummary returns daily and monthly public-benefit spend with
// configured budget thresholds. Root-only, used for monitoring and alerts.
func GetAdminBudgetSummary(c *gin.Context) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(location)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)

	day, err := model.GetQuotaLedgerSpendSummary(dayStart.Unix(), now.Unix())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	month, err := model.GetQuotaLedgerSpendSummary(monthStart.Unix(), now.Unix())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	dailyBudget := budgetUnitsThreshold("YANCHUANER_DAILY_BUDGET_UNITS")
	monthlyBudget := budgetUnitsThreshold("YANCHUANER_MONTHLY_BUDGET_UNITS")
	common.ApiSuccess(c, gin.H{
		"day":            day,
		"month":          month,
		"daily_budget":   dailyBudget,
		"monthly_budget": monthlyBudget,
		"over_daily":     dailyBudget > 0 && day.ConsumedUnits > dailyBudget,
		"over_monthly":   monthlyBudget > 0 && month.ConsumedUnits > monthlyBudget,
	})
}
