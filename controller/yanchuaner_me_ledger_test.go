package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestYanCoreMeLedgerUsesGrantAsCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_me_ledger?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.QuotaLedgerEntry{}, &model.YanCoreSubjectGrant{}, &model.Log{}))
	user := &model.User{Username: "ledger-member", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AffCode: "ledger-member-aff"}
	require.NoError(t, db.Create(user).Error)
	_, err = model.ApplyQuotaLedgerChange(model.QuotaLedgerChange{
		UserId:         user.Id,
		IdempotencyKey: "ledger-member:grant:v1",
		EntryType:      model.QuotaLedgerTypeGrant,
		FundingSource:  model.QuotaFundingPublicBenefit,
		Amount:         500,
		Reason:         "test grant",
	})
	require.NoError(t, err)

	t.Setenv("YANCHUANER_SUBJECT_GRANTS_ENABLED", "true")
	t.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "abcdefghijklmnopqrstuvwxyz012345")
	grant, _, err := model.IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:read chat:write", 900)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/api/yancore/me/ledger", YanCoreMeLedger)
	request := httptest.NewRequest(http.MethodGet, "/api/yancore/me/ledger?page=1&pageSize=20", nil)
	request.Header.Set("Authorization", "Bearer "+grant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var success struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.QuotaLedgerEntry `json:"items"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &success))
	require.True(t, success.Success)
	assert.Len(t, success.Data.Items, 1)
	assert.Equal(t, 1, success.Data.Total)
	assert.Equal(t, model.QuotaLedgerTypeGrant, success.Data.Items[0].EntryType)

	bad := httptest.NewRequest(http.MethodGet, "/api/yancore/me/ledger", nil)
	bad.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	badRecorder := httptest.NewRecorder()
	router.ServeHTTP(badRecorder, bad)
	assert.Equal(t, http.StatusUnauthorized, badRecorder.Code)
}
