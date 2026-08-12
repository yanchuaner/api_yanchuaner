package controller

import (
	"encoding/json"
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

func setupAdminQuotaDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yanchuaner_admin_quota?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.QuotaLedgerEntry{}, &model.Log{}))
	return db
}

func runAdminQuota(t *testing.T, body string, adminID int) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/yanchuaner/admin/quota", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", adminID)
	AdminQuotaAdjust(context)
	return recorder
}

func TestAdminQuotaGrantIsIdempotentAndAudited(t *testing.T) {
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	db := setupAdminQuotaDB(t)
	user := &model.User{Username: "quota-member", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AffCode: "quota-member-aff"}
	require.NoError(t, db.Create(user).Error)
	admin := &model.User{Username: "quota-admin", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleRootUser, Quota: 0, AffCode: "quota-admin-aff"}
	require.NoError(t, db.Create(admin).Error)

	body := `{"user_id":1,"action":"grant","amount":5000,"reason":"微信线下收款 50 元","reference":"wx-20260812-0001"}`
	first := runAdminQuota(t, body, admin.Id)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var firstData struct {
		Data struct {
			EntryId      int64 `json:"entry_id"`
			BalanceAfter int   `json:"balance_after"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstData))
	assert.EqualValues(t, 6000, firstData.Data.BalanceAfter)

	replay := runAdminQuota(t, body, admin.Id)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	var replayData struct {
		Data struct {
			EntryId int64 `json:"entry_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(replay.Body.Bytes(), &replayData))
	assert.Equal(t, firstData.Data.EntryId, replayData.Data.EntryId)

	var entries []model.QuotaLedgerEntry
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&entries).Error)
	assert.Len(t, entries, 1)
	assert.Equal(t, model.QuotaLedgerTypeGrant, entries[0].EntryType)
	assert.Equal(t, "quota:ref:wx-20260812-0001", entries[0].IdempotencyKey)
}

func TestAdminQuotaRejectsConflictOverdrawAndMissingReference(t *testing.T) {
	t.Setenv("YANCHUANER_QUOTA_LEDGER_ENABLED", "true")
	db := setupAdminQuotaDB(t)
	user := &model.User{Username: "quota-member-2", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 100, AffCode: "quota-member-2-aff"}
	require.NoError(t, db.Create(user).Error)
	admin := &model.User{Username: "quota-admin-2", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleRootUser, Quota: 0, AffCode: "quota-admin-2-aff"}
	require.NoError(t, db.Create(admin).Error)

	conflict := runAdminQuota(t, `{"user_id":2,"action":"grant","amount":100,"reason":"线下收款","idempotency_key":"key-1"}`, admin.Id)
	require.Equal(t, http.StatusOK, conflict.Code)
	conflict2 := runAdminQuota(t, `{"user_id":2,"action":"grant","amount":200,"reason":"线下收款","idempotency_key":"key-1"}`, admin.Id)
	assert.Equal(t, http.StatusConflict, conflict2.Code)

	overdraw := runAdminQuota(t, `{"user_id":2,"action":"adjust","amount":-5000,"reason":"误发回退","reference":"ref-overdraw"}`, admin.Id)
	assert.Equal(t, http.StatusBadRequest, overdraw.Code)

	missing := runAdminQuota(t, `{"user_id":2,"action":"grant","amount":100,"reason":"线下收款"}`, admin.Id)
	assert.Equal(t, http.StatusBadRequest, missing.Code)
}
