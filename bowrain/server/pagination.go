package server

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// maxListPageSize caps a single page for the general list endpoints, so
// ?limit=1000000 can never reach SQL verbatim. Endpoints with a domain reason
// for a different ceiling (block queries → store.DefaultBlockLimit) pass their
// own max.
const maxListPageSize = 1000

// pageParams reads the shared limit/offset query parameters, applying a default
// and clamping the limit to max. A missing, unparseable or non-positive limit
// falls back to def; a limit above max is clamped DOWN to max rather than
// rejected, so a client asking for ?limit=1000000 gets a bounded page instead
// of an unbounded scan reaching SQL verbatim. offset is floored at 0.
//
// This is the one place list endpoints derive their page bounds; a handler
// copied from a neighbour inherits the clamp for free.
func pageParams(c echo.Context, def, max int) (limit, offset int) {
	limit = def
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if max > 0 && limit > max {
		limit = max
	}
	if v := c.QueryParam("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

// csvParam reads a comma-separated multi-value query parameter, trimming each
// element and dropping empties. A missing or all-empty parameter returns nil,
// which every consuming query reads as "no filter".
func csvParam(c echo.Context, name string) []string {
	raw := c.QueryParam(name)
	if raw == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
