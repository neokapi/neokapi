package safehttp

import (
	"flag"
	"sync/atomic"
)

// Test networking replaces DNS resolution and dialing for policies created
// after installation. Tests address fixtures through public hostnames that
// resolve to documentation-range addresses. Production scheme and address
// checks still run; only the final connection is redirected to the fixture.
// InstallTestNetwork rejects use outside a test process.

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
