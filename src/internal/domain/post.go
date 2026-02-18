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

type PostType string

type PostStatus string

const (
	PostTypePost PostType = "post"
	PostTypePage PostType = "page"
)

const (
	PostStatusDraft     PostStatus = "draft"
	PostStatusPublished PostStatus = "published"
	PostStatusArchived  PostStatus = "archived"
	PostStatusDeleted   PostStatus = "deleted"
)

type Post struct {
	ID          string
	Type        PostType
	Title       string
	Slug        string
	BodyMD      string
	Status      PostStatus
	AuthorID    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt *time.Time
	DeletedAt   *time.Time
}

// IsValidPostType explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func IsValidPostType(t PostType) bool {
	return t == PostTypePost || t == PostTypePage
}

// IsValidPostStatus explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func IsValidPostStatus(s PostStatus) bool {
	return s == PostStatusDraft || s == PostStatusPublished || s == PostStatusArchived || s == PostStatusDeleted
}
