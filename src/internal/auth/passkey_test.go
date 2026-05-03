package auth

import (
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

func TestNewWebAuthnAttestationConveyance(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  protocol.ConveyancePreference
	}{
		{name: "default", value: "", want: protocol.PreferNoAttestation},
		{name: "none", value: "none", want: protocol.PreferNoAttestation},
		{name: "indirect", value: "indirect", want: protocol.PreferIndirectAttestation},
		{name: "direct", value: "direct", want: protocol.PreferDirectAttestation},
		{name: "enterprise", value: "enterprise", want: protocol.PreferEnterpriseAttestation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wa, err := NewWebAuthn(WebAuthnConfig{
				RPID:        "localhost",
				RPName:      "kcNotes",
				RPOrigins:   []string{"http://localhost:5555"},
				Attestation: tt.value,
			})
			if err != nil {
				t.Fatalf("new webauthn: %v", err)
			}
			if wa.Config.AttestationPreference != tt.want {
				t.Fatalf("expected attestation preference %q, got %q", tt.want, wa.Config.AttestationPreference)
			}
		})
	}
}

func TestNewWebAuthnRejectsInvalidAttestationConveyance(t *testing.T) {
	_, err := NewWebAuthn(WebAuthnConfig{
		RPID:        "localhost",
		RPName:      "kcNotes",
		RPOrigins:   []string{"http://localhost:5555"},
		Attestation: "silent-weaken",
	})
	if err == nil {
		t.Fatalf("expected invalid attestation conveyance to fail")
	}
	if !strings.Contains(err.Error(), "WEBAUTHN_ATTESTATION_CONVEYANCE") {
		t.Fatalf("expected env var name in error, got %v", err)
	}
}

func TestNewWebAuthnAllowedAAGUIDs(t *testing.T) {
	wa, err := NewWebAuthn(WebAuthnConfig{
		RPID:      "localhost",
		RPName:    "kcNotes",
		RPOrigins: []string{"http://localhost:5555"},
		AllowedAAGUIDs: []string{
			"00000000-0000-0000-0000-000000000001",
			"00000000-0000-0000-0000-000000000002",
		},
	})
	if err != nil {
		t.Fatalf("new webauthn: %v", err)
	}
	if wa.Config.Filtering == nil {
		t.Fatalf("expected filtering config")
	}
	if got := len(wa.Config.Filtering.PermittedAAGUIDs); got != 2 {
		t.Fatalf("expected 2 permitted AAGUIDs, got %d", got)
	}
}

func TestNewWebAuthnRejectsInvalidAllowedAAGUID(t *testing.T) {
	_, err := NewWebAuthn(WebAuthnConfig{
		RPID:           "localhost",
		RPName:         "kcNotes",
		RPOrigins:      []string{"http://localhost:5555"},
		AllowedAAGUIDs: []string{"not-a-uuid"},
	})
	if err == nil {
		t.Fatalf("expected invalid AAGUID to fail")
	}
	if !strings.Contains(err.Error(), "WEBAUTHN_ALLOWED_AAGUIDS") {
		t.Fatalf("expected env var name in error, got %v", err)
	}
}

func TestEnforceAllowedAAGUIDsRejectsZeroAAGUID(t *testing.T) {
	err := EnforceAllowedAAGUIDs(
		&webauthn.Credential{Authenticator: webauthn.Authenticator{AAGUID: uuid.Nil[:]}},
		&webauthn.FilteringConfig{PermittedAAGUIDs: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")}},
	)
	if err == nil {
		t.Fatalf("expected zero AAGUID to be rejected when allowlist is configured")
	}
}

func TestEnforceAllowedAAGUIDsAcceptsPermittedAAGUID(t *testing.T) {
	permitted := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	err := EnforceAllowedAAGUIDs(
		&webauthn.Credential{Authenticator: webauthn.Authenticator{AAGUID: permitted[:]}},
		&webauthn.FilteringConfig{PermittedAAGUIDs: []uuid.UUID{permitted}},
	)
	if err != nil {
		t.Fatalf("expected permitted AAGUID to pass: %v", err)
	}
}
