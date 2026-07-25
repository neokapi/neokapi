//go:build js && wasm

package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
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

//go:embed fixtures/project.tmx
var fixtureTMX []byte

//go:embed fixtures/glossary.csv
var fixtureGlossaryCSV []byte

// seedBackends initialises the in-memory content memory and terms on app and
// assigns them to app.MemoryBackend / app.TermsBackend so that the tm,
// terms, term-check, and extract commands work in the browser build
// without cgo / SQLite.
func seedBackends() {
	tm := memory.NewInMemoryStore()
	opts := memory.ImportTMXOptions{
		OriginKey:     "fixture/project.tmx",
		OriginAddedBy: "kapi-wasm-cli",
		WarnFunc: func(msg string) {
			fmt.Fprintln(os.Stderr, "warning:", msg)
		},
	}
	if _, err := memory.ImportTMXWithOptions(context.Background(), tm, bytes.NewReader(fixtureTMX), model.LocaleID("en"), model.LocaleID("fr"), opts); err != nil {
		fmt.Fprintln(os.Stderr, "wasm: seed content memory:", err)
	}
	app.MemoryBackend = tm

	tb := terms.NewInMemoryStore()
	csvOpts := terms.CSVImportOptions{
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
		HasHeader:    true,
	}
	if _, err := terms.ImportCSV(context.Background(), tb, bytes.NewReader(fixtureGlossaryCSV), csvOpts); err != nil {
		fmt.Fprintln(os.Stderr, "wasm: seed terms:", err)
	}
	app.TermsBackend = tb
}
