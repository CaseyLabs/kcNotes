package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"kcnotes/internal/domain"
)

type WebAuthnConfig struct {
	RPID        string
	RPName      string
	RPOrigins   []string
	SiteBaseURL string
	AppEnv      string
}

func NewWebAuthn(cfg WebAuthnConfig) (*webauthn.WebAuthn, error) {
	rpName := strings.TrimSpace(cfg.RPName)
	if rpName == "" {
		rpName = "kcNotes"
	}
	origins := cfg.RPOrigins
	if len(origins) == 0 && strings.TrimSpace(cfg.SiteBaseURL) != "" {
		origins = []string{strings.TrimRight(strings.TrimSpace(cfg.SiteBaseURL), "/")}
	}
	rpID := strings.TrimSpace(cfg.RPID)
	if rpID == "" && len(origins) > 0 {
		rpID = hostFromOrigin(origins[0])
	}
	if rpID == "" && cfg.AppEnv == "dev" {
		rpID = "localhost"
		for port := 5555; port <= 5565; port++ {
			origins = append(origins, fmt.Sprintf("http://localhost:%d", port))
		}
		origins = append(origins, "http://localhost:8080")
	}
	if rpID == "" || len(origins) == 0 {
		return nil, fmt.Errorf("WEBAUTHN_RP_ID and WEBAUTHN_ORIGINS are required outside dev when SITE_BASE_URL is empty")
	}
	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     origins,
	})
}

type WebAuthnUser struct {
	User       domain.User
	Credential []webauthn.Credential
}

func (u WebAuthnUser) WebAuthnID() []byte {
	return []byte(u.User.ID)
}

func (u WebAuthnUser) WebAuthnName() string {
	return u.User.Email
}

func (u WebAuthnUser) WebAuthnDisplayName() string {
	return u.User.Email
}

func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credential
}

func CredentialToDomain(id, userID, nickname string, credential *webauthn.Credential) (domain.PasskeyCredential, error) {
	raw, err := json.Marshal(credential)
	if err != nil {
		return domain.PasskeyCredential{}, fmt.Errorf("marshal webauthn credential: %w", err)
	}
	transports := make([]string, 0, len(credential.Transport))
	for _, transport := range credential.Transport {
		transports = append(transports, string(transport))
	}
	return domain.PasskeyCredential{
		ID:                 id,
		UserID:             userID,
		CredentialID:       credential.ID,
		PublicKey:          credential.PublicKey,
		AttestationType:    credential.AttestationType,
		Transports:         transports,
		BackupEligible:     credential.Flags.BackupEligible,
		BackupState:        credential.Flags.BackupState,
		SignCount:          credential.Authenticator.SignCount,
		Nickname:           strings.TrimSpace(nickname),
		WebAuthnCredential: raw,
	}, nil
}

func DomainCredentialToWebAuthn(credential domain.PasskeyCredential) (webauthn.Credential, error) {
	var result webauthn.Credential
	if len(credential.WebAuthnCredential) > 0 {
		if err := json.Unmarshal(credential.WebAuthnCredential, &result); err != nil {
			return webauthn.Credential{}, fmt.Errorf("unmarshal webauthn credential: %w", err)
		}
		return result, nil
	}
	transports := make([]protocol.AuthenticatorTransport, 0, len(credential.Transports))
	for _, transport := range credential.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(transport))
	}
	result.ID = credential.CredentialID
	result.PublicKey = credential.PublicKey
	result.AttestationType = credential.AttestationType
	result.Transport = transports
	result.Flags.BackupEligible = credential.BackupEligible
	result.Flags.BackupState = credential.BackupState
	result.Authenticator.SignCount = credential.SignCount
	return result, nil
}

func DomainCredentialsToWebAuthn(credentials []domain.PasskeyCredential) ([]webauthn.Credential, error) {
	items := make([]webauthn.Credential, 0, len(credentials))
	for _, credential := range credentials {
		item, err := DomainCredentialToWebAuthn(credential)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func EncodeSession(session *webauthn.SessionData) ([]byte, error) {
	return json.Marshal(session)
}

func DecodeSession(raw []byte) (webauthn.SessionData, error) {
	var session webauthn.SessionData
	err := json.Unmarshal(raw, &session)
	return session, err
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func Base64URL(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func hostFromOrigin(origin string) string {
	trimmed := strings.TrimSpace(origin)
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	if idx := strings.Index(trimmed, ":"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return trimmed
}
