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
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
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

// seedBackends initialises the in-memory TM and termbase on app and
// assigns them to app.TMBackend / app.TBBackend so that the tm,
// termbase, term-check, and extract commands work in the browser build
// without cgo / SQLite.
func seedBackends() {
	tm := sievepen.NewInMemoryTM()
	opts := sievepen.ImportTMXOptions{
		OriginKey:     "fixture/project.tmx",
		OriginAddedBy: "kapi-wasm-cli",
		WarnFunc: func(msg string) {
			fmt.Fprintln(os.Stderr, "warning:", msg)
		},
	}
	if _, err := sievepen.ImportTMXWithOptions(context.Background(), tm, bytes.NewReader(fixtureTMX), model.LocaleID("en"), model.LocaleID("fr"), opts); err != nil {
		fmt.Fprintln(os.Stderr, "wasm: seed TM:", err)
	}
	app.TMBackend = tm

	tb := termbase.NewInMemoryTermBase()
	csvOpts := termbase.CSVImportOptions{
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
		HasHeader:    true,
	}
	if _, err := termbase.ImportCSV(context.Background(), tb, bytes.NewReader(fixtureGlossaryCSV), csvOpts); err != nil {
		fmt.Fprintln(os.Stderr, "wasm: seed termbase:", err)
	}
	app.TBBackend = tb
}
