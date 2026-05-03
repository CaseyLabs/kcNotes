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
	"github.com/google/uuid"

	"kcnotes/internal/domain"
)

type WebAuthnConfig struct {
	RPID           string
	RPName         string
	RPOrigins      []string
	SiteBaseURL    string
	AppEnv         string
	Attestation    string
	AllowedAAGUIDs []string
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
	attestation, err := parseAttestationConveyance(cfg.Attestation)
	if err != nil {
		return nil, err
	}
	allowedAAGUIDs, err := parseAllowedAAGUIDs(cfg.AllowedAAGUIDs)
	if err != nil {
		return nil, err
	}
	webAuthnConfig := &webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         rpName,
		RPOrigins:             origins,
		AttestationPreference: attestation,
	}
	if len(allowedAAGUIDs) > 0 {
		webAuthnConfig.Filtering = &webauthn.FilteringConfig{PermittedAAGUIDs: allowedAAGUIDs}
	}
	return webauthn.New(webAuthnConfig)
}

func parseAttestationConveyance(value string) (protocol.ConveyancePreference, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return protocol.PreferNoAttestation, nil
	case "indirect":
		return protocol.PreferIndirectAttestation, nil
	case "direct":
		return protocol.PreferDirectAttestation, nil
	case "enterprise":
		return protocol.PreferEnterpriseAttestation, nil
	default:
		return "", fmt.Errorf("invalid WEBAUTHN_ATTESTATION_CONVEYANCE %q: expected none, indirect, direct, or enterprise", value)
	}
}

func parseAllowedAAGUIDs(values []string) ([]uuid.UUID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid WEBAUTHN_ALLOWED_AAGUIDS value %q: %w", value, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func EnforceAllowedAAGUIDs(credential *webauthn.Credential, filtering *webauthn.FilteringConfig) error {
	if filtering == nil || len(filtering.PermittedAAGUIDs) == 0 {
		return nil
	}
	if credential == nil {
		return fmt.Errorf("credential is required for AAGUID policy")
	}
	aaguid, err := uuid.FromBytes(credential.Authenticator.AAGUID)
	if err != nil {
		return fmt.Errorf("invalid credential AAGUID: %w", err)
	}
	for _, permitted := range filtering.PermittedAAGUIDs {
		if aaguid == permitted {
			return nil
		}
	}
	return fmt.Errorf("credential AAGUID is not permitted")
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
