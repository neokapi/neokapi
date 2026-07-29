package connector

import (
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/bowrain/safehttp/safehttptest"
)

// testPrivateHost is an ordinary-looking name that resolves into RFC 1918
// space — the case only a check at connect time can catch.
const testPrivateHost = safehttptest.PrivateHost

// withTestNetwork points the connector policy at ts for the duration of the
// test and returns the base URL to configure a connector with. It replaces DNS
// and the raw dial only: every address check stays in the path, so a test that
// is refused has been refused by the shipping code.
func withTestNetwork(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	return safehttptest.Serve(t, ts)
}
