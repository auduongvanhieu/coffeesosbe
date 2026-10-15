package auth

import (
	"strings"
	"testing"
)

// The emailed token must never be what the database stores, so a leaked table
// cannot be replayed into a password reset.
func TestResetTokenStoresOnlyItsHash(t *testing.T) {
	token, hash, err := newResetToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 {
		t.Errorf("token too short: %d chars", len(token))
	}
	if strings.Contains(hash, token) || hash == token {
		t.Error("stored hash must not contain the token")
	}
	if hashToken(token) != hash {
		t.Error("hashToken must reproduce the stored hash")
	}
	if len(hash) != 64 {
		t.Errorf("hash = %d chars, want 64 (sha256 hex)", len(hash))
	}
}

func TestResetTokensAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		token, _, err := newResetToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[token] {
			t.Fatal("newResetToken repeated a token")
		}
		seen[token] = true
	}
}
