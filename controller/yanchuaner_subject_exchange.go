/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	yanCoreExchangeApplication = "ai-web"
	yanCoreExchangeAudience    = "yanchuaner-ai"
	yanCoreExchangeScopes      = "chat:read chat:write"
	yanCoreExchangeDefaultTTL  = int64(15 * time.Minute / time.Second)
)

var (
	errYanCoreExchangeIdentity    = errors.New("subject identity is invalid")
	errYanCoreExchangeUnavailable = errors.New("subject identity provider is unavailable")
)

type yanCoreSubjectExchangeRequest struct {
	SubjectToken string `json:"subject_token"`
	TTL          int64  `json:"ttl"`
}

type yanCoreMainSiteIdentity struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Role          string `json:"role"`
}

func subjectGrantExchangeEnabled() bool {
	return common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_GRANTS_ENABLED", false) &&
		common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_EXCHANGE_ENABLED", false)
}

func aiWebSessionKeyPolicy() (int, []string, error) {
	if !common.GetEnvOrDefaultBool("YANCHUANER_HASHED_KEYS_ENABLED", false) {
		return 0, nil, model.ErrAiWebSessionKeyPolicy
	}
	quota, err := strconv.Atoi(strings.TrimSpace(common.GetEnvOrDefaultString("YANCHUANER_AI_WEB_SESSION_QUOTA", "")))
	if err != nil || quota <= 0 || quota > int(common.QuotaPerUnit) {
		return 0, nil, model.ErrAiWebSessionKeyPolicy
	}
	models, err := model.NormalizeAiWebSessionModels(strings.Split(common.GetEnvOrDefaultString("YANCHUANER_AI_WEB_MODELS", ""), ","))
	if err != nil {
		return 0, nil, err
	}
	return quota, models, nil
}

func authorizeYanCoreExchangeClient(value string) bool {
	clientID := strings.TrimSpace(common.GetEnvOrDefaultString("YANCHUANER_SUBJECT_EXCHANGE_CLIENT_ID", ""))
	clientSecret := strings.TrimSpace(common.GetEnvOrDefaultString("YANCHUANER_SUBJECT_EXCHANGE_CLIENT_SECRET", ""))
	if clientID == "" || len(clientSecret) < 32 {
		return false
	}
	value = strings.TrimSpace(value)
	if len(value) <= len("Basic ") || !strings.EqualFold(value[:6], "Basic ") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value[6:]))
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 || len(parts[0]) != len(clientID) || len(parts[1]) != len(clientSecret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[0]), []byte(clientID)) == 1 &&
		subtle.ConstantTimeCompare([]byte(parts[1]), []byte(clientSecret)) == 1
}

func validYanCoreMainSiteIdentity(identity *yanCoreMainSiteIdentity) bool {
	if identity == nil || strings.TrimSpace(identity.Subject) == "" || !identity.EmailVerified {
		return false
	}
	switch strings.TrimSpace(identity.Role) {
	case "admin", "alumni", "student", "teacher":
		return true
	default:
		return false
	}
}

func fetchYanCoreMainSiteIdentity(c *gin.Context, subjectToken string) (*yanCoreMainSiteIdentity, error) {
	subjectToken = strings.TrimSpace(subjectToken)
	if subjectToken == "" || len(subjectToken) > 2048 || strings.ContainsAny(subjectToken, "\r\n") {
		return nil, errYanCoreExchangeIdentity
	}
	endpoint := strings.TrimSpace(common.GetEnvOrDefaultString("YANCHUANER_SUBJECT_EXCHANGE_USERINFO_URL", ""))
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errYanCoreExchangeUnavailable
	}
	if parsed.Scheme == "http" && !common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_EXCHANGE_ALLOW_INSECURE_HTTP", false) {
		return nil, errYanCoreExchangeUnavailable
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, errYanCoreExchangeUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+subjectToken)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errYanCoreExchangeUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errYanCoreExchangeIdentity
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 32*1024+1))
	if err != nil || len(body) > 32*1024 {
		return nil, errYanCoreExchangeIdentity
	}
	var identity yanCoreMainSiteIdentity
	if err := common.Unmarshal(body, &identity); err != nil || !validYanCoreMainSiteIdentity(&identity) {
		return nil, errYanCoreExchangeIdentity
	}
	return &identity, nil
}

// ExchangeYanCoreSubjectGrant turns a short-lived main-site access token into
// a YanCore grant. The endpoint is server-to-server authenticated and only
// maps an already-bound yanchuaner OAuth identity; it never creates users.
func ExchangeYanCoreSubjectGrant(c *gin.Context) {
	if !subjectGrantExchangeEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore subject exchange is disabled."})
		return
	}
	if !authorizeYanCoreExchangeClient(c.GetHeader("Authorization")) {
		c.Header("WWW-Authenticate", `Basic realm="yancore-subject-exchange"`)
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "YanCore subject exchange client is invalid."})
		return
	}
	var request yanCoreSubjectExchangeRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.SubjectToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "subject_token is required."})
		return
	}
	ttl := request.TTL
	if ttl == 0 {
		ttl = yanCoreExchangeDefaultTTL
	}
	if ttl <= 0 || ttl > yanCoreExchangeDefaultTTL {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "ttl is outside the ai-web policy."})
		return
	}
	identity, err := fetchYanCoreMainSiteIdentity(c, request.SubjectToken)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errYanCoreExchangeUnavailable) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"success": false, "message": "main-site identity could not be verified."})
		return
	}
	provider, err := model.GetCustomOAuthProviderBySlug("yanchuaner")
	if err != nil || provider == nil || !provider.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "YanCore identity binding is unavailable."})
		return
	}
	user, err := model.GetUserByOAuthBinding(provider.Id, identity.Subject)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "main-site identity is not bound to an API account."})
		return
	}
	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "API account is disabled."})
		return
	}
	quota, allowedModels, err := aiWebSessionKeyPolicy()
	if err != nil {
		common.SysLog("YanCore ai-web session key policy is incomplete: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "YanCore ai-web credential policy is unavailable."})
		return
	}
	token, grant, err := model.IssueSubjectGrant(user.Id, yanCoreExchangeApplication, yanCoreExchangeAudience, yanCoreExchangeScopes, ttl)
	if err != nil {
		common.SysLog(fmt.Sprintf("YanCore subject grant exchange failed for user %d: %s", user.Id, err.Error()))
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "YanCore subject grant could not be issued."})
		return
	}
	accessKey, sessionToken, err := model.IssueAiWebSessionKey(user.Id, grant.Id, grant.ExpiresAt, quota, allowedModels)
	if err != nil {
		_ = model.RevokeSubjectGrant(user.Id, grant.Id)
		common.SysLog(fmt.Sprintf("YanCore ai-web session key issuance failed for user %d: %s", user.Id, err.Error()))
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "YanCore ai-web credential could not be issued."})
		return
	}
	recordUserSecurityAudit(c, user.Id, "yancore.subject-exchange.issue", map[string]interface{}{
		"target_user_id": user.Id,
		"grant_id":       grant.Id,
		"token_id":       sessionToken.Id,
		"application":    grant.Application,
		"audience":       grant.Audience,
		"scopes":         grant.Scopes,
		"expires_at":     grant.ExpiresAt,
		"quota_units":    sessionToken.RemainQuota,
		"models":         sessionToken.GetModelLimits(),
	})
	common.ApiSuccess(c, gin.H{
		"grant": token,
		"credential": gin.H{
			"access_key":  accessKey,
			"models":      sessionToken.GetModelLimits(),
			"quota_units": sessionToken.RemainQuota,
			"expires_at":  sessionToken.ExpiredTime,
		},
		"subject": gin.H{
			"user_id":     user.Id,
			"application": grant.Application,
			"audience":    grant.Audience,
			"scopes":      grant.Scopes,
			"expires_at":  grant.ExpiresAt,
		},
	})
}
