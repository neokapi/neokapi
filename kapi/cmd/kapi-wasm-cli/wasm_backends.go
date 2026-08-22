//go:build js && wasm

package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
)

// voiceProfile is the small, deterministic voice profile seeded for the
// browser build. labInspectAnnotated runs profile.MatchVocabulary against it so
// the docs "Anatomy" explorer can show voice-vocabulary overlays without any
// network or store. Forbidden terms suggest a preferred replacement; the
// competitor term has no replacement. Kept tiny and stable so the rendered
// overlays are reproducible.
var voiceProfile = &profile.VoiceProfile{
	ID:   "kapi-wasm-demo",
	Name: "Kapi Demo Brand",
	Vocabulary: profile.VocabularyRules{
		ForbiddenTerms: []profile.TermRule{
			{Term: "login", Replacement: "log in", Note: "use the verb form", Severity: "major"},
			{Term: "utilize", Replacement: "use", Severity: "minor"},
		},
		CompetitorTerms: []profile.TermRule{
			{Term: "Acme", Severity: "critical"},
		},
	},
}

// The seeds are native bundles, not TMX/CSV. The interchange tier is for
// crossing a boundary into or out of kapi; seeding kapi's own demo backends is
// not that boundary, and a lossless bundle is what a project actually commits.
// It also means the browser build parses the same serialization the docs teach.

//go:embed fixtures/memory.json
var fixtureMemory []byte

//go:embed fixtures/terms.json
var fixtureTerms []byte

// seedBackends initialises the in-memory content memory and terms on app and
// assigns them to app.MemoryBackend / app.TermsBackend so that the memory,
// terms, term-check, and extract commands work in the browser build
// without cgo / SQLite.
func seedBackends() {
	ctx := context.Background()

	tm := memory.NewInMemoryStore()
	if file, err := kmb.Decode(bytes.NewReader(fixtureMemory)); err != nil {
		fmt.Fprintln(os.Stderr, "wasm: parse content memory bundle:", err)
	} else {
		for _, e := range file.ModelEntries() {
			if err := tm.AddWithStream(ctx, e, ""); err != nil {
				fmt.Fprintln(os.Stderr, "wasm: seed content memory:", err)
			}
		}
	}
	app.MemoryBackend = tm

	tb := terms.NewInMemoryStore()
	if _, err := cli.ImportKTBFile(ctx, tb, bytes.NewReader(fixtureTerms)); err != nil {
		fmt.Fprintln(os.Stderr, "wasm: seed terms:", err)
	}
	app.TermsBackend = tb

	// The block cache, which has nothing to seed: it is filled by whatever the
	// lab extracts in this session. A process-lifetime store is what makes that
	// carry between commands, so `extract` then `terms occurrences` answers in
	// the browser exactly as it does on a desktop — with a linear scan instead
	// of an FTS index, which at one lab document is the cheaper of the two
	// anyway.
	app.BlocksBackend = blockstore.NewPersistentMemoryStore()
}
