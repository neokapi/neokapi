package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// RefKind indicates whether a ref is a stream or tag.
type RefKind string

const (
	RefKindStream RefKind = "stream"
	RefKindTag    RefKind = "tag"
)

// ResolvedRef holds the resolved ref information set by RefResolutionMiddleware.
type ResolvedRef struct {
	Name string  // The ref name (stream or tag slug)
	Kind RefKind // Whether this is a stream or tag
}

// RefResolutionMiddleware resolves the :ref path parameter to either a stream
// (read-write) or a tag (read-only snapshot). It sets the resolved ref on the
// echo context for downstream handlers.
//
// AD-040: Streams and tags share a namespace. Resolution order:
//  1. Exact stream name match → read-write
//  2. Exact tag name match → read-only
//  3. 404
//
// Write operations through a tag ref return 409 Conflict.
func RefResolutionMiddleware(contentStore store.ContentStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ref := c.Param("ref")
			if ref == "" {
				// No ref param in this route — pass through.
				return next(c)
			}

			if contentStore == nil {
				// No content store — skip resolution, handlers will use ref as stream name.
				c.Set("ref", &ResolvedRef{Name: ref, Kind: RefKindStream})
				return next(c)
			}

			pid := projectParam(c)
			if pid == "" {
				pid = c.Param("id")
			}
			if pid == "" {
				// No project context — use ref as-is.
				c.Set("ref", &ResolvedRef{Name: ref, Kind: RefKindStream})
				return next(c)
			}

			ctx := c.Request().Context()

			// Try stream first.
			stream, err := contentStore.GetStream(ctx, pid, ref)
			if err == nil && stream != nil {
				c.Set("ref", &ResolvedRef{Name: ref, Kind: RefKindStream})
				// Also set "stream" on context for backward compat with handlers
				// that use streamParam().
				c.Set("resolved_stream", ref)
				return next(c)
			}

			// Try tag.
			tag, err := contentStore.GetStreamTag(ctx, pid, "", ref)
			if err == nil && tag != nil {
				c.Set("ref", &ResolvedRef{Name: ref, Kind: RefKindTag})
				c.Set("resolved_stream", tag.Stream)
				return next(c)
			}

			// If ref is "main", treat as default stream even if not created yet.
			if ref == "main" {
				c.Set("ref", &ResolvedRef{Name: ref, Kind: RefKindStream})
				c.Set("resolved_stream", ref)
				return next(c)
			}

			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "ref not found: " + ref + " (not a stream or tag)",
			})
		}
	}
}

// RequireWritableRef returns 409 Conflict if the current ref is a tag (read-only).
// Use this as middleware on write routes (PUT, POST, DELETE on content).
func RequireWritableRef() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			resolved, ok := c.Get("ref").(*ResolvedRef)
			if ok && resolved.Kind == RefKindTag {
				return c.JSON(http.StatusConflict, ErrorResponse{
					Error: "cannot write to tag ref: " + resolved.Name + " (tags are read-only snapshots)",
				})
			}
			return next(c)
		}
	}
}
