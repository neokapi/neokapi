//go:build js && wasm

package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
)

// brandProfile is the small, deterministic brand voice profile seeded for the
// browser build. labInspectAnnotated runs brand.MatchVocabulary against it so
// the docs "Anatomy" explorer can show brand-vocabulary overlays without any
// network or store. Forbidden terms suggest a preferred replacement; the
// competitor term has no replacement. Kept tiny and stable so the rendered
// overlays are reproducible.
var brandProfile = &brand.VoiceProfile{
	ID:   "kapi-wasm-demo",
	Name: "Kapi Demo Brand",
	Vocabulary: brand.VocabularyRules{
		ForbiddenTerms: []brand.TermRule{
			{Term: "login", Replacement: "log in", Note: "use the verb form", Severity: "major"},
			{Term: "utilize", Replacement: "use", Severity: "minor"},
		},
		CompetitorTerms: []brand.TermRule{
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
}
