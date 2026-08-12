package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestYanCoreAdminQuotaUsesAdminGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_admin_quota_grant?mode=memory&cache=shared"), &gorm.Config{})
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
	admin := &model.User{Username: "grant-admin", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleRootUser, Quota: 0, AffCode: "grant-admin-aff"}
	member := &model.User{Username: "grant-member", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AffCode: "grant-member-aff"}
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(member).Error)

	t.Setenv("YANCHUANER_SUBJECT_GRANTS_ENABLED", "true")
	t.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "abcdefghijklmnopqrstuvwxyz012345")
	adminGrant, _, err := model.IssueSubjectGrant(admin.Id, "ai-web", "yanchuaner-ai", "chat:read chat:write", 900)
	require.NoError(t, err)
	claims, err := model.ParseSubjectGrantForAudience(adminGrant, "yanchuaner-ai")
	require.NoError(t, err)
	assert.True(t, claims.Admin)

	memberGrant, _, err := model.IssueSubjectGrant(member.Id, "ai-web", "yanchuaner-ai", "chat:read chat:write", 900)
	require.NoError(t, err)
	memberClaims, err := model.ParseSubjectGrantForAudience(memberGrant, "yanchuaner-ai")
	require.NoError(t, err)
	assert.False(t, memberClaims.Admin)

	router := gin.New()
	router.POST("/api/yancore/admin/quota", YanCoreAdminQuota)
	body := bytes.NewBufferString(`{"user_id":2,"action":"grant","amount":500,"reason":"线下收款测试","reference":"wx-grant-e2e"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/yancore/admin/quota", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+adminGrant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"balance_after":1500`)

	denied := httptest.NewRequest(http.MethodPost, "/api/yancore/admin/quota", bytes.NewBufferString(`{"user_id":2,"action":"grant","amount":1,"reason":"测试","reference":"denied"}`))
	denied.Header.Set("Content-Type", "application/json")
	denied.Header.Set("Authorization", "Bearer "+memberGrant)
	deniedRecorder := httptest.NewRecorder()
	router.ServeHTTP(deniedRecorder, denied)
	assert.Equal(t, http.StatusForbidden, deniedRecorder.Code)
}
