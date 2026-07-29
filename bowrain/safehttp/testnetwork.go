package safehttp

import (
	"flag"
	"sync/atomic"
)

// The dilemma this solves: a policy that refuses loopback and private address
// space refuses every address a local test fixture could possibly bind to. So
// a test either reaches its fixture or exercises the real client — and the one
// that matters is the real client.
//
// The seam below replaces DNS resolution and the raw dial for policies built
// after it, and nothing else. Every scheme and address check stays in the
// path: a test names its fixture through a public hostname, the policy vets
// that hostname's (documentation-range) answer exactly as it would in
// production, and only the final connect is redirected to the listener. A test
// that is refused has been refused by the shipping code.
//
// It cannot be reached from a production process — see [InstallTestNetwork].

// testOptions holds the options appended to every policy built by [NewPolicy].
// It is nil in any normal process.
var testOptions atomic.Pointer[[]Option]

func loadTestOptions() []Option {
	if p := testOptions.Load(); p != nil {
		return *p
	}
	return nil
}

// InstallTestNetwork appends opts to every policy built by [NewPolicy] from
// here on, and returns the function that removes them again. Prefer the
// wrapper in safehttp/safehttptest, which ties the removal to the test.
//
// It panics unless the running binary is a test binary, so no configuration
// mistake or injected call can relax the address policy in production. The
// check is the test.v flag, which only testing.Init registers.
func InstallTestNetwork(opts ...Option) (restore func()) {
	if flag.Lookup("test.v") == nil {
		panic("safehttp: InstallTestNetwork is available only under `go test`")
	}
	prev := testOptions.Load()
	testOptions.Store(&opts)
	return func() {
		testOptions.Store(prev)
	}
}
