package pluginhost

import "fmt"

// Tombstone records a plugin kapi has retired. The set is compiled into the
// binary, which is itself the version gate: a kapi old enough to predate a
// retirement simply has no entry for it, so there is no runtime version
// comparison to get wrong (kapi's dev/build version strings don't sort
// reliably). RetiredIn is shown to the user for context, not compared.
//
// This built-in list is the offline-authoritative source for retirement: it
// works with no network and against a stale/pinned registry snapshot, and it
// still applies when the retired plugin's binary is sitting on disk from a prior
// install (the common case — a Homebrew/registry install kapi can't un-ship).
// The registry's `deprecated` field (see registry/index.go) augments this for
// install-time refusal and fresher messaging, but the built-in list wins for
// load-time enforcement.
type Tombstone struct {
	// Plugin is the retired plugin's name (manifest "plugin" field).
	Plugin string
	// RetiredIn is the kapi version that retired the plugin, shown in the notice.
	RetiredIn string
	// Because is the reason fragment, phrased to follow "Reason: ".
	Because string
	// Replacement is the successor plugin name, or "" when there is none.
	Replacement string
	// ReplacementMsg is free-text guidance toward the replacement path.
	ReplacementMsg string
	// InfoURL points at documentation about the retirement/migration.
	InfoURL string
}

// tombstones is the compiled-in retirement registry, keyed by plugin name.
var tombstones = map[string]Tombstone{
	"llm": {
		Plugin:         "llm",
		RetiredIn:      "1.2.0",
		Because:        "the bundled on-device Gemma engine is superseded by the built-in Ollama provider (GPU-accelerated, no cgo/onnxruntime stack)",
		ReplacementMsg: "run local models with Ollama: `kapi models ollama install`, then `kapi translate --provider ollama`",
		InfoURL:        "https://neokapi.github.io/kapi/framework/ai-translation",
	},
}

// LookupTombstone returns the retirement record for a plugin name, if it has
// been retired.
func LookupTombstone(name string) (Tombstone, bool) {
	t, ok := tombstones[name]
	return t, ok
}

// Notice renders the multi-line retirement message shown in `kapi plugins list`,
// `doctor`, and `prune`.
func (t Tombstone) Notice() string {
	msg := fmt.Sprintf("plugin %q was retired in kapi %s — it is installed but no longer loaded.\n  Reason: %s.", t.Plugin, t.RetiredIn, t.Because)
	if t.ReplacementMsg != "" {
		msg += "\n  Replacement: " + t.ReplacementMsg
	}
	if t.InfoURL != "" {
		msg += "\n  More: " + t.InfoURL
	}
	msg += "\n  Remove it with: kapi plugins prune"
	return msg
}
