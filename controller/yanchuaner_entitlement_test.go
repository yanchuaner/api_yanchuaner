/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimYanCoreEntitlementDoesNotExposeCodeState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 42)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/yancore/entitlements/claim", strings.NewReader(`{"code":"not-a-real-code"}`))
	context.Request.Header.Set("Content-Type", "application/json")

	original := claimYanCoreRedeemCode
	claimYanCoreRedeemCode = func(int, string) (*model.YanCoreEntitlement, bool, error) {
		return nil, false, errors.New("redeem code expired")
	}
	t.Cleanup(func() { claimYanCoreRedeemCode = original })

	ClaimYanCoreEntitlement(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.JSONEq(t, `{"success":false,"message":"entitlement claim failed"}`, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "expired")
}
