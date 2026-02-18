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

type Media struct {
	ID           string
	StoredName   string
	OriginalName string
	MIME         string
	Size         int64
	SHA256       string
	CreatedBy    string
	CreatedAt    time.Time
}
