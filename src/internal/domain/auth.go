// TEACHING NOTES:
// Domain files define core business data shapes independent of transport/storage.
// In Go, domain structs are plain data with explicit field types.
// Useful Go concepts to notice:
// 1. Exported field names (capitalized) are visible to other packages.
// 2. `time.Time` is the canonical timestamp type.
// 3. Zero values are meaningful; model optional values intentionally.
// 4. Keeping domain types small helps decouple handlers/stores.
package domain

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleAuthor Role = "author"
)

// IsValidRole explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func IsValidRole(role Role) bool {
	return role == RoleAdmin || role == RoleEditor || role == RoleAuthor
}

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	Disabled     bool
	MFAEnabled   bool
	MFASecret    string
}

type PasskeyCredential struct {
	ID                 string
	UserID             string
	CredentialID       []byte
	PublicKey          []byte
	AttestationType    string
	Transports         []string
	BackupEligible     bool
	BackupState        bool
	SignCount          uint32
	Nickname           string
	WebAuthnCredential []byte
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastUsedAt         *time.Time
}

type WebAuthnChallenge struct {
	ID        string
	UserID    string
	Type      string
	Session   []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type EnrollmentInvitation struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedBy string
	CreatedAt time.Time
}

type SessionUser struct {
	SessionID string
	CSRFToken string
	ExpiresAt time.Time
	User      User
}
