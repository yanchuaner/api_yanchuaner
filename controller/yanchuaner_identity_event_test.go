package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVerifyYanCoreIdentityEventSignature(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	body := []byte(`{"event":"account.disabled"}`)
	signature := yanCoreIdentityEventSignature(secret, body)
	assert.True(t, verifyYanCoreIdentityEventSignature(secret, body, yanCoreIdentityEventSignaturePrefix+signature))
	assert.False(t, verifyYanCoreIdentityEventSignature(secret, body, "sha256="+strings.Repeat("0", 64)))
	assert.False(t, verifyYanCoreIdentityEventSignature(secret, body, signature))
	assert.False(t, verifyYanCoreIdentityEventSignature(secret, []byte(`{"event":"role.changed"}`), yanCoreIdentityEventSignaturePrefix+signature))
}

func setupYanCoreIdentityEventDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_identity_event_controller?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(
		&model.CustomOAuthProvider{},
		&model.UserOAuthBinding{},
		&model.User{},
		&model.Token{},
		&model.YanCoreSubjectGrant{},
		&model.YanCoreIdentityEvent{},
		&model.Log{},
	))
	return db
}

func TestHandleYanCoreIdentityEventDisablesBoundUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupYanCoreIdentityEventDB(t)
	provider := &model.CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)
	user := &model.User{Username: "member-controller", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "main-subject-controller"}).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.Token{UserId: user.Id, Key: "sha256:controller-session", KeyHashEnabled: true, Status: common.TokenStatusEnabled, Name: "yancore:ai-web:session:9", ExpiredTime: now + 900}).Error)
	require.NoError(t, db.Create(&model.YanCoreSubjectGrant{UserId: user.Id, Application: "ai-web", Audience: "yanchuaner-ai", Scopes: "chat:read chat:write", JTIHash: strings.Repeat("f", 64), ExpiresAt: now + 900}).Error)

	secret := strings.Repeat("c", 32)
	t.Setenv("YANCHUANER_IDENTITY_EVENT_SECRET", secret)
	router := gin.New()
	router.POST("/api/yancore/identity-events", HandleYanCoreIdentityEvent)

	body := fmt.Sprintf(
		`{"event_id":"controller-event-000001","subject":"main-subject-controller","event":"account.disabled","role":"alumni","account_status":"DISABLED","status":"PENDING","occurred_at":%d}`,
		now,
	)
	signature := yanCoreIdentityEventSignature([]byte(secret), []byte(body))
	request := httptest.NewRequest(http.MethodPost, "/api/yancore/identity-events", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Yanchuaner-Event-Time", strconv.FormatInt(now, 10))
	request.Header.Set("X-Yanchuaner-Signature", yanCoreIdentityEventSignaturePrefix+signature)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"tokens_disabled":1`)
	assert.Contains(t, recorder.Body.String(), `"grants_revoked":1`)

	var storedUser model.User
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, storedUser.Status)
	var activeGrants int64
	require.NoError(t, db.Model(&model.YanCoreSubjectGrant{}).Where("user_id = ? AND revoked_at = 0", user.Id).Count(&activeGrants).Error)
	assert.Zero(t, activeGrants)

	// Replay is idempotent.
	request2 := httptest.NewRequest(http.MethodPost, "/api/yancore/identity-events", bytes.NewBufferString(body))
	request2.Header.Set("Content-Type", "application/json")
	request2.Header.Set("X-Yanchuaner-Event-Time", strconv.FormatInt(now, 10))
	request2.Header.Set("X-Yanchuaner-Signature", yanCoreIdentityEventSignaturePrefix+signature)
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, request2)
	require.Equal(t, http.StatusOK, recorder2.Code, recorder2.Body.String())
	assert.Contains(t, recorder2.Body.String(), `"already_processed":true`)
}

func TestHandleYanCoreIdentityEventRejectsBadSignatureAndStaleTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupYanCoreIdentityEventDB(t)
	provider := &model.CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)
	user := &model.User{Username: "member-controller-2", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "main-subject-controller-2"}).Error)

	secret := strings.Repeat("d", 32)
	t.Setenv("YANCHUANER_IDENTITY_EVENT_SECRET", secret)
	router := gin.New()
	router.POST("/api/yancore/identity-events", HandleYanCoreIdentityEvent)
	now := time.Now().Unix()
	body := fmt.Sprintf(
		`{"event_id":"controller-event-bad-00001","subject":"main-subject-controller-2","event":"role.changed","role":"admin","occurred_at":%d}`,
		now,
	)

	badSignature := httptest.NewRequest(http.MethodPost, "/api/yancore/identity-events", bytes.NewBufferString(body))
	badSignature.Header.Set("X-Yanchuaner-Event-Time", strconv.FormatInt(now, 10))
	badSignature.Header.Set("X-Yanchuaner-Signature", yanCoreIdentityEventSignaturePrefix+strings.Repeat("0", 64))
	badRecorder := httptest.NewRecorder()
	router.ServeHTTP(badRecorder, badSignature)
	assert.Equal(t, http.StatusUnauthorized, badRecorder.Code)

	signature := yanCoreIdentityEventSignature([]byte(secret), []byte(body))
	stale := httptest.NewRequest(http.MethodPost, "/api/yancore/identity-events", bytes.NewBufferString(body))
	stale.Header.Set("X-Yanchuaner-Event-Time", strconv.FormatInt(now-400, 10))
	stale.Header.Set("X-Yanchuaner-Signature", yanCoreIdentityEventSignaturePrefix+signature)
	staleRecorder := httptest.NewRecorder()
	router.ServeHTTP(staleRecorder, stale)
	assert.Equal(t, http.StatusUnauthorized, staleRecorder.Code)
}
