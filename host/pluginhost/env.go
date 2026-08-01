package pluginhost

import (
	"os"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
)

// WithoutProviderKeys returns env with every provider API-key variable removed.
// See core/credentials/providerenv for why the rule and the list live there:
// the plugin conformance harness has to launch a plugin exactly the way this
// package does, and it cannot import host.
//
// No in-tree plugin reads a provider key: none of plugins/{asr,av,check,sat,
// vision,pdfium} nor bowrain/plugin references one, and none imports
// providers/ai at all.
func WithoutProviderKeys(env []string) []string {
	return providerenv.Without(env)
}

// pluginEnviron is the host environment a plugin subprocess starts from: the
// parent's, less the provider keys. Every place that spawns a plugin builds on
// this rather than on os.Environ, so a new spawn site cannot reintroduce the
// leak by writing the obvious thing.
func pluginEnviron() []string {
	return WithoutProviderKeys(os.Environ())
}
