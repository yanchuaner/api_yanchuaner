package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
)

func TestTrustedOAuthRoleOnlyAcceptsYanchuanerProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider    *oauth.GenericOAuthProvider
		role        string
		expected    int
		managesRole bool
	}{
		{
			name:        "main site admin becomes root",
			provider:    oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "yanchuaner"}),
			role:        "admin",
			expected:    common.RoleRootUser,
			managesRole: true,
		},
		{
			name:        "verified student stays common user",
			provider:    oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "yanchuaner"}),
			role:        "student",
			expected:    common.RoleCommonUser,
			managesRole: true,
		},
		{
			name:        "other generic provider cannot assign roles",
			provider:    oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{Slug: "other"}),
			role:        "admin",
			expected:    common.RoleCommonUser,
			managesRole: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			role, managesRole := trustedOAuthRole(testCase.provider, &oauth.OAuthUser{
				Extra: map[string]any{"role": testCase.role},
			})
			if role != testCase.expected || managesRole != testCase.managesRole {
				t.Fatalf("trustedOAuthRole() = (%d, %v), want (%d, %v)", role, managesRole, testCase.expected, testCase.managesRole)
			}
		})
	}
}
