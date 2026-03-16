// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package sqlite

import "testing"

// TestBuildRemoteDSNAddsAuthToken explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestBuildRemoteDSNAddsAuthToken(t *testing.T) {
	t.Parallel()
	dsn, err := buildRemoteDSN("libsql://example-org.turso.io", "secret-token")
	if err != nil {
		t.Fatalf("buildRemoteDSN: %v", err)
	}
	want := "libsql://example-org.turso.io?authToken=secret-token"
	if dsn != want {
		t.Fatalf("expected %q, got %q", want, dsn)
	}
}

// TestBuildRemoteDSNDoesNotOverrideExistingToken explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestBuildRemoteDSNDoesNotOverrideExistingToken(t *testing.T) {
	t.Parallel()
	dsn, err := buildRemoteDSN("libsql://example-org.turso.io?authToken=already", "secret-token")
	if err != nil {
		t.Fatalf("buildRemoteDSN: %v", err)
	}
	want := "libsql://example-org.turso.io?authToken=already"
	if dsn != want {
		t.Fatalf("expected %q, got %q", want, dsn)
	}
}

// TestBuildRemoteDSNRequiresScheme explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestBuildRemoteDSNRequiresScheme(t *testing.T) {
	t.Parallel()
	if _, err := buildRemoteDSN("example-org.turso.io", "secret-token"); err == nil {
		t.Fatal("expected error for missing scheme")
	}
}
