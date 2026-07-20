/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"errors"
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
	ErrYanCoreVirtualKeyPolicyInvalid       = errors.New("yancore virtual key policy is invalid")
	ErrYanCoreVirtualKeyPolicyNotFound      = errors.New("yancore virtual key policy not found")
	ErrYanCoreVirtualKeyPolicyModelRequired = errors.New("yancore virtual key policy requires a model allowlist")
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
	Providers      []string `json:"providers"`
	MaxRPM         int      `json:"max_rpm"`
	MaxTPM         int      `json:"max_tpm"`
	MaxConcurrency int      `json:"max_concurrency"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason"`
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
	return tx.Create(&YanCoreVirtualKeyPolicyRevision{PolicyId: policy.Id, TokenId: policy.TokenId, UserId: policy.UserId, ActorUserId: actorUserID, Version: policy.Version, ProviderScope: policy.ProviderScope, ModelScope: token.ModelLimits, SourceScope: token.GetIpLimitsString(), MaxRPM: policy.MaxRPM, MaxTPM: policy.MaxTPM, MaxConcurrency: policy.MaxConcurrency, Status: policy.Status, Reason: reason}).Error
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
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrYanCoreVirtualKeyPolicyNotFound
			}
			return err
		}
		var policy YanCoreVirtualKeyPolicy
		if err := lockForUpdate(tx).Where("token_id = ? AND user_id = ?", tokenID, userID).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrYanCoreVirtualKeyPolicyNotFound
			}
			return err
		}
		providers := config.Providers
		if len(providers) == 0 {
			providers = strings.Split(policy.ProviderScope, ",")
		}
		providerScope, err := normalizeYanCoreProviderScope(providers)
		if err != nil {
			return err
		}
		requiredProviders, err := inferYanCoreProvidersFromToken(&token)
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
		if config.Status == "" {
			config.Status = policy.Status
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
		if err := tx.Model(&YanCoreVirtualKeyPolicy{}).Where("id = ?", policy.Id).Updates(map[string]any{"provider_scope": candidate.ProviderScope, "max_rpm": candidate.MaxRPM, "max_tpm": candidate.MaxTPM, "max_concurrency": candidate.MaxConcurrency, "status": candidate.Status, "version": candidate.Version, "updated_at": candidate.UpdatedAt}).Error; err != nil {
			return err
		}
		if err := createYanCoreVirtualKeyPolicyRevisionWithTx(tx, &candidate, &token, actorUserID, config.Reason); err != nil {
			return err
		}
		updated = &candidate
		return nil
	})
	return updated, err
}

func ListYanCoreVirtualKeyPolicyRevisions(tokenID, userID int) ([]*YanCoreVirtualKeyPolicyRevision, error) {
	if tokenID <= 0 || userID <= 0 {
		return nil, ErrYanCoreVirtualKeyPolicyNotFound
	}
	var revisions []*YanCoreVirtualKeyPolicyRevision
	err := DB.Where("token_id = ? AND user_id = ?", tokenID, userID).Order("version desc").Limit(100).Find(&revisions).Error
	return revisions, err
}

// BackfillYanCoreVirtualKeyPolicies creates a disabled policy for ambiguous
// legacy hashed keys instead of silently granting an unrestricted policy.
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
