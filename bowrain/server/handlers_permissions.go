package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// CallerPermissionsResponse reports what the caller may do on one project.
// Languages is the set they are scoped to for the language-scoped permissions
// (translate, review); empty means every language.
type CallerPermissionsResponse struct {
	Permissions []string `json:"permissions"`
	Languages   []string `json:"languages"`
}

// HandleGetCallerPermissions answers with the caller's own effective
// permissions on the project, already resolved by ProjectAccessMiddleware
// (project membership, workspace-role fallback, or claim-token grant, narrowed
// by deny rules, token scopes and any custody lapse).
//
// It exists so a surface can offer only the actions the server will accept: a
// translator sees no Approve button rather than a 403 after clicking it. It
// reports, and enforces nothing; every handler still gates itself.
//
// GET /:ws/:id/permissions
func (s *Server) HandleGetCallerPermissions(c echo.Context) error {
	perms, ok := c.Get("project_permissions").(platauth.Permission)
	if !ok {
		// Fail closed, as requirePermission does: an unresolved context is a
		// caller with nothing, not a caller with everything.
		return c.JSON(http.StatusOK, CallerPermissionsResponse{Permissions: []string{}, Languages: []string{}})
	}
	languages, _ := c.Get("project_languages").([]string)
	if languages == nil {
		languages = []string{}
	}
	names := perms.Strings()
	if names == nil {
		names = []string{}
	}
	return c.JSON(http.StatusOK, CallerPermissionsResponse{Permissions: names, Languages: languages})
}
