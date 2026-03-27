// TEACHING NOTES:
// Handler files translate HTTP requests into application/store operations.
// In Go's net/http model, handlers are ordinary functions with signature:
// `func(http.ResponseWriter, *http.Request)`.
// Useful Go concepts to notice:
// 1. Request parsing/validation is separate from persistence logic.
// 2. `context.Context` from the request is passed into store calls.
// 3. Errors are mapped to HTTP status codes + HTML fragment/full-page responses.
// 4. HTMX requests are just HTTP with extra headers; server logic stays explicit.
// 5. Handlers often return early on errors to keep happy-path code readable.
package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	storesqlite "kcnotes/internal/store/sqlite"
)

const defaultAuditPageSize = 25

// AuditPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) AuditPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	filter := parseAuditListFilter(r)
	events, total, err := h.store.ListAuditEvents(r.Context(), filter)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load audit log")
		return
	}
	data := h.auditListData(r, user, filter, events, total)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-audit", data)
}

// AuditTable explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) AuditTable(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	filter := parseAuditListFilter(r)
	events, total, err := h.store.ListAuditEvents(r.Context(), filter)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load audit log")
		return
	}
	data := h.auditListData(r, user, filter, events, total)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-audit-table", data)
}

// parseAuditListFilter explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parseAuditListFilter(r *http.Request) storesqlite.AuditListFilter {
	query := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(query.Get("page")))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(query.Get("page_size")))
	if pageSize <= 0 {
		pageSize = defaultAuditPageSize
	}
	return storesqlite.AuditListFilter{
		Query:    strings.TrimSpace(query.Get("q")),
		Action:   strings.TrimSpace(query.Get("action")),
		Page:     page,
		PageSize: pageSize,
	}
}

// auditListData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) auditListData(r *http.Request, user domain.User, filter storesqlite.AuditListFilter, events []domain.AuditEvent, total int) map[string]any {
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = defaultAuditPageSize
	}
	currentPage := filter.Page
	if currentPage <= 0 {
		currentPage = 1
	}
	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if currentPage > totalPages {
		currentPage = totalPages
	}

	queryBase := fmt.Sprintf(
		"q=%s&action=%s&page_size=%d",
		url.QueryEscape(filter.Query),
		url.QueryEscape(filter.Action),
		pageSize,
	)

	return map[string]any{
		"Title":        "Audit Log",
		"CSRFToken":    middleware.CSRFToken(r),
		"UserEmail":    user.Email,
		"UserRole":     string(user.Role),
		"Events":       events,
		"Total":        total,
		"Page":         currentPage,
		"PageSize":     pageSize,
		"TotalPages":   totalPages,
		"HasPrevPage":  currentPage > 1,
		"HasNextPage":  currentPage < totalPages,
		"PrevPage":     currentPage - 1,
		"NextPage":     currentPage + 1,
		"Query":        filter.Query,
		"FilterAction": filter.Action,
		"QueryBase":    queryBase,
	}
}
