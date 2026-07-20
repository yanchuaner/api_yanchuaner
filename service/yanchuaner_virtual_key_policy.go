/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const yanCoreVirtualKeyPolicyContextKey = "yancore_virtual_key_policy"

var (
	ErrYanCoreVirtualKeyPolicyMissing  = errors.New("yancore virtual key policy is missing")
	ErrYanCoreVirtualKeyProviderDenied = errors.New("yancore virtual key provider is not allowed")
)

func SetYanCoreVirtualKeyPolicyContext(c *gin.Context, policy *model.YanCoreVirtualKeyPolicy) {
	if c != nil && policy != nil {
		c.Set(yanCoreVirtualKeyPolicyContextKey, policy)
	}
}

func YanCoreVirtualKeyPolicyFromContext(c *gin.Context) *model.YanCoreVirtualKeyPolicy {
	if c == nil {
		return nil
	}
	value, ok := c.Get(yanCoreVirtualKeyPolicyContextKey)
	if !ok {
		return nil
	}
	policy, _ := value.(*model.YanCoreVirtualKeyPolicy)
	return policy
}

// CheckYanCoreVirtualKeyProvider is called after channel selection, so the
// policy is checked against the actual gateway provider rather than a user
// controlled model alias alone.
func CheckYanCoreVirtualKeyProvider(c *gin.Context, modelName string, channelType int) error {
	if !model.YanCoreVirtualKeyPolicyEnabled() {
		return nil
	}
	policy := YanCoreVirtualKeyPolicyFromContext(c)
	if policy == nil {
		return ErrYanCoreVirtualKeyPolicyMissing
	}
	provider := YanCoreProviderForRequest(modelName, channelType)
	if !policy.AllowsProvider(provider) {
		return fmt.Errorf("%w: %s", ErrYanCoreVirtualKeyProviderDenied, strings.TrimSpace(provider))
	}
	return nil
}
