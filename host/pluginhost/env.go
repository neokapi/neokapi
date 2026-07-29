package pluginhost

import (
	"os"
	"strings"

	"github.com/neokapi/neokapi/host/credentials"
)

// WithoutProviderKeys returns env with every provider API-key variable removed.
//
// A plugin is a separate binary kapi launches, and it inherited the whole host
// environment — including ANTHROPIC_API_KEY and its siblings, which resolve
// from the environment by design (host/credentials). Installing a plugin is
// already a decision the user makes, and cosign verification is what makes it
// a safe one; handing every plugin the user's model credentials is a separate
// grant nobody asked for, and it is invisible.
//
// Removal is a denylist, not an allowlist. Plugins are local compute engines
// that legitimately need much of the environment — PATH, HOME, TMPDIR,
// XDG_CACHE_HOME, the KAPI_* variables the host injects, and the ONNX/model
// paths their own manifests declare. An allowlist would break them on the
// first variable nobody thought of, and would break quietly.
//
// No in-tree plugin reads a provider key: none of plugins/{asr,av,check,sat,
// vision,pdfium} nor bowrain/plugin references one, and none imports
// providers/ai at all. If one ever needs to reach a model provider it should
// declare that in its manifest, next to `models`, so the grant is written down
// where the user installing it can see it — the same argument that put the
// exec-class decision in front of a person (AD-038).
func WithoutProviderKeys(env []string) []string {
	deny := credentials.ProviderKeyEnvNames()
	if len(deny) == 0 {
		return env
	}
	denied := make(map[string]bool, len(deny))
	for _, name := range deny {
		denied[name] = true
	}

	out := make([]string, 0, len(env))
	for _, kv := range env {
		// An entry with no "=" is not a variable assignment; keep it rather
		// than guessing at it.
		name, _, ok := strings.Cut(kv, "=")
		if ok && denied[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// pluginEnviron is the host environment a plugin subprocess starts from: the
// parent's, less the provider keys. Every place that spawns a plugin builds on
// this rather than on os.Environ, so a new spawn site cannot reintroduce the
// leak by writing the obvious thing.
func pluginEnviron() []string {
	return WithoutProviderKeys(os.Environ())
}
