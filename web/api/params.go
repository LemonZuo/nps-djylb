package api

import (
	"net/http"
	"strconv"
	"strings"
)

// Query-string parsing for the list endpoints. Kept in one place so that every
// collection paginates, sorts and searches identically, and so the bounds are
// enforced once rather than per handler.

// maxPageSize bounds how much one request can ask for. The underlying stores
// are in-memory sync.Maps walked linearly, so an unbounded limit is a cheap way
// for an authenticated user to make the server do a lot of work.
const maxPageSize = 500

// defaultPageSize is used when the caller omits limit. 0 would mean "no
// pagination" to the server-side list helpers, which is not a safe default for
// a public endpoint.
const defaultPageSize = 20

// ListQuery is the common shape of a collection request.
type ListQuery struct {
	// Offset is the number of rows to skip.
	Offset int
	// Limit is the page size, already clamped to [1, maxPageSize].
	Limit int
	// Search is a free-text filter; its meaning is per-collection.
	Search string
	// Sort is a field name understood by the server list helpers; Order is
	// "asc" or "desc".
	Sort  string
	Order string
	// ClientID is the client filter the caller asked for. Handlers must pass
	// it through resolveClientScope rather than using it directly.
	ClientID int
}

// parseListQuery reads the pagination parameters. Names match the old
// bootstrap-table contract (offset/limit/search/sort/order) so that a script
// written against the previous API keeps working.
func parseListQuery(r *http.Request) ListQuery {
	q := r.URL.Query()
	limit := queryInt(q.Get("limit"), defaultPageSize)
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}
	offset := queryInt(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	order := strings.ToLower(strings.TrimSpace(q.Get("order")))
	if order != "asc" {
		// Anything that is not an explicit ascending request is descending,
		// which is what the list helpers assume.
		order = "desc"
	}
	return ListQuery{
		Offset:   offset,
		Limit:    limit,
		Search:   strings.TrimSpace(q.Get("search")),
		Sort:     strings.TrimSpace(q.Get("sort")),
		Order:    order,
		ClientID: queryInt(q.Get("clientId"), 0),
	}
}

// resolveClientScope decides which client id a listing should be filtered by.
//
// For a user it is always their own, whatever the query string said — this is
// the single point where a user is prevented from reading another tenant's
// rows, so it must not be bypassed. For an admin the query parameter is
// honoured, with 0 meaning "all clients".
func resolveClientScope(p *Principal, requested int) int {
	if p == nil {
		return -1 // matches nothing; a nil principal should never get here
	}
	if !p.IsAdmin {
		return p.ClientID
	}
	if requested < 0 {
		return 0
	}
	return requested
}

// queryInt parses an integer parameter, falling back to def when it is absent
// or malformed. A bad value is treated as absent rather than as an error: these
// are display parameters, and rejecting the whole request over an unparseable
// sort offset would be worse than ignoring it.
func queryInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// pathID reads the {id} path segment. It returns ok=false for a missing or
// non-positive id, since every id in the JSON DB is a positive counter.
func pathID(r *http.Request) (int, bool) {
	v, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// normalizeMultiline canonicalises a textarea value: CRLF to LF, trimmed, and
// lowercased. Target lists and ACL rules are compared and dialled
// case-insensitively, and the previous UI applied exactly this transformation
// before storing.
func normalizeMultiline(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")))
}

// splitLines turns a textarea value into a de-duplicated list, preserving the
// last occurrence of each entry. That ordering rule is inherited from the old
// RemoveRepeatedElement: an operator who re-adds an address at the bottom of
// the box expects it to stay where they typed it.
func splitLines(s string) []string {
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		v := strings.TrimSpace(raw[i])
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
