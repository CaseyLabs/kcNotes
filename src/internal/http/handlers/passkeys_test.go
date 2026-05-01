package handlers

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"kcnotes/internal/auth"
	"kcnotes/internal/domain"
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

func TestPasskeyLoginOptionsRequireUserVerification(t *testing.T) {
	wa := newTestWebAuthn(t)

	assertion, session, err := wa.BeginDiscoverableLogin(passkeyLoginOptions()...)
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}

	if assertion.Response.UserVerification != protocol.VerificationRequired {
		t.Fatalf("expected login assertion to require user verification, got %q", assertion.Response.UserVerification)
	}
	if session.UserVerification != protocol.VerificationRequired {
		t.Fatalf("expected login session to require user verification, got %q", session.UserVerification)
	}
}

func TestPasskeyRegistrationOptionsRequireResidentKeyAndUserVerification(t *testing.T) {
	wa := newTestWebAuthn(t)
	user := auth.WebAuthnUser{User: domain.User{ID: "user-1", Email: "admin@example.com"}}

	creation, session, err := wa.BeginRegistration(user, passkeyRegistrationOptions()...)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}

	selection := creation.Response.AuthenticatorSelection
	if selection.ResidentKey != protocol.ResidentKeyRequirementRequired {
		t.Fatalf("expected resident key requirement, got %q", selection.ResidentKey)
	}
	if selection.RequireResidentKey == nil || !*selection.RequireResidentKey {
		t.Fatalf("expected requireResidentKey to be true")
	}
	if selection.UserVerification != protocol.VerificationRequired {
		t.Fatalf("expected registration to require user verification, got %q", selection.UserVerification)
	}
	if session.UserVerification != protocol.VerificationRequired {
		t.Fatalf("expected registration session to require user verification, got %q", session.UserVerification)
	}
}

func newTestWebAuthn(t *testing.T) *webauthn.WebAuthn {
	t.Helper()
	wa, err := auth.NewWebAuthn(auth.WebAuthnConfig{
		RPID:      "localhost",
		RPName:    "kcNotes",
		RPOrigins: []string{"http://localhost:5555"},
	})
	if err != nil {
		t.Fatalf("new webauthn: %v", err)
	}
	return wa
}
