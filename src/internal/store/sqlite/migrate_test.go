// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package sqlite

import "testing"

// TestSplitSQLStatementsSimple explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestSplitSQLStatementsSimple(t *testing.T) {
	t.Parallel()
	sql := `
CREATE TABLE a (id TEXT);
CREATE TABLE b (id TEXT);
`
	got := splitSQLStatements(sql)
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(got))
	}
}

// TestSplitSQLStatementsTriggerBlock explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestSplitSQLStatementsTriggerBlock(t *testing.T) {
	t.Parallel()
	sql := `
CREATE TABLE posts(id TEXT);
CREATE TRIGGER posts_ai AFTER INSERT ON posts BEGIN
  INSERT INTO posts(id) VALUES (NEW.id);
END;
`
	got := splitSQLStatements(sql)
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(got))
	}
}
