package middleware

import "testing"

func TestAccessTokenFromAuthorization(t *testing.T) {
	tests := map[string]string{
		"raw-token":         "raw-token",
		"Bearer access-key": "access-key",
		"bearer access-key": "access-key",
		"  Bearer key==  ":  "key==",
	}

	for input, expected := range tests {
		if actual := accessTokenFromAuthorization(input); actual != expected {
			t.Fatalf("accessTokenFromAuthorization(%q) = %q, want %q", input, actual, expected)
		}
	}
}
