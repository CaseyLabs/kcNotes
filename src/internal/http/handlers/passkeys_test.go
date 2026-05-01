package handlers

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestPasskeyChallengeExpiresAtDefaultsWhenLibraryExpiryUnset(t *testing.T) {
	got := passkeyChallengeExpiresAt(&webauthn.SessionData{})
	if !got.After(time.Now().UTC()) {
		t.Fatalf("expected default passkey challenge expiry in the future, got %s", got)
	}

	explicit := time.Now().UTC().Add(time.Hour)
	got = passkeyChallengeExpiresAt(&webauthn.SessionData{Expires: explicit})
	if !got.Equal(explicit) {
		t.Fatalf("expected explicit expiry %s, got %s", explicit, got)
	}
}
