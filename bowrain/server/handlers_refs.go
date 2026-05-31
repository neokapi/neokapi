package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// ProjectRef is a single entry in the unified ref listing. A "ref" is either a
// stream (a movable branch) or a tag (an immutable marker pinned to a cursor),
// mirroring the git-style streams+tags model (Bowrain AD-011).
type ProjectRef struct {
	// Type is "stream" or "tag".
	Type string `json:"type"`
	// Name is the ref name (e.g. "main", "v2.0", "release-2024").
	Name string `json:"name"`
	// Stream is the owning stream. For a stream ref it equals Name; for a tag
	// it is the stream the tag was pinned on.
	Stream string `json:"stream"`
	// Stream-only fields (zero for tags).
	Parent   string `json:"parent,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	Locked   bool   `json:"locked,omitempty"`
	// Tag-only fields (zero for streams).
	Kind   store.StreamTagKind `json:"kind,omitempty"`
	Cursor int64               `json:"cursor,omitempty"`
}

// HandleListProjectRefs returns a unified listing of a project's refs — both
// streams (branches) and tags — for the GitHub-style /:id/refs endpoint
// (Bowrain AD-011). Use ?include_archived=true to include archived streams and
// ?kind=<kind> to filter tags by kind.
func (s *Server) HandleListProjectRefs(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	ctx := c.Request().Context()
	projectID := c.Param("id")
	includeArchived := c.QueryParam("include_archived") == "true"
	kind := store.StreamTagKind(c.QueryParam("kind"))

	streams, err := s.ContentStore.ListStreams(ctx, projectID, includeArchived)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	tags, err := s.ContentStore.ListProjectTags(ctx, projectID, kind)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	refs := make([]ProjectRef, 0, len(streams)+len(tags))
	for _, st := range streams {
		if st == nil {
			continue
		}
		refs = append(refs, ProjectRef{
			Type:     "stream",
			Name:     st.Name,
			Stream:   st.Name,
			Parent:   st.Parent,
			Archived: st.Archived,
			Locked:   st.Locked,
		})
	}
	for _, tg := range tags {
		if tg == nil {
			continue
		}
		refs = append(refs, ProjectRef{
			Type:   "tag",
			Name:   tg.Name,
			Stream: tg.Stream,
			Kind:   tg.Kind,
			Cursor: tg.Cursor,
		})
	}

	return c.JSON(http.StatusOK, refs)
}
