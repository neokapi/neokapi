package server

import (
	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// blockEditAllowed is the access state a person's edit of a block meets (ABAC),
// as a predicate: the rule the stream's change policy applies to every
// content operation (streamPolicy.access), asked by an action that skips one
// protected block in its own result instead of refusing the whole request.
//
//   - open       → no extra requirement (normal perms apply)
//   - restricted → requires PermReview for the locale, unless the actor owns
//     the block (an owner may keep working their own held content)
//   - published  → requires PermManageProject (re-opening published content is
//     privileged)
func (s *Server) blockEditAllowed(c echo.Context, projectID, blockID, locale string) bool {
	as, ok := s.ContentStore.(platstore.BlockAccessStore)
	if !ok {
		return true
	}
	access, owner, err := as.GetBlockAccess(c.Request().Context(), projectID, refParam(c), blockID)
	if err != nil {
		return true
	}
	switch access {
	case bstore.BlockAccessPublished:
		return hasPermission(c, platauth.PermManageProject)
	case bstore.BlockAccessRestricted:
		if actor, _ := c.Get("user_id").(string); owner != "" && owner == actor {
			return true
		}
		return allowsLanguage(c, platauth.PermReview, locale)
	default:
		return true
	}
}
