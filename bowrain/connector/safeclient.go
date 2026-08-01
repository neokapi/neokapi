package connector

import (
	"net/http"
	"time"

	"github.com/neokapi/neokapi/bowrain/resilience"
	"github.com/neokapi/neokapi/bowrain/safehttp"
)

// A remote connector's base URL is tenant input — saved through the workspace
// connectors API by anyone holding PermManageConnectors — and the server
// dials it from inside its own network. Without an address policy that makes
// a connector config a way to aim the server at a cloud metadata endpoint, an
// RFC 1918 neighbour, or a loopback service that trusts its caller, and to
// read back whatever answered. Every HTTP client in this package is therefore
// built from a [safehttp.Policy].
//
// The same reasoning applies to what comes *back*: an error from a host
// somebody else chose must not be reflected to the requester. A status code
// tells the operator their integration is failing; the upstream body would
// tell them what the server can reach.

// connectorTimeout bounds a single call to a remote integration.
const connectorTimeout = 30 * time.Second

// errorBodyLimit is how much of a failed response is drained before the
// connection is released. Never returned to a caller.
const errorBodyLimit = 8 << 10

// remotePolicy is the network policy every remote connector client is built
// from. Plain http is permitted alongside https because self-hosted
// integrations are not all on TLS; the address half is what stops the forgery,
// and it applies to both schemes.
func remotePolicy() *safehttp.Policy {
	return safehttp.NewPolicy(safehttp.WithSchemes("http", "https"))
}

// remoteClient returns the HTTP client for a remote connector: SSRF-safe
// dialing and redirect handling, wrapped in the named circuit breaker. The
// breaker is outermost so one logical call is one observation.
func remoteClient(name string) *http.Client {
	return resilience.Guard(remotePolicy().Client(connectorTimeout),
		name, resilience.KindConnector, connectorTimeout)
}
