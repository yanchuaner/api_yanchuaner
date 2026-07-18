/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	virtualKeyPrefix     = "yc_"
	virtualKeyHashPrefix = "sha256:"
)

var ErrVirtualKeyInvalid = errors.New("virtual key is invalid")

// GenerateVirtualKey returns the presented key without the OpenAI-compatible
// "sk-" prefix, the one-way database value, and safe display fragments.
func GenerateVirtualKey() (presented string, storedHash string, displayPrefix string, displaySuffix string, err error) {
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", "", err
	}
	// Hex keeps the credential compatible with New API's optional
	// "key-channel" suffix parser because it never contains a hyphen.
	presented = virtualKeyPrefix + hex.EncodeToString(secret)
	storedHash = HashVirtualKey(presented)
	displayPrefix = presented[:10]
	displaySuffix = presented[len(presented)-4:]
	return presented, storedHash, displayPrefix, displaySuffix, nil
}

// HashVirtualKey produces the only secret-derived value stored by the server.
func HashVirtualKey(presented string) string {
	digest := sha256.Sum256([]byte(presented))
	return virtualKeyHashPrefix + hex.EncodeToString(digest[:])
}

func resolvePresentedTokenKey(presented string) (lookupKey string, hashed bool, err error) {
	switch {
	case strings.HasPrefix(presented, virtualKeyPrefix):
		return HashVirtualKey(presented), true, nil
	case strings.HasPrefix(presented, virtualKeyHashPrefix):
		// A database hash must never be accepted as a bearer credential.
		return "", false, ErrVirtualKeyInvalid
	default:
		return presented, false, nil
	}
}

// GetTokenByPresentedKey resolves both legacy New API keys and Yanchuaner
// virtual keys while preventing a stored hash from being replayed directly.
func GetTokenByPresentedKey(presented string, fromDB bool) (*Token, error) {
	lookupKey, hashed, err := resolvePresentedTokenKey(presented)
	if err != nil {
		return nil, err
	}
	token, err := GetTokenByKey(lookupKey, fromDB)
	if err != nil {
		return nil, err
	}
	if token.KeyHashEnabled != hashed {
		return nil, ErrVirtualKeyInvalid
	}
	return token, nil
}
