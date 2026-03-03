// TEACHING NOTES:
// This file is in the authentication/crypto area.
// Security-related Go code should be explicit, side-effect-light, and fail-closed.
// Useful Go concepts to notice:
// 1. Byte slices (`[]byte`) are common for crypto inputs/outputs.
// 2. Errors are first-class values and should be wrapped with context.
// 3. Deterministic parsing/validation prevents subtle auth bypasses.
// 4. Time handling (`time.Time`, `time.Duration`) drives token windows/expiry.
// 5. Table-driven tests are the standard style for edge-case coverage.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	totpPeriodSeconds = 30
	totpDigits        = 6
)

// GenerateTOTPSecret explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// VerifyTOTP explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func VerifyTOTP(secret, code string, now time.Time) bool {
	normalized := normalizeOTPCode(code)
	if len(normalized) != totpDigits {
		return false
	}
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(secretBytes) == 0 {
		return false
	}
	counter := now.UTC().Unix() / totpPeriodSeconds
	for _, delta := range []int64{-1, 0, 1} {
		candidate := totp(secretBytes, counter+delta)
		if candidate == normalized {
			return true
		}
	}
	return false
}

// OTPAuthURL explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func OTPAuthURL(issuer, accountName, secret string) string {
	safeIssuer := strings.TrimSpace(issuer)
	if safeIssuer == "" {
		safeIssuer = "kcNotes"
	}
	label := url.PathEscape(safeIssuer + ":" + strings.TrimSpace(accountName))
	q := url.Values{}
	q.Set("secret", strings.ToUpper(strings.TrimSpace(secret)))
	q.Set("issuer", safeIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", strconv.Itoa(totpDigits))
	q.Set("period", strconv.Itoa(totpPeriodSeconds))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// GenerateRecoveryCodes explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func GenerateRecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		return nil, nil, fmt.Errorf("count must be positive")
	}
	codes := make([]string, 0, count)
	hashes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, RecoveryCodeHash(code))
	}
	return codes, hashes, nil
}

// RecoveryCodeHash explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func RecoveryCodeHash(code string) string {
	normalized := normalizeRecoveryCode(code)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// normalizeOTPCode explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func normalizeOTPCode(code string) string {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	return code
}

// normalizeRecoveryCode explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func normalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	return code
}

// generateRecoveryCode explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func generateRecoveryCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	raw := strings.ToUpper(hex.EncodeToString(buf))
	return raw[:4] + "-" + raw[4:], nil
}

// totp explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func totp(secret []byte, counter int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	otp := binCode % 1_000_000
	return fmt.Sprintf("%06d", otp)
}
