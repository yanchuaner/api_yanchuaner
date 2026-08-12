/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const (
	SubjectGrantIssuer = "yancore"
	SubjectGrantType   = "yc-subject-grant"
	SubjectGrantMaxTTL = int64(24 * time.Hour / time.Second)
)

var (
	ErrSubjectGrantInvalid = errors.New("subject grant is invalid")
	ErrSubjectGrantRevoked = errors.New("subject grant is revoked")
	ErrSubjectGrantPolicy  = errors.New("subject grant policy does not allow this request")
)

var subjectGrantPolicies = map[string]struct {
	Audience string
	Scopes   map[string]bool
	MaxTTL   int64
}{
	"ai-web": {
		Audience: "yanchuaner-ai",
		MaxTTL:   15 * 60,
		Scopes: map[string]bool{
			"chat:read":  true,
			"chat:write": true,
		},
	},
}

type YanCoreSubjectGrant struct {
	Id          int64  `json:"id"`
	UserId      int    `json:"user_id" gorm:"index;not null"`
	Application string `json:"application" gorm:"type:varchar(64);index;not null"`
	Audience    string `json:"audience" gorm:"type:varchar(128);index;not null"`
	Scopes      string `json:"scopes" gorm:"type:varchar(512);not null"`
	JTIHash     string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	ExpiresAt   int64  `json:"expires_at" gorm:"index;not null"`
	RevokedAt   int64  `json:"revoked_at" gorm:"index;not null;default:0"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;index"`
	LastUsedAt  int64  `json:"last_used_at" gorm:"index;not null;default:0"`
}

type SubjectGrantClaims struct {
	Application string `json:"app"`
	Scopes      string `json:"scp"`
	GrantType   string `json:"grant_type"`
	Admin       bool   `json:"adm"`
	jwt.RegisteredClaims
}

func subjectSigningSecret() ([]byte, error) {
	secret := strings.TrimSpace(common.GetEnvOrDefaultString("YANCHUANER_SUBJECT_SIGNING_SECRET", ""))
	if len(secret) < 32 {
		return nil, errors.New("YANCHUANER_SUBJECT_SIGNING_SECRET must contain at least 32 characters")
	}
	return []byte(secret), nil
}

func hashSubjectGrantID(jti string) string {
	digest := sha256.Sum256([]byte(jti))
	return hex.EncodeToString(digest[:])
}

func normalizeSubjectGrantInput(application, audience, scopes string, ttl int64) (string, string, string, int64, error) {
	application = strings.TrimSpace(application)
	audience = strings.TrimSpace(audience)
	scopes = strings.TrimSpace(scopes)
	if application == "" || len(application) > 64 || audience == "" || len(audience) > 128 || scopes == "" || len(scopes) > 512 {
		return "", "", "", 0, errors.New("application, audience and scopes are required and bounded")
	}
	if ttl <= 0 || ttl > SubjectGrantMaxTTL {
		return "", "", "", 0, fmt.Errorf("ttl must be between 1 and %d seconds", SubjectGrantMaxTTL)
	}
	policy, ok := subjectGrantPolicies[application]
	if !ok || policy.Audience != audience || ttl > policy.MaxTTL {
		return "", "", "", 0, ErrSubjectGrantPolicy
	}
	requestedScopes := strings.Fields(strings.ReplaceAll(scopes, ",", " "))
	if len(requestedScopes) == 0 {
		return "", "", "", 0, ErrSubjectGrantPolicy
	}
	seen := make(map[string]bool, len(requestedScopes))
	for _, scope := range requestedScopes {
		if !policy.Scopes[scope] || seen[scope] {
			return "", "", "", 0, ErrSubjectGrantPolicy
		}
		seen[scope] = true
	}
	scopes = strings.Join(requestedScopes, " ")
	return application, audience, scopes, ttl, nil
}

func IssueSubjectGrant(userID int, application, audience, scopes string, ttl int64) (string, *YanCoreSubjectGrant, error) {
	if userID <= 0 {
		return "", nil, errors.New("user id is required")
	}
	application, audience, scopes, ttl, err := normalizeSubjectGrantInput(application, audience, scopes, ttl)
	if err != nil {
		return "", nil, err
	}
	secret, err := subjectSigningSecret()
	if err != nil {
		return "", nil, err
	}
	var user User
	if err := DB.Select("id", "status", "role").First(&user, userID).Error; err != nil || user.Status != common.UserStatusEnabled {
		return "", nil, ErrSubjectGrantRevoked
	}
	now := time.Now().Unix()
	jti := common.GetUUID()
	expiresAt := now + ttl
	grant := &YanCoreSubjectGrant{
		UserId:      userID,
		Application: application,
		Audience:    audience,
		Scopes:      scopes,
		JTIHash:     hashSubjectGrantID(jti),
		ExpiresAt:   expiresAt,
	}
	if err := DB.Create(grant).Error; err != nil {
		return "", nil, err
	}
	claims := SubjectGrantClaims{
		Application: application,
		Scopes:      scopes,
		GrantType:   SubjectGrantType,
		Admin:       user.Role == common.RoleRootUser,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    SubjectGrantIssuer,
			Subject:   fmt.Sprintf("yc_user_%d", userID),
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(time.Unix(expiresAt, 0)),
			IssuedAt:  jwt.NewNumericDate(time.Unix(now, 0)),
			NotBefore: jwt.NewNumericDate(time.Unix(now, 0)),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		_ = DB.Delete(grant).Error
		return "", nil, err
	}
	return signed, grant, nil
}

func ParseSubjectGrant(raw string) (*SubjectGrantClaims, error) {
	return parseSubjectGrant(raw, "")
}

func ParseSubjectGrantForAudience(raw, expectedAudience string) (*SubjectGrantClaims, error) {
	if strings.TrimSpace(expectedAudience) == "" {
		return nil, errors.New("expected audience is required")
	}
	return parseSubjectGrant(raw, expectedAudience)
}

func parseSubjectGrant(raw, expectedAudience string) (*SubjectGrantClaims, error) {
	secret, err := subjectSigningSecret()
	if err != nil {
		return nil, err
	}
	claims := &SubjectGrantClaims{}
	options := []jwt.ParserOption{
		jwt.WithIssuer(SubjectGrantIssuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	}
	if expectedAudience != "" {
		options = append(options, jwt.WithAudience(expectedAudience))
	}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrSubjectGrantInvalid
		}
		return secret, nil
	}, options...)
	if err != nil || token == nil || !token.Valid || claims.GrantType != SubjectGrantType || claims.ID == "" || claims.Subject == "" {
		return nil, ErrSubjectGrantInvalid
	}
	if expectedAudience != "" {
		matched := false
		for _, audience := range claims.Audience {
			if audience == expectedAudience {
				matched = true
				break
			}
		}
		if !matched {
			return nil, ErrSubjectGrantInvalid
		}
	}
	var grant YanCoreSubjectGrant
	if err := DB.Where("jti_hash = ?", hashSubjectGrantID(claims.ID)).First(&grant).Error; err != nil {
		return nil, ErrSubjectGrantInvalid
	}
	now := time.Now().Unix()
	if grant.RevokedAt > 0 || grant.ExpiresAt <= now {
		return nil, ErrSubjectGrantRevoked
	}
	grantAudienceMatched := false
	for _, audience := range claims.Audience {
		if audience == grant.Audience {
			grantAudienceMatched = true
			break
		}
	}
	if !grantAudienceMatched || claims.Subject != fmt.Sprintf("yc_user_%d", grant.UserId) || grant.Application != claims.Application || grant.Scopes != claims.Scopes || grant.ExpiresAt != claims.ExpiresAt.Unix() {
		return nil, ErrSubjectGrantInvalid
	}
	user := User{}
	if err := DB.Select("id", "status").First(&user, grant.UserId).Error; err != nil || user.Status != common.UserStatusEnabled {
		return nil, ErrSubjectGrantRevoked
	}
	if err := DB.Model(&grant).Update("last_used_at", now).Error; err != nil {
		return nil, err
	}
	return claims, nil
}

func RevokeSubjectGrant(userID int, id int64) error {
	if userID <= 0 || id <= 0 {
		return errors.New("grant id is invalid")
	}
	result := DB.Model(&YanCoreSubjectGrant{}).Where("id = ? AND user_id = ? AND revoked_at = 0", id, userID).Update("revoked_at", time.Now().Unix())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func GetSubjectGrants(userID int) ([]*YanCoreSubjectGrant, error) {
	var grants []*YanCoreSubjectGrant
	err := DB.Where("user_id = ?", userID).Order("id desc").Limit(100).Find(&grants).Error
	return grants, err
}
