package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
)

// HandleSyncTree answers with the stream's content reduced to hashes, at a
// known ref.
//
// GET /sync/:ref/tree[?scope=<pattern>&scope=<pattern>]
//
// This is the fetch a push is built on. It replaces a negotiation that
// descended one item at a time — a few hundred files were a few hundred
// sequential round trips before a byte of content moved, and a first push,
// having no cache, paid it for every item. The producer now diffs its scan
// against this locally and knows exactly which blocks are missing.
//
// The response carries the ref it was computed at, which is what makes the
// producer's local mirror a mirror rather than a hopeful cache: the commit
// asserts that ref, so a tree that went stale between fetch and push is
// refused rather than acted on.
//
// A root hash is served as the ETag, so a warm producer re-validates in one
// conditional request instead of reading the whole tree back.
func (s *Server) HandleSyncTree(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "content store not configured")
	}

	projectID := c.Param("id")
	stream := refParam(c)
	scope := venue.Scope(c.QueryParams()["scope"])

	ctx := c.Request().Context()
	diffEngine := bowsync.NewDiffEngine(s.ContentStore, s.SyncCache)
	tree, itemIDs, err := diffEngine.LoadTree(ctx, projectID, stream, scope)
	if err != nil {
		return serverErr(c, err)
	}

	// The ref is best-effort in the same way the push's is: one that cannot be
	// computed costs the producer its compare-and-swap, never its content.
	currentRef, refErr := bowsync.CurrentRef(ctx, s.streamRefSource(), projectID, stream)
	if refErr != nil {
		currentRef = ref.Ref{}
	}

	root := bowsync.TreeRootHash(tree)
	etag := `"` + root + `"`
	if match := c.Request().Header.Get("If-None-Match"); match != "" && etagMatches(match, etag) {
		c.Response().Header().Set("ETag", etag)
		return c.NoContent(http.StatusNotModified)
	}
	c.Response().Header().Set("ETag", etag)

	// A list rather than a map, in path order, so the body is byte-identical
	// for identical content and the ETag means what it claims.
	paths := bowsync.SortedPaths(tree)
	items := make([]treeItemResponse, 0, len(paths))
	for _, p := range paths {
		ti := tree[p]
		items = append(items, treeItemResponse{
			ID:       itemIDs[p],
			TreeItem: ti,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ref":       currentRef,
		"root_hash": root,
		"items":     items,
	})
}

// treeItemResponse is a tree entry plus the venue's own name for it. The id is
// the venue's, never the producer's: a rename grafts one document's approvals
// onto another, so which document this IS is a claim only the side holding the
// content gets to make.
type treeItemResponse struct {
	venue.TreeItem
	ID string `json:"id"`
}

// etagMatches compares a request's If-None-Match against the tag served,
// tolerating the weak prefix and a comma-separated list.
func etagMatches(header, etag string) bool {
	for candidate := range strings.SplitSeq(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
