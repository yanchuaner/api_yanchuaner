package model

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIssueAndRevokeSubjectGrant(t *testing.T) {
	truncateTables(t)
	previous := os.Getenv("YANCHUANER_SUBJECT_SIGNING_SECRET")
	t.Cleanup(func() { _ = os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", previous) })
	require.NoError(t, os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "01234567890123456789012345678901"))
	user := &User{Username: "subject-grant-user", Status: 1, Role: 1}
	require.NoError(t, DB.Create(user).Error)
	raw, grant, err := IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:write chat:read", 300)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
	assert.NotEmpty(t, grant.JTIHash)
	assert.NotContains(t, grant.JTIHash, raw)
	var stored YanCoreSubjectGrant
	require.NoError(t, DB.First(&stored, grant.Id).Error)
	assert.NotContains(t, stored.JTIHash, ".")
	claims, err := ParseSubjectGrant(raw)
	require.NoError(t, err)
	assert.Equal(t, "yc_user_"+strconv.Itoa(user.Id), claims.Subject)
	_, err = ParseSubjectGrantForAudience(raw, "other-app")
	assert.ErrorIs(t, err, ErrSubjectGrantInvalid)
	claims, err = ParseSubjectGrantForAudience(raw, "yanchuaner-ai")
	require.NoError(t, err)
	assert.Equal(t, "ai-web", claims.Application)
	require.NoError(t, DB.Model(&stored).Update("application", "tampered-app").Error)
	_, err = ParseSubjectGrantForAudience(raw, "yanchuaner-ai")
	assert.ErrorIs(t, err, ErrSubjectGrantInvalid)
	require.NoError(t, DB.Model(&stored).Update("application", "ai-web").Error)
	require.NoError(t, RevokeSubjectGrant(user.Id, grant.Id))
	_, err = ParseSubjectGrant(raw)
	assert.ErrorIs(t, err, ErrSubjectGrantRevoked)
}

func TestSubjectGrantRejectsUnregisteredPolicyAndDisabledUser(t *testing.T) {
	truncateTables(t)
	previous := os.Getenv("YANCHUANER_SUBJECT_SIGNING_SECRET")
	t.Cleanup(func() { _ = os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", previous) })
	require.NoError(t, os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "01234567890123456789012345678901"))
	user := &User{Username: "subject-grant-policy-user", Status: 1, Role: 1}
	require.NoError(t, DB.Create(user).Error)
	_, _, err := IssueSubjectGrant(user.Id, "unknown-app", "yanchuaner-ai", "chat:write", 300)
	assert.ErrorIs(t, err, ErrSubjectGrantPolicy)
	_, _, err = IssueSubjectGrant(user.Id, "ai-web", "other-audience", "chat:write", 300)
	assert.ErrorIs(t, err, ErrSubjectGrantPolicy)
	_, _, err = IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "admin:write", 300)
	assert.ErrorIs(t, err, ErrSubjectGrantPolicy)
	raw, _, err := IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:write", 300)
	require.NoError(t, err)
	require.NoError(t, DB.Model(user).Update("status", 2).Error)
	_, err = ParseSubjectGrantForAudience(raw, "yanchuaner-ai")
	assert.ErrorIs(t, err, ErrSubjectGrantRevoked)
}

func TestSubjectGrantRejectsWeakSecretAndInvalidTTL(t *testing.T) {
	truncateTables(t)
	previous := os.Getenv("YANCHUANER_SUBJECT_SIGNING_SECRET")
	t.Cleanup(func() { _ = os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", previous) })
	user := &User{Username: "subject-grant-boundary-user", Status: 1, Role: 1}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "short"))
	_, _, err := IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:read", 300)
	assert.Error(t, err)
	require.NoError(t, os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "01234567890123456789012345678901"))
	_, _, err = IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:read", 0)
	assert.Error(t, err)
	_, _, err = IssueSubjectGrant(user.Id, "ai-web", "yanchuaner-ai", "chat:read", SubjectGrantMaxTTL+1)
	assert.Error(t, err)
}

func TestSubjectGrantCannotBeRevokedByAnotherUser(t *testing.T) {
	truncateTables(t)
	previous := os.Getenv("YANCHUANER_SUBJECT_SIGNING_SECRET")
	t.Cleanup(func() { _ = os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", previous) })
	require.NoError(t, os.Setenv("YANCHUANER_SUBJECT_SIGNING_SECRET", "01234567890123456789012345678901"))
	owner := &User{Username: "subject-grant-owner", Status: 1, Role: 1, AffCode: "subject-owner"}
	other := &User{Username: "subject-grant-other", Status: 1, Role: 1, AffCode: "subject-other"}
	require.NoError(t, DB.Create(owner).Error)
	require.NoError(t, DB.Create(other).Error)
	raw, grant, err := IssueSubjectGrant(owner.Id, "ai-web", "yanchuaner-ai", "chat:read", 300)
	require.NoError(t, err)
	assert.ErrorIs(t, RevokeSubjectGrant(other.Id, grant.Id), gorm.ErrRecordNotFound)
	_, err = ParseSubjectGrantForAudience(raw, "yanchuaner-ai")
	require.NoError(t, err)
}
