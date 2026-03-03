// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package auth

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// TestGenerateAndVerifyTOTP explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestGenerateAndVerifyTOTP(t *testing.T) {
	t.Parallel()

	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if len(secret) < 16 {
		t.Fatalf("expected non-trivial secret length, got %d", len(secret))
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	code := totpFromSecret(secret, now)
	if !VerifyTOTP(secret, code, now) {
		t.Fatalf("expected code to verify")
	}
	if VerifyTOTP(secret, "000000", now) {
		t.Fatalf("expected wrong code to fail")
	}
}

// TestRecoveryCodes explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRecoveryCodes(t *testing.T) {
	t.Parallel()

	codes, hashes, err := GenerateRecoveryCodes(5)
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	if len(codes) != 5 || len(hashes) != 5 {
		t.Fatalf("expected 5 recovery codes and hashes")
	}
	for i := range codes {
		if codes[i] == "" || hashes[i] == "" {
			t.Fatalf("expected populated code/hash")
		}
		if got := RecoveryCodeHash(codes[i]); got != hashes[i] {
			t.Fatalf("hash mismatch at index %d", i)
		}
		if !strings.Contains(codes[i], "-") {
			t.Fatalf("expected hyphenated code format, got %q", codes[i])
		}
	}
}

// TestOTPAuthURL explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestOTPAuthURL(t *testing.T) {
	t.Parallel()

	url := OTPAuthURL("Example CMS", "admin@example.com", "ABC123")
	if !strings.HasPrefix(url, "otpauth://totp/") {
		t.Fatalf("unexpected otpauth prefix: %s", url)
	}
	if !strings.Contains(url, "issuer=Example+CMS") {
		t.Fatalf("expected issuer query value")
	}
}

// totpFromSecret explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func totpFromSecret(secret string, now time.Time) string {
	decoded, _ := base32NoPadDecode(secret)
	return totp(decoded, now.UTC().Unix()/totpPeriodSeconds)
}

// base32NoPadDecode explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func base32NoPadDecode(s string) ([]byte, error) {
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(s)))
}
