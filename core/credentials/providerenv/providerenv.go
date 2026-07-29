// Package providerenv names the environment variables that may carry an AI
// provider's API key, and strips them from an environment handed to a
// subprocess.
//
// It exists as its own package for one reason: the list has to be reachable
// from both sides of the module boundary. The host resolves credentials from
// these variables and scrubs them before launching a plugin
// (host/credentials, host/pluginhost); the plugin conformance harness
// (core/plugin/conformance) has to launch a plugin the same way, or "passes
// conformance" stops predicting "behaves under kapi" — and core may not import
// host. A second copy of the list would drift, and the failure mode of drift is
// a key kapi believes it removed.
//
// It deliberately depends on nothing but the standard library. Its parent,
// core/credentials, reaches the OS keychain; the conformance harness is
// compiled by out-of-tree plugin repositories, and a test harness has no
// business pulling in a keychain binding to learn five variable names.
package providerenv

import (
	"slices"
	"strings"
)

// Vars maps a provider id to the ordered list of environment variables that may
// carry its API key; the first non-empty variable wins. The names follow each
// provider's own SDKs and CLIs.
//
// Provider ids match the constants in providers/ai. `ollama` and `demo` have no
// entry on purpose — they never require a key. The map is the single source of
// truth for the environment fallback, kept self-contained to avoid an import
// cycle with the provider packages.
var Vars = map[string][]string{
	"anthropic":   {"ANTHROPIC_API_KEY"},
	"openai":      {"OPENAI_API_KEY"},
	"gemini":      {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	"azureopenai": {"AZURE_OPENAI_API_KEY"},
}

// Names returns every environment variable that may carry a provider API key,
// sorted. It is the denylist for anything that hands an environment to a
// subprocess kapi did not write.
func Names() []string {
	var out []string
	for _, names := range Vars {
		out = append(out, names...)
	}
	slices.Sort(out)
	return out
}

// Without returns env with every provider API-key variable removed.
//
// A plugin is a separate binary kapi launches, and it inherits the whole host
// environment — including ANTHROPIC_API_KEY and its siblings, which resolve
// from the environment by design. Installing a plugin is already a decision the
// user makes, and cosign verification is what makes it a safe one; handing
// every plugin the user's model credentials is a separate grant nobody asked
// for, and it is invisible.
//
// Removal is a denylist, not an allowlist. Plugins are local compute engines
// that legitimately need much of the environment — PATH, HOME, TMPDIR,
// XDG_CACHE_HOME, the KAPI_* variables the host injects, and the ONNX/model
// paths their own manifests declare. An allowlist would break them on the first
// variable nobody thought of, and would break quietly.
//
// A plugin that genuinely needs to reach a model provider should declare that
// in its manifest, next to `models`, so the grant is written down where the
// user installing it can see it — the same argument that put the exec-class
// decision in front of a person (AD-038).
func Without(env []string) []string {
	deny := Names()
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
