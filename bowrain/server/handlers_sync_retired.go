package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/apierror"
)

// MinPushProtocolVersion is the oldest kapi-bowrain whose push this server can
// serve. 1.2.0-rc27 is the first release built after the push moved to
// tree-fetch plus init/commit; rc26 and everything before it negotiates the
// diff server-side over an endpoint that no longer exists.
const MinPushProtocolVersion = "1.2.0-rc27"

// InstallMinPushClient is the command that resolves MinPushProtocolVersion.
// The channel is explicit because a bare plugin name resolves on stable, and
// the stable channel does not carry a 1.2.0 build until 1.2.0 ships.
const InstallMinPushClient = "kapi plugins install --channel beta bowrain@" + MinPushProtocolVersion

// HandleSyncPushDiffRetired answers the push endpoint this server retired.
//
// The push used to send its local hashes to /push/diff and let the server
// decide what was missing. It now fetches the tree, computes that answer
// locally, and calls /push/init and /push/commit, so nothing serves the old
// endpoint any more.
//
// Leaving the path unrouted is what makes an old client's failure unreadable:
// Echo answers 404, the plugin reports "push diff failed (HTTP 404)" against
// whichever file it happened to send first, and the operator sees a per-file
// error for a whole-client problem. A push that worked last week and 404s today
// reads as a missing document, not an old plugin. This route exists to say the
// true thing instead, and to say it on the first file rather than once per file.
//
// 426 is the accurate status: the request is well-formed and the caller is
// authorized, and what fails is the protocol it speaks.
//
// The sentence has to survive two renderings, because the clients that reach
// here are exactly the ones too old to have been built against this envelope.
// From rc13 the client decodes it and prints the message alone; 1.1.0, which is
// what the stable channel still resolves to, prints the whole body raw. So the
// message is one plain sentence that reads correctly quoted inside JSON, and
// the version and command appear in it rather than only in the detail fields.
func (s *Server) HandleSyncPushDiffRetired(c echo.Context) error {
	return apiErr(c, http.StatusUpgradeRequired, apierror.CodeClientTooOld, map[string]any{
		"minimum_version": MinPushProtocolVersion,
		"install":         InstallMinPushClient,
	})
}
