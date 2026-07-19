package controller

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuthorizeYanCoreExchangeClientUsesConstantTimeCredentialShape(t *testing.T) {
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_CLIENT_ID", "ai-yancore-bff")
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET", "01234567890123456789012345678901")
	valid := "Basic " + base64.StdEncoding.EncodeToString([]byte("ai-yancore-bff:01234567890123456789012345678901"))
	assert.True(t, authorizeYanCoreExchangeClient(valid))
	assert.False(t, authorizeYanCoreExchangeClient("Basic "+base64.StdEncoding.EncodeToString([]byte("ai-yancore-bff:wrong"))))
	assert.False(t, authorizeYanCoreExchangeClient("Bearer 01234567890123456789012345678901"))
}

func TestValidYanCoreMainSiteIdentityRequiresVerifiedSupportedRole(t *testing.T) {
	assert.True(t, validYanCoreMainSiteIdentity(&yanCoreMainSiteIdentity{Subject: "member-1", EmailVerified: true, Role: "student"}))
	assert.False(t, validYanCoreMainSiteIdentity(&yanCoreMainSiteIdentity{Subject: "member-1", EmailVerified: false, Role: "student"}))
	assert.False(t, validYanCoreMainSiteIdentity(&yanCoreMainSiteIdentity{Subject: "member-1", EmailVerified: true, Role: "guest"}))
}

func TestFetchYanCoreMainSiteIdentityStopsRedirectAndBoundsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer short-lived", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"sub":"member-1","email":"member@example.com","email_verified":true,"role":"alumni"}`))
	}))
	defer server.Close()
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_USERINFO_URL", server.URL)
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_ALLOW_INSECURE_HTTP", "true")

	request := httptest.NewRequest(http.MethodPost, "/api/yancore/subject-exchange", nil)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	identity, err := fetchYanCoreMainSiteIdentity(context, "short-lived")
	require.NoError(t, err)
	assert.Equal(t, "member-1", identity.Subject)
	assert.Equal(t, "alumni", identity.Role)
}

func TestExchangeYanCoreSubjectGrantRequiresTrustedBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:yancore_subject_exchange?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CustomOAuthProvider{}, &model.UserOAuthBinding{}, &model.YanCoreSubjectGrant{}))

	provider := &model.CustomOAuthProvider{Name: "Yanchuaner", Slug: "yanchuaner", Enabled: true}
	require.NoError(t, db.Create(provider).Error)
	user := &model.User{Username: "member-1", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(user).Error)
	binding := &model.UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "main-subject-1"}
	require.NoError(t, db.Create(binding).Error)

	userinfo := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer main-access-token", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"sub":"main-subject-1","email":"member@example.com","email_verified":true,"role":"alumni"}`))
	}))
	defer userinfo.Close()
	t.Setenv("YANCHUANER_SUBJECT_GRANTS_ENABLED", "true")
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_ENABLED", "true")
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_CLIENT_ID", "ai-yancore-bff")
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET", "01234567890123456789012345678901")
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_USERINFO_URL", userinfo.URL)
	t.Setenv("YANCHUANER_SUBJECT_EXCHANGE_ALLOW_INSECURE_HTTP", "true")
	t.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "abcdefghijklmnopqrstuvwxyz012345")

	router := gin.New()
	router.POST("/api/yancore/subject-exchange", ExchangeYanCoreSubjectGrant)
	request := httptest.NewRequest(http.MethodPost, "/api/yancore/subject-exchange", bytes.NewBufferString(`{"subject_token":"main-access-token","ttl":600}`))
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth("ai-yancore-bff", "01234567890123456789012345678901")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var success struct {
		Success bool `json:"success"`
		Data    struct {
			Grant string `json:"grant"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &success))
	require.True(t, success.Success)
	require.NotEmpty(t, success.Data.Grant)
	claims, err := model.ParseSubjectGrantForAudience(success.Data.Grant, yanCoreExchangeAudience)
	require.NoError(t, err)
	assert.Equal(t, "yc_user_"+strconv.Itoa(user.Id), claims.Subject)

	require.NoError(t, db.Delete(binding).Error)
	request = httptest.NewRequest(http.MethodPost, "/api/yancore/subject-exchange", bytes.NewBufferString(`{"subject_token":"main-access-token","ttl":600}`))
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth("ai-yancore-bff", "01234567890123456789012345678901")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
}
