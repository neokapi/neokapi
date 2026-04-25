package server

import platauth "github.com/neokapi/neokapi/bowrain/core/auth"

// Slug validation lives in bowrain/core/auth so it can be used from the
// service layer without importing the server package. These aliases keep
// existing handler call-sites compiling.
var (
	ValidateSlug          = platauth.ValidateSlug
	ValidateWorkspaceSlug = platauth.ValidateWorkspaceSlug
	ValidateProjectSlug   = platauth.ValidateProjectSlug
)
