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
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Version  = 19
	argon2Time     = 3
	argon2Memory   = 64 * 1024
	argon2Threads  = 2
	argon2KeyLen   = 32
	argon2SaltLen  = 16
	passwordPrefix = "argon2id"
)

// HashPassword explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}

	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	encSalt := base64.RawStdEncoding.EncodeToString(salt)
	encHash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("%s$v=%d$m=%d,t=%d,p=%d$%s$%s", passwordPrefix, argon2Version, argon2Memory, argon2Time, argon2Threads, encSalt, encHash), nil
}

// VerifyPassword explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != passwordPrefix {
		return false, errors.New("invalid password hash format")
	}
	if parts[1] != "v=19" {
		return false, errors.New("unsupported argon2 version")
	}

	memory, timeCost, threads, err := parseArgonParams(parts[2])
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	check := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(hash)))
	return subtle.ConstantTimeCompare(check, hash) == 1, nil
}

// parseArgonParams explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parseArgonParams(raw string) (memory, timeCost uint32, threads uint8, _ error) {
	values := strings.Split(raw, ",")
	if len(values) != 3 {
		return 0, 0, 0, errors.New("invalid argon2 params")
	}
	mem, err := strconv.ParseUint(strings.TrimPrefix(values[0], "m="), 10, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse memory: %w", err)
	}
	t, err := strconv.ParseUint(strings.TrimPrefix(values[1], "t="), 10, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse time cost: %w", err)
	}
	p, err := strconv.ParseUint(strings.TrimPrefix(values[2], "p="), 10, 8)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse threads: %w", err)
	}
	return uint32(mem), uint32(t), uint8(p), nil
}
