package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/apierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postPushDiff sends what a pre-rc27 plugin sends: hashes for the server to
// diff. The body shape does not matter to the answer, because the endpoint
// itself identifies the protocol: nothing has to be parsed to know the client
// is too old.
func postPushDiff(t *testing.T, e http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"project_id":"p","stream":"main","item_hashes":{"docs/a.json":"deadbeef"}}`
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// The failure this replaces: an old plugin got Echo's bare 404 and reported
// "push diff failed (HTTP 404)" against whichever file it sent first, so a
// whole-client problem read as a missing document. It cost the nightly dogfood
// six days of failures that named a markdown file every time.
func TestRetiredPushDiff_TellsTheClientToUpgrade(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	rec := postPushDiff(t, e, "/api/v1/projects/"+pid+"/sync/main/push/diff", authHeader)

	require.Equal(t, http.StatusUpgradeRequired, rec.Code,
		"the request is well-formed and authorized; what fails is the protocol it speaks")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, apierror.CodeClientTooOld, body["error"])
	assert.Equal(t, MinPushProtocolVersion, body["minimum_version"])
	assert.Equal(t, InstallMinPushClient, body["install"])

	message, _ := body["message"].(string)
	assert.Contains(t, message, MinPushProtocolVersion,
		"the sentence names the version, so the fix does not require reading the detail fields")
	assert.Contains(t, message, InstallMinPushClient,
		"and it names the command, because the channel is the part a reader gets wrong")
}

// Both route groups serve the same client. A plugin configured with a
// workspace-scoped server URL must get the upgrade answer too, rather than the
// 404 that sent it looking at its own files.
func TestRetiredPushDiff_AnswersOnTheWorkspaceScopedRoute(t *testing.T) {
	srv, _ := newTestServer(t)
	e := srv.GetEcho()

	registered := map[string]bool{}
	for _, r := range e.Routes() {
		registered[r.Method+" "+r.Path] = true
	}

	assert.True(t, registered["POST /api/v1/projects/:id/sync/:ref/push/diff"],
		"the flat route answers the retired endpoint")
	assert.True(t, registered["POST /api/v1/:ws/:id/sync/:ref/push/diff"],
		"and so does the workspace-scoped one")
}

// The minimum is a fact about releases, not a preference: rc26 negotiates the
// diff server-side and rc27 is the first build that fetches the tree instead.
// Naming a version the client cannot get, or one that still speaks the old
// protocol, makes the message worse than the 404 it replaces.
func TestMinPushProtocolVersion_NamesAReleaseThatSpeaksTheProtocol(t *testing.T) {
	assert.Equal(t, "1.2.0-rc27", MinPushProtocolVersion)

	assert.Contains(t, InstallMinPushClient, "--channel beta",
		"a bare plugin name resolves on stable, which carries no 1.2.0 build until 1.2.0 ships")
	assert.Contains(t, InstallMinPushClient, "bowrain@"+MinPushProtocolVersion,
		"the command installs the version the message names")
}
