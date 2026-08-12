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

func TestYanCoreTokenKeysLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open("file:yancore_token_keys?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.YanCoreSubjectGrant{}, &model.Log{}))
	user := &model.User{Username: "key-member", Password: "x", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000, AffCode: "key-member-aff"}
	require.NoError(t, db.Create(user).Error)

	t.Setenv("YANCHUANER_SUBJECT_GRANTS_ENABLED", "true")
	t.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "abcdefghijklmnopqrstuvwxyz012345")
	t.Setenv("YANCHUANER_HASHED_KEYS_ENABLED", "true")
	grant, _, err := model.IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:read chat:write", 900)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/api/yancore/me/keys", YanCoreTokenList)
	router.POST("/api/yancore/me/keys", YanCoreTokenCreate)
	router.DELETE("/api/yancore/me/keys/:id", YanCoreTokenDelete)

	expires := time.Now().Add(24 * time.Hour).Unix()
	createBody := fmt.Sprintf(`{"name":"my-key","expired_time":%d,"remain_quota":1000,"unlimited_quota":false,"model_limits_enabled":true,"model_limits":"deepseek-v4-flash"}`, expires)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/yancore/me/keys", bytes.NewBufferString(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Authorization", "Bearer "+grant)
	createRecorder := httptest.NewRecorder()
	router.ServeHTTP(createRecorder, createRequest)
	require.Equal(t, http.StatusOK, createRecorder.Code, createRecorder.Body.String())
	var created struct {
		Success bool `json:"success"`
		Data    struct {
			Key   string      `json:"key"`
			Token model.Token `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(createRecorder.Body.Bytes(), &created))
	require.True(t, created.Success)
	assert.True(t, strings.HasPrefix(created.Data.Key, "sk-yc_"))
	assert.NotEqual(t, created.Data.Key, created.Data.Token.Key)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/yancore/me/keys?page=1&pageSize=20", nil)
	listRequest.Header.Set("Authorization", "Bearer "+grant)
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, listRequest)
	require.Equal(t, http.StatusOK, listRecorder.Code, listRecorder.Body.String())
	var listed struct {
		Data struct {
			Items []model.Token `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listed))
	require.Len(t, listed.Data.Items, 1)
	assert.NotContains(t, listed.Data.Items[0].Key, "sk-yc_")

	var stored model.Token
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, model.HashVirtualKey(strings.TrimPrefix(created.Data.Key, "sk-")), stored.Key)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/yancore/me/keys/"+strconv.Itoa(stored.Id), nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+grant)
	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, deleteRequest)
	assert.Equal(t, http.StatusOK, deleteRecorder.Code)

	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
}
