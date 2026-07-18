/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVirtualKeyIsStoredAsHashAndCannotReplayHash(t *testing.T) {
	truncateTables(t)

	presented, storedHash, prefix, suffix, err := GenerateVirtualKey()
	require.NoError(t, err)
	assert.NotContains(t, storedHash, presented)
	assert.Equal(t, storedHash, HashVirtualKey(presented))
	assert.NotEmpty(t, prefix)
	assert.NotEmpty(t, suffix)

	token := &Token{
		UserId:           1,
		Key:              storedHash,
		KeyHashEnabled:   true,
		KeyDisplayPrefix: prefix,
		KeyDisplaySuffix: suffix,
		Status:           1,
		ExpiredTime:      -1,
		UnlimitedQuota:   true,
	}
	require.NoError(t, DB.Create(token).Error)

	resolved, err := GetTokenByPresentedKey(presented, true)
	require.NoError(t, err)
	assert.Equal(t, token.Id, resolved.Id)

	_, err = GetTokenByPresentedKey(storedHash, true)
	assert.ErrorIs(t, err, ErrVirtualKeyInvalid)

	_, err = GetTokenByPresentedKey("legacy-key", true)
	assert.False(t, errors.Is(err, ErrVirtualKeyInvalid))
}
