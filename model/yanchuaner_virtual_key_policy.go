/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"errors"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	YanCoreVirtualKeyPolicyActive             = "active"
	YanCoreVirtualKeyPolicyDisabled           = "disabled"
	YanCoreVirtualKeyPolicyDefaultRPM         = 60
	YanCoreVirtualKeyPolicyDefaultTPM         = 100000
	YanCoreVirtualKeyPolicyDefaultConcurrency = 2
)

var (
	ErrYanCoreVirtualKeyPolicyInvalid           = errors.New("yancore virtual key policy is invalid")
	ErrYanCoreVirtualKeyPolicyNotFound          = errors.New("yancore virtual key policy not found")
	ErrYanCoreVirtualKeyPolicyModelRequired     = errors.New("yancore virtual key policy requires a model allowlist")
	ErrYanCoreVirtualKeyPolicyRolloutNotPending = errors.New("yancore virtual key policy rollout target is not pending")
)

// YanCoreVirtualKeyPolicy is an autonomous policy overlay for a hashed Token.
// Budget, expiry, model and source IP remain on Token as compatibility
// projections until the gateway is moved out of New API.
type YanCoreVirtualKeyPolicy struct {
	Id             int64  `json:"id"`
	TokenId        int    `json:"token_id" gorm:"uniqueIndex;not null"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	ProviderScope  string `json:"provider_scope" gorm:"type:text;not null"`
	MaxRPM         int    `json:"max_rpm" gorm:"not null"`
	MaxTPM         int    `json:"max_tpm" gorm:"not null"`
	MaxConcurrency int    `json:"max_concurrency" gorm:"not null"`
	Status         string `json:"status" gorm:"type:varchar(16);index;not null"`
	Version        int    `json:"version" gorm:"not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime;index"`
}

// YanCoreVirtualKeyPolicyRevision is append-only evidence for every policy
// creation or change. Token model/IP scopes are snapshotted for audit.
type YanCoreVirtualKeyPolicyRevision struct {
	Id             int64  `json:"id"`
	PolicyId       int64  `json:"policy_id" gorm:"index;not null"`
	TokenId        int    `json:"token_id" gorm:"index;not null"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	ActorUserId    int    `json:"actor_user_id" gorm:"index;not null"`
	Version        int    `json:"version" gorm:"not null"`
	ProviderScope  string `json:"provider_scope" gorm:"type:text;not null"`
	ModelScope     string `json:"model_scope" gorm:"type:text;not null"`
	SourceScope    string `json:"source_scope" gorm:"type:text;not null"`
	BudgetQuota    int    `json:"budget_quota" gorm:"not null"`
	ExpiresAt      int64  `json:"expires_at" gorm:"not null"`
	TokenStatus    int    `json:"token_status" gorm:"not null"`
	MaxRPM         int    `json:"max_rpm" gorm:"not null"`
	MaxTPM         int    `json:"max_tpm" gorm:"not null"`
	MaxConcurrency int    `json:"max_concurrency" gorm:"not null"`
	Status         string `json:"status" gorm:"type:varchar(16);not null"`
	Reason         string `json:"reason" gorm:"type:varchar(255);not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

// YanCoreVirtualKeyPolicyConfig is used by user-facing policy APIs and token
// creation. Providers are normalized to lower-case exact names.
type YanCoreVirtualKeyPolicyConfig struct {
	Providers      []string                      `json:"providers"`
	MaxRPM         int                           `json:"max_rpm"`
	MaxTPM         int                           `json:"max_tpm"`
	MaxConcurrency int                           `json:"max_concurrency"`
	Status         string                        `json:"status"`
	Reason         string                        `json:"reason"`
	Token          *YanCoreVirtualKeyTokenUpdate `json:"token,omitempty"`
}

// YanCoreVirtualKeyTokenUpdate contains the compatibility fields that still
// live on Token during stage B. Pointer fields preserve partial-update
// semantics while allowing explicit zero/false values where valid.
type YanCoreVirtualKeyTokenUpdate struct {
	Name            *string   `json:"name,omitempty"`
	RemainQuota     *int      `json:"remain_quota,omitempty"`
	ExpiredTime     *int64    `json:"expired_time,omitempty"`
	Models          *[]string `json:"models,omitempty"`
	AllowIPs        *string   `json:"allow_ips,omitempty"`
	Group           *string   `json:"group,omitempty"`
	CrossGroupRetry *bool     `json:"cross_group_retry,omitempty"`
	Status          *int      `json:"status,omitempty"`
}

const (
	YanCoreVirtualKeyPolicyRolloutReady  = "ready_to_activate"
	YanCoreVirtualKeyPolicyRolloutReview = "requires_review"
)

// YanCoreVirtualKeyPolicyRolloutItem is an administrator-facing preflight
// record. It intentionally excludes all key material and display fragments.
type YanCoreVirtualKeyPolicyRolloutItem struct {
	TokenID        int    `json:"token_id"`
	UserID         int    `json:"user_id"`
	ModelScope     string `json:"model_scope"`
	Classification string `json:"classification"`
	Reason         string `json:"reason"`
}

// YanCoreVirtualKeyPolicyRolloutReport is a point-in-time view of legacy
// hashed keys that have not yet received a YanCore policy.
type YanCoreVirtualKeyPolicyRolloutReport struct {
	GeneratedAt     int64                                `json:"generated_at"`
	TotalHashedKeys int                                  `json:"total_hashed_keys"`
	ManagedActive   int                                  `json:"managed_active"`
	ManagedDisabled int                                  `json:"managed_disabled"`
	PendingReady    int                                  `json:"pending_ready"`
	PendingReview   int                                  `json:"pending_review"`
	Items           []YanCoreVirtualKeyPolicyRolloutItem `json:"items"`
}

// YanCoreVirtualKeyPolicyRolloutResult is the auditable outcome of one
// explicit administrator-approved rollout batch.
type YanCoreVirtualKeyPolicyRolloutResult struct {
	Applied   int                                  `json:"applied"`
	Activated int                                  `json:"activated"`
	Disabled  int                                  `json:"disabled"`
	Items     []YanCoreVirtualKeyPolicyRolloutItem `json:"items"`
}

func YanCoreVirtualKeyPolicyEnabled() bool {
	return common.GetEnvOrDefaultBool("YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED", false)
}

func normalizeYanCoreProviderScope(providers []string) (string, error) {
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		for _, item := range strings.FieldsFunc(provider, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
		}) {
			item = strings.ToLower(strings.TrimSpace(item))
			if item == "" {
				continue
			}
			if item != "*" && item != "openai" && item != "deepseek" {
				return "", ErrYanCoreVirtualKeyPolicyInvalid
			}
			seen[item] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return "", ErrYanCoreVirtualKeyPolicyInvalid
	}
	if _, wildcard := seen["*"]; wildcard {
		if len(seen) != 1 {
			return "", ErrYanCoreVirtualKeyPolicyInvalid
		}
		return "*", nil
	}
	values := make([]string, 0, len(seen))
	for provider := range seen {
		values = append(values, provider)
	}
	sort.Strings(values)
	return strings.Join(values, ","), nil
}

func inferYanCoreProvidersFromToken(token *Token) ([]string, error) {
	if token == nil || !token.ModelLimitsEnabled || strings.TrimSpace(token.ModelLimits) == "" {
		return nil, ErrYanCoreVirtualKeyPolicyModelRequired
	}
	providers := make(map[string]struct{})
	for _, modelName := range strings.FieldsFunc(token.ModelLimits, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	}) {
		modelName = strings.ToLower(strings.TrimSpace(modelName))
		if modelName == "" {
			continue
		}
		switch {
		case modelName == "*":
			return []string{"*"}, nil
		case strings.HasPrefix(modelName, "deepseek"):
			providers["deepseek"] = struct{}{}
		case strings.HasPrefix(modelName, "gpt-"), strings.HasPrefix(modelName, "o1"), strings.HasPrefix(modelName, "o3"), strings.HasPrefix(modelName, "o4"):
			providers["openai"] = struct{}{}
		default:
			return nil, ErrYanCoreVirtualKeyPolicyInvalid
		}
	}
	if len(providers) == 0 {
		return nil, ErrYanCoreVirtualKeyPolicyModelRequired
	}
	result := make([]string, 0, len(providers))
	for provider := range providers {
		result = append(result, provider)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeYanCoreVirtualKeyModels(models []string) (string, error) {
	if len(models) == 0 || len(models) > 32 {
		return "", ErrYanCoreVirtualKeyPolicyModelRequired
	}
	seen := make(map[string]struct{}, len(models))
	normalized := make([]string, 0, len(models))
	for _, candidate := range models {
		modelName := strings.TrimSpace(candidate)
		if modelName == "" || len(modelName) > 128 || strings.ContainsAny(modelName, ",\r\n\t") {
			return "", ErrYanCoreVirtualKeyPolicyInvalid
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		normalized = append(normalized, modelName)
	}
	if len(normalized) == 0 {
		return "", ErrYanCoreVirtualKeyPolicyModelRequired
	}
	sort.Strings(normalized)
	return strings.Join(normalized, ","), nil
}

func normalizeYanCoreVirtualKeyIPs(raw string) (string, error) {
	items := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	if len(items) > 64 {
		return "", ErrYanCoreVirtualKeyPolicyInvalid
	}
	seen := make(map[string]struct{}, len(items))
	normalized := make([]string, 0, len(items))
	for _, candidate := range items {
		candidate = strings.TrimSpace(candidate)
		var value string
		if strings.Contains(candidate, "/") {
			_, network, err := net.ParseCIDR(candidate)
			if err != nil {
				return "", ErrYanCoreVirtualKeyPolicyInvalid
			}
			value = network.String()
		} else {
			ip := net.ParseIP(candidate)
			if ip == nil {
				return "", ErrYanCoreVirtualKeyPolicyInvalid
			}
			value = ip.String()
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\n"), nil
}

func normalizeYanCoreVirtualKeyTokenForPolicy(token *Token) error {
	if token == nil || token.UserId <= 0 || !token.KeyHashEnabled || token.UnlimitedQuota || token.RemainQuota <= 0 || !token.ModelLimitsEnabled {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	models := strings.FieldsFunc(token.ModelLimits, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	modelScope, err := normalizeYanCoreVirtualKeyModels(models)
	if err != nil {
		return err
	}
	token.ModelLimits = modelScope
	if token.AllowIps != nil {
		allowIPs, err := normalizeYanCoreVirtualKeyIPs(*token.AllowIps)
		if err != nil {
			return err
		}
		token.AllowIps = &allowIPs
	}
	if token.ExpiredTime == 0 {
		token.ExpiredTime = -1
	}
	if token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix() {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	if token.Status == 0 {
		token.Status = common.TokenStatusEnabled
	}
	return nil
}

func applyYanCoreVirtualKeyTokenUpdate(token *Token, update *YanCoreVirtualKeyTokenUpdate) error {
	if update == nil {
		return nil
	}
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" || len(name) > 50 || IsReservedYanCoreTokenName(name) {
			return ErrYanCoreVirtualKeyPolicyInvalid
		}
		token.Name = name
	}
	if update.RemainQuota != nil {
		if *update.RemainQuota <= 0 || *update.RemainQuota > int(1000000000*common.QuotaPerUnit) {
			return ErrYanCoreVirtualKeyPolicyInvalid
		}
		token.RemainQuota = *update.RemainQuota
		token.UnlimitedQuota = false
	}
	if update.ExpiredTime != nil {
		if *update.ExpiredTime != -1 && *update.ExpiredTime <= time.Now().Unix() {
			return ErrYanCoreVirtualKeyPolicyInvalid
		}
		token.ExpiredTime = *update.ExpiredTime
	}
	if update.Models != nil {
		modelScope, err := normalizeYanCoreVirtualKeyModels(*update.Models)
		if err != nil {
			return err
		}
		token.ModelLimitsEnabled = true
		token.ModelLimits = modelScope
	}
	if update.AllowIPs != nil {
		allowIPs, err := normalizeYanCoreVirtualKeyIPs(*update.AllowIPs)
		if err != nil {
			return err
		}
		token.AllowIps = &allowIPs
	}
	if update.Group != nil {
		token.Group = strings.TrimSpace(*update.Group)
	}
	if update.CrossGroupRetry != nil {
		token.CrossGroupRetry = *update.CrossGroupRetry
	}
	if update.Status != nil {
		if *update.Status != common.TokenStatusEnabled && *update.Status != common.TokenStatusDisabled {
			return ErrYanCoreVirtualKeyPolicyInvalid
		}
		token.Status = *update.Status
	}
	if !token.KeyHashEnabled || token.UnlimitedQuota || token.RemainQuota <= 0 || !token.ModelLimitsEnabled {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	return nil
}

func validateYanCoreVirtualKeyPolicy(policy *YanCoreVirtualKeyPolicy) error {
	if policy == nil || policy.TokenId < 0 || policy.UserId <= 0 || strings.TrimSpace(policy.ProviderScope) == "" ||
		policy.MaxRPM < 0 || policy.MaxTPM < 0 || policy.MaxConcurrency < 0 ||
		(policy.Status != YanCoreVirtualKeyPolicyActive && policy.Status != YanCoreVirtualKeyPolicyDisabled) || policy.Version <= 0 {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	if policy.Status == YanCoreVirtualKeyPolicyActive && policy.ProviderScope == "*" {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	if policy.MaxRPM > 100000 || policy.MaxTPM > 1000000000 || policy.MaxConcurrency > 10000 {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	return nil
}

func validatePersistedYanCoreVirtualKeyPolicy(policy *YanCoreVirtualKeyPolicy) error {
	if err := validateYanCoreVirtualKeyPolicy(policy); err != nil || policy.TokenId <= 0 {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	return nil
}

func BuildYanCoreVirtualKeyPolicy(token *Token, config *YanCoreVirtualKeyPolicyConfig) (*YanCoreVirtualKeyPolicy, error) {
	if token == nil || token.UserId <= 0 || !token.KeyHashEnabled {
		return nil, ErrYanCoreVirtualKeyPolicyInvalid
	}
	if err := normalizeYanCoreVirtualKeyTokenForPolicy(token); err != nil {
		return nil, err
	}
	if config == nil {
		config = &YanCoreVirtualKeyPolicyConfig{}
	}
	providers := config.Providers
	requiredProviders, err := inferYanCoreProvidersFromToken(token)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		providers = requiredProviders
	}
	providerScope, err := normalizeYanCoreProviderScope(providers)
	if err != nil {
		return nil, err
	}
	for _, required := range requiredProviders {
		if providerScope != "*" && !strings.Contains(","+providerScope+",", ","+required+",") {
			return nil, ErrYanCoreVirtualKeyPolicyInvalid
		}
	}
	maxRPM := config.MaxRPM
	if maxRPM == 0 {
		maxRPM = common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_RPM", YanCoreVirtualKeyPolicyDefaultRPM)
	}
	maxTPM := config.MaxTPM
	if maxTPM == 0 {
		maxTPM = common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_TPM", YanCoreVirtualKeyPolicyDefaultTPM)
	}
	maxConcurrency := config.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_CONCURRENCY", YanCoreVirtualKeyPolicyDefaultConcurrency)
	}
	status := config.Status
	if status == "" {
		status = YanCoreVirtualKeyPolicyActive
	}
	policy := &YanCoreVirtualKeyPolicy{TokenId: token.Id, UserId: token.UserId, ProviderScope: providerScope, MaxRPM: maxRPM, MaxTPM: maxTPM, MaxConcurrency: maxConcurrency, Status: status, Version: 1}
	if err := validateYanCoreVirtualKeyPolicy(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func (policy *YanCoreVirtualKeyPolicy) AllowsProvider(provider string) bool {
	if policy == nil || policy.Status != YanCoreVirtualKeyPolicyActive {
		return false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return false
	}
	return policy.ProviderScope == "*" || strings.Contains(","+policy.ProviderScope+",", ","+provider+",")
}

func createYanCoreVirtualKeyPolicyRevisionWithTx(tx *gorm.DB, policy *YanCoreVirtualKeyPolicy, token *Token, actorUserID int, reason string) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 255 || policy == nil || token == nil {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	return tx.Create(&YanCoreVirtualKeyPolicyRevision{PolicyId: policy.Id, TokenId: policy.TokenId, UserId: policy.UserId, ActorUserId: actorUserID, Version: policy.Version, ProviderScope: policy.ProviderScope, ModelScope: token.ModelLimits, SourceScope: token.GetIpLimitsString(), BudgetQuota: token.RemainQuota, ExpiresAt: token.ExpiredTime, TokenStatus: token.Status, MaxRPM: policy.MaxRPM, MaxTPM: policy.MaxTPM, MaxConcurrency: policy.MaxConcurrency, Status: policy.Status, Reason: reason}).Error
}

// GetIpLimitsString returns the compatibility source projection for audits.
func (token *Token) GetIpLimitsString() string {
	if token == nil || token.AllowIps == nil {
		return ""
	}
	return strings.TrimSpace(*token.AllowIps)
}

func createYanCoreVirtualKeyPolicyWithTx(tx *gorm.DB, token *Token, policy *YanCoreVirtualKeyPolicy, actorUserID int, reason string) error {
	if token == nil || policy == nil || token.UserId != policy.UserId {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	policy.TokenId = token.Id
	policy.CreatedAt = time.Now().Unix()
	policy.UpdatedAt = policy.CreatedAt
	if err := validatePersistedYanCoreVirtualKeyPolicy(policy); err != nil {
		return err
	}
	if err := tx.Create(policy).Error; err != nil {
		return err
	}
	return createYanCoreVirtualKeyPolicyRevisionWithTx(tx, policy, token, actorUserID, reason)
}

func CreateYanCoreVirtualKeyWithPolicy(token *Token, policy *YanCoreVirtualKeyPolicy, actorUserID int, reason string) error {
	if token == nil || policy == nil || token.UserId != policy.UserId {
		return ErrYanCoreVirtualKeyPolicyInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		return createYanCoreVirtualKeyPolicyWithTx(tx, token, policy, actorUserID, reason)
	})
}

func GetYanCoreVirtualKeyPolicy(tokenID, userID int) (*YanCoreVirtualKeyPolicy, error) {
	if tokenID <= 0 || userID <= 0 {
		return nil, ErrYanCoreVirtualKeyPolicyNotFound
	}
	var policy YanCoreVirtualKeyPolicy
	if err := DB.Where("token_id = ? AND user_id = ?", tokenID, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &policy, nil
}

func UpdateYanCoreVirtualKeyPolicy(tokenID, userID, actorUserID int, config YanCoreVirtualKeyPolicyConfig) (*YanCoreVirtualKeyPolicy, error) {
	if strings.TrimSpace(config.Reason) == "" {
		return nil, ErrYanCoreVirtualKeyPolicyInvalid
	}
	var updated *YanCoreVirtualKeyPolicy
	var tokenUpdated bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrYanCoreVirtualKeyPolicyNotFound
			}
			return err
		}
		originalTokenStatus := token.Status
		if err := applyYanCoreVirtualKeyTokenUpdate(&token, config.Token); err != nil {
			return err
		}
		var policy YanCoreVirtualKeyPolicy
		if err := lockForUpdate(tx).Where("token_id = ? AND user_id = ?", tokenID, userID).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrYanCoreVirtualKeyPolicyNotFound
			}
			return err
		}
		requiredProviders, err := inferYanCoreProvidersFromToken(&token)
		if err != nil {
			return err
		}
		providers := config.Providers
		if len(providers) == 0 {
			if config.Token != nil && config.Token.Models != nil {
				providers = requiredProviders
			} else {
				providers = strings.Split(policy.ProviderScope, ",")
			}
		}
		providerScope, err := normalizeYanCoreProviderScope(providers)
		if err != nil {
			return err
		}
		for _, required := range requiredProviders {
			if providerScope != "*" && !strings.Contains(","+providerScope+",", ","+required+",") {
				return ErrYanCoreVirtualKeyPolicyInvalid
			}
		}
		if config.MaxRPM == 0 {
			config.MaxRPM = policy.MaxRPM
		}
		if config.MaxTPM == 0 {
			config.MaxTPM = policy.MaxTPM
		}
		if config.MaxConcurrency == 0 {
			config.MaxConcurrency = policy.MaxConcurrency
		}
		statusProvided := config.Status != ""
		if !statusProvided {
			config.Status = policy.Status
		}
		if config.Token != nil && config.Token.Status != nil {
			expectedPolicyStatus := YanCoreVirtualKeyPolicyActive
			if token.Status == common.TokenStatusDisabled {
				expectedPolicyStatus = YanCoreVirtualKeyPolicyDisabled
			}
			if statusProvided && config.Status != expectedPolicyStatus {
				return ErrYanCoreVirtualKeyPolicyInvalid
			}
			config.Status = expectedPolicyStatus
		} else if token.Status != policyTokenStatus(config.Status) && config.Status == policy.Status {
			return ErrYanCoreVirtualKeyPolicyInvalid
		}
		token.Status = policyTokenStatus(config.Status)
		if token.Status == common.TokenStatusEnabled && (token.RemainQuota <= 0 || (token.ExpiredTime > 0 && token.ExpiredTime <= time.Now().Unix())) {
			return ErrYanCoreVirtualKeyPolicyInvalid
		}
		candidate := policy
		candidate.ProviderScope = providerScope
		candidate.MaxRPM = config.MaxRPM
		candidate.MaxTPM = config.MaxTPM
		candidate.MaxConcurrency = config.MaxConcurrency
		candidate.Status = config.Status
		candidate.Version++
		candidate.UpdatedAt = time.Now().Unix()
		if err := validatePersistedYanCoreVirtualKeyPolicy(&candidate); err != nil {
			return err
		}
		if config.Token != nil || token.Status != originalTokenStatus {
			if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, token.UserId).Updates(map[string]any{
				"name": token.Name, "status": token.Status, "expired_time": token.ExpiredTime,
				"remain_quota": token.RemainQuota, "unlimited_quota": false,
				"model_limits_enabled": token.ModelLimitsEnabled, "model_limits": token.ModelLimits,
				"allow_ips": token.AllowIps, "group": token.Group, "cross_group_retry": token.CrossGroupRetry,
			}).Error; err != nil {
				return err
			}
			tokenUpdated = true
		}
		if err := tx.Model(&YanCoreVirtualKeyPolicy{}).Where("id = ?", policy.Id).Updates(map[string]any{"provider_scope": candidate.ProviderScope, "max_rpm": candidate.MaxRPM, "max_tpm": candidate.MaxTPM, "max_concurrency": candidate.MaxConcurrency, "status": candidate.Status, "version": candidate.Version, "updated_at": candidate.UpdatedAt}).Error; err != nil {
			return err
		}
		if err := createYanCoreVirtualKeyPolicyRevisionWithTx(tx, &candidate, &token, actorUserID, config.Reason); err != nil {
			return err
		}
		updated = &candidate
		return nil
	})
	if err == nil && tokenUpdated {
		if cacheErr := InvalidateUserTokensCache(userID); cacheErr != nil {
			common.SysLog("failed to invalidate virtual key cache after policy update: " + cacheErr.Error())
		}
	}
	return updated, err
}

func policyTokenStatus(status string) int {
	if status == YanCoreVirtualKeyPolicyDisabled {
		return common.TokenStatusDisabled
	}
	return common.TokenStatusEnabled
}

func planYanCoreVirtualKeyPolicyRollout(token *Token) (*YanCoreVirtualKeyPolicy, YanCoreVirtualKeyPolicyRolloutItem) {
	item := YanCoreVirtualKeyPolicyRolloutItem{TokenID: token.Id, UserID: token.UserId, ModelScope: strings.TrimSpace(token.ModelLimits)}
	if token.Status != common.TokenStatusEnabled {
		item.Classification = YanCoreVirtualKeyPolicyRolloutReview
		item.Reason = "token_status_not_enabled"
		return nil, item
	}
	if token.UnlimitedQuota || token.RemainQuota <= 0 {
		item.Classification = YanCoreVirtualKeyPolicyRolloutReview
		item.Reason = "finite_positive_budget_required"
		return nil, item
	}
	if token.ExpiredTime > 0 && token.ExpiredTime <= time.Now().Unix() {
		item.Classification = YanCoreVirtualKeyPolicyRolloutReview
		item.Reason = "token_expired"
		return nil, item
	}
	tokenCopy := *token
	policy, err := BuildYanCoreVirtualKeyPolicy(&tokenCopy, nil)
	if err != nil {
		item.Classification = YanCoreVirtualKeyPolicyRolloutReview
		item.Reason = "model_or_source_scope_ambiguous"
		return nil, item
	}
	item.Classification = YanCoreVirtualKeyPolicyRolloutReady
	item.Reason = "model_and_source_scope_valid"
	item.ModelScope = tokenCopy.ModelLimits
	return policy, item
}

// GetYanCoreVirtualKeyPolicyRolloutReport supports a review before the
// feature flag is enabled. It never loads or returns virtual-key material.
func GetYanCoreVirtualKeyPolicyRolloutReport(limit int) (*YanCoreVirtualKeyPolicyRolloutReport, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var tokens []Token
	if err := DB.Select("id", "user_id", "key_hash_enabled", "status", "expired_time", "remain_quota", "unlimited_quota", "model_limits_enabled", "model_limits", "allow_ips").Where("key_hash_enabled = ?", true).Order("id").Find(&tokens).Error; err != nil {
		return nil, err
	}
	tokenIDs := make([]int, 0, len(tokens))
	for _, token := range tokens {
		tokenIDs = append(tokenIDs, token.Id)
	}
	policies := make(map[int]YanCoreVirtualKeyPolicy, len(tokenIDs))
	if len(tokenIDs) > 0 {
		var stored []YanCoreVirtualKeyPolicy
		if err := DB.Select("token_id", "status").Where("token_id IN ?", tokenIDs).Find(&stored).Error; err != nil {
			return nil, err
		}
		for _, policy := range stored {
			policies[policy.TokenId] = policy
		}
	}
	report := &YanCoreVirtualKeyPolicyRolloutReport{GeneratedAt: time.Now().Unix(), TotalHashedKeys: len(tokens), Items: make([]YanCoreVirtualKeyPolicyRolloutItem, 0)}
	for i := range tokens {
		token := &tokens[i]
		if policy, exists := policies[token.Id]; exists {
			if policy.Status == YanCoreVirtualKeyPolicyActive {
				report.ManagedActive++
			} else {
				report.ManagedDisabled++
			}
			continue
		}
		_, item := planYanCoreVirtualKeyPolicyRollout(token)
		if item.Classification == YanCoreVirtualKeyPolicyRolloutReady {
			report.PendingReady++
		} else {
			report.PendingReview++
		}
		if len(report.Items) < limit {
			report.Items = append(report.Items, item)
		}
	}
	return report, nil
}

func normalizeYanCoreVirtualKeyPolicyRolloutTokenIDs(tokenIDs []int) ([]int, error) {
	if len(tokenIDs) == 0 || len(tokenIDs) > 100 {
		return nil, ErrYanCoreVirtualKeyPolicyInvalid
	}
	seen := make(map[int]struct{}, len(tokenIDs))
	result := make([]int, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if tokenID <= 0 {
			return nil, ErrYanCoreVirtualKeyPolicyInvalid
		}
		if _, exists := seen[tokenID]; exists {
			return nil, ErrYanCoreVirtualKeyPolicyInvalid
		}
		seen[tokenID] = struct{}{}
		result = append(result, tokenID)
	}
	sort.Ints(result)
	return result, nil
}

// ApplyYanCoreVirtualKeyPolicyRollout creates policies only for the explicit
// reviewed token IDs. Uncertain keys are frozen instead of being activated.
func ApplyYanCoreVirtualKeyPolicyRollout(tokenIDs []int, actorUserID int, reason string) (*YanCoreVirtualKeyPolicyRolloutResult, error) {
	normalizedIDs, err := normalizeYanCoreVirtualKeyPolicyRolloutTokenIDs(tokenIDs)
	if err != nil || actorUserID <= 0 {
		return nil, ErrYanCoreVirtualKeyPolicyInvalid
	}
	reason = strings.TrimSpace(reason)
	if len(reason) < 3 || len(reason) > 160 {
		return nil, ErrYanCoreVirtualKeyPolicyInvalid
	}
	result := &YanCoreVirtualKeyPolicyRolloutResult{Items: make([]YanCoreVirtualKeyPolicyRolloutItem, 0, len(normalizedIDs))}
	affectedUsers := make(map[int]struct{})
	err = DB.Transaction(func(tx *gorm.DB) error {
		for _, tokenID := range normalizedIDs {
			var token Token
			if err := lockForUpdate(tx).Select("id", "user_id", "key_hash_enabled", "status", "expired_time", "remain_quota", "unlimited_quota", "model_limits_enabled", "model_limits", "allow_ips").Where("id = ? AND key_hash_enabled = ?", tokenID, true).First(&token).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrYanCoreVirtualKeyPolicyRolloutNotPending
				}
				return err
			}
			var existing YanCoreVirtualKeyPolicy
			err := lockForUpdate(tx).Select("id").Where("token_id = ?", token.Id).First(&existing).Error
			if err == nil {
				return ErrYanCoreVirtualKeyPolicyRolloutNotPending
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			policy, item := planYanCoreVirtualKeyPolicyRollout(&token)
			if item.Classification == YanCoreVirtualKeyPolicyRolloutReady {
				result.Activated++
			} else {
				policy = &YanCoreVirtualKeyPolicy{TokenId: token.Id, UserId: token.UserId, ProviderScope: "*", MaxRPM: common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_RPM", YanCoreVirtualKeyPolicyDefaultRPM), MaxTPM: common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_TPM", YanCoreVirtualKeyPolicyDefaultTPM), MaxConcurrency: common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_CONCURRENCY", YanCoreVirtualKeyPolicyDefaultConcurrency), Status: YanCoreVirtualKeyPolicyDisabled, Version: 1}
				if token.Status == common.TokenStatusEnabled {
					token.Status = common.TokenStatusDisabled
					if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, token.UserId).Update("status", token.Status).Error; err != nil {
						return err
					}
				}
				result.Disabled++
			}
			if err := createYanCoreVirtualKeyPolicyWithTx(tx, &token, policy, actorUserID, "admin rollout: "+item.Reason+"; "+reason); err != nil {
				return err
			}
			result.Applied++
			result.Items = append(result.Items, item)
			affectedUsers[token.UserId] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for userID := range affectedUsers {
		if cacheErr := InvalidateUserTokensCache(userID); cacheErr != nil {
			common.SysLog("failed to invalidate virtual key cache after policy rollout: " + cacheErr.Error())
		}
	}
	return result, nil
}

func ListYanCoreVirtualKeyPolicyRevisions(tokenID, userID int) ([]*YanCoreVirtualKeyPolicyRevision, error) {
	if tokenID <= 0 || userID <= 0 {
		return nil, ErrYanCoreVirtualKeyPolicyNotFound
	}
	var revisions []*YanCoreVirtualKeyPolicyRevision
	err := DB.Where("token_id = ? AND user_id = ?", tokenID, userID).Order("version desc").Limit(100).Find(&revisions).Error
	return revisions, err
}

// BackfillYanCoreVirtualKeyPolicies is retained for controlled compatibility
// tests. Startup migration must use the administrator preflight and rollout
// APIs instead of calling this helper.
func BackfillYanCoreVirtualKeyPolicies() error {
	if !YanCoreVirtualKeyPolicyEnabled() {
		return nil
	}
	var tokens []Token
	if err := DB.Where("key_hash_enabled = ?", true).Find(&tokens).Error; err != nil {
		return err
	}
	for i := range tokens {
		token := &tokens[i]
		var count int64
		if err := DB.Model(&YanCoreVirtualKeyPolicy{}).Where("token_id = ?", token.Id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		policy, err := BuildYanCoreVirtualKeyPolicy(token, nil)
		if err != nil {
			policy = &YanCoreVirtualKeyPolicy{TokenId: token.Id, UserId: token.UserId, ProviderScope: "*", MaxRPM: common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_RPM", YanCoreVirtualKeyPolicyDefaultRPM), MaxTPM: common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_TPM", YanCoreVirtualKeyPolicyDefaultTPM), MaxConcurrency: common.GetEnvOrDefault("YANCHUANER_VIRTUAL_KEY_DEFAULT_CONCURRENCY", YanCoreVirtualKeyPolicyDefaultConcurrency), Status: YanCoreVirtualKeyPolicyDisabled, Version: 1}
		}
		if err := validatePersistedYanCoreVirtualKeyPolicy(policy); err != nil {
			return err
		}
		err = DB.Transaction(func(tx *gorm.DB) error {
			var existing int64
			if err := tx.Model(&YanCoreVirtualKeyPolicy{}).Where("token_id = ?", token.Id).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				return nil
			}
			if err := tx.Create(policy).Error; err != nil {
				return err
			}
			reason := "backfill hashed virtual key policy"
			if policy.Status == YanCoreVirtualKeyPolicyDisabled {
				reason = "backfill disabled: model/provider scope is ambiguous"
			}
			return createYanCoreVirtualKeyPolicyRevisionWithTx(tx, policy, token, 0, reason)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
