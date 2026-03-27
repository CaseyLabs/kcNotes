// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package handlers

import (
	"net/http"
	"testing"
)

// TestParseAuditListFilterDefaults explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAuditListFilterDefaults(t *testing.T) {
	t.Parallel()
	r, _ := http.NewRequest(http.MethodGet, "/admin/audit", nil)
	filter := parseAuditListFilter(r)
	if filter.PageSize != defaultAuditPageSize {
		t.Fatalf("expected default page size %d, got %d", defaultAuditPageSize, filter.PageSize)
	}
}

// TestParseAuditListFilterValues explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAuditListFilterValues(t *testing.T) {
	t.Parallel()
	r, _ := http.NewRequest(http.MethodGet, "/admin/audit?q=login&action=login_success&page=2&page_size=50", nil)
	filter := parseAuditListFilter(r)
	if filter.Query != "login" || filter.Action != "login_success" {
		t.Fatalf("unexpected parsed filter: %+v", filter)
	}
	if filter.Page != 2 || filter.PageSize != 50 {
		t.Fatalf("unexpected paging values: %+v", filter)
	}
}
