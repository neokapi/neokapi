package server

// The stream-scoped endpoints that shipped bowrain clients call.
//
// A released plugin is out of our hands: whoever installed it keeps calling
// what it was built to call, for as long as they keep it. So the server's
// obligation is not "serve the current protocol" but "answer every endpoint any
// shipped client reaches for", either by serving it or by saying plainly that
// the client is too old (HandleSyncPushDiffRetired).
//
// Deleting a route alongside the client change that stopped using it is the
// move this table exists to catch. It reads as a clean removal, compiles, and
// passes every test written from the current client, because the callers it
// breaks are outside the repo. /push/diff went that way in #2136: the route and
// its caller left in one commit, and every plugin older than 1.2.0-rc27 began
// answering "push diff failed (HTTP 404)" against whichever file it happened to
// send first. The nightly dogfood said so for six days.
//
// Generated from the client package at every `v*` and `bowrain-v*` tag, by
// matching the streamPrefix() concatenations. Add a row when a client starts
// calling something new; never delete one, because the client that calls it
// still exists.
type streamEndpoint struct {
	Method string
	// Path is the Echo pattern, so the parameterised chunk path appears here
	// as it is registered rather than as the client's Sprintf form.
	Path string
	// Clients names the released versions that call it, in the plugin's own
	// version numbers.
	Clients string
}

var shippedStreamEndpoints = []streamEndpoint{
	{"GET", "/pull", "every release"},
	{"GET", "/blocks", "every release"},
	{"GET", "/status", "every release"},
	{"GET", "/ref", "1.2.0-rc23 and newer"},
	{"GET", "/tree", "1.2.0-rc27 and newer"},

	{"POST", "/push/init", "every release"},
	{"POST", "/push/commit", "every release"},
	{"POST", "/push/uploads", "1.2.0-rc27 and newer"},
	{"PUT", "/push/chunks/:uploadId/:chunkIndex", "every release"},

	// Retired in #2136, still answered. 1.2.0-rc26 and everything before it
	// negotiates the diff server-side, which is every build the stable channel
	// resolves to until 1.2.0 ships.
	{"POST", "/push/diff", "1.2.0-rc26 and older"},
}
