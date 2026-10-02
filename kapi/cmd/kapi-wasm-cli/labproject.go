//go:build js && wasm

package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/terms"
)

// voiceProfile is the small, deterministic voice the lab's annotators match
// against, carrying its word rules the way a starter pack does.
// labInspectAnnotated matches them with profile.MatchCarriedTerms so the docs
// "Anatomy" explorer can show word-rule overlays without any network or store.
// Forbidden terms suggest a preferred replacement; the competitor term has no
// replacement. Kept tiny and stable so the rendered overlays are reproducible.
var voiceProfile = (&profile.VoiceProfile{
	ID:   "kapi-wasm-demo",
	Name: "Kapi Demo Brand",
}).Carry("pack kapi-wasm-demo", []profile.TermRule{
	{Term: "login", Replacement: "log in", Note: "use the verb form"},
	{Term: "utilize", Replacement: "use", Advisory: true},
	{Term: "Acme", Competitor: true},
})

// fixtureTerms is the terms bundle the lab project holds: a native bundle, the
// lossless form a project commits, so the browser parses the same
// serialization the docs teach.
//
//go:embed fixtures/terms.json
var fixtureTerms []byte

// labProjectDir is the lab project's root in the engine's file system. It sits
// beside the lab's traces, out of the way of the directories a page works in,
// so no command a reader types finds it by walking up from where it runs.
const labProjectDir = "/.lab/project"

// labRecipe is the lab project's recipe: English source, French target, which
// is what the fixture terms cover.
const labRecipe = `version: v1
name: kapi-lab
defaults:
  source_language: en
  target_languages: [fr]
`

// The lab project is the store the read-only annotators look terms up in
// (termOverlay). It is an ordinary project in the engine's file system, opened
// through the same App every command uses, and its terms are imported through
// the projector, so they are operations in the workspace log as they would be
// natively. It is created on first use rather than at boot: a page that never
// asks for a term overlay pays nothing for it.
var labTerms struct {
	mu sync.Mutex
	// db is the lab project's store once its terms are imported. A reset of
	// the page's directories (kapiReset) can close it, and a failed open
	// leaves it nil; either way the next overlay opens the project again.
	db *projectdb.DB
}

// labTermsStore returns the lab project's terms store, creating and seeding
// the project when it is not open.
func labTermsStore(ctx context.Context) (terms.Store, error) {
	labTerms.mu.Lock()
	defer labTerms.mu.Unlock()
	if labTerms.db == nil || labTerms.db.Raw() == nil {
		db, err := openLabProject(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "kapi: term overlay:", err)
			return nil, err
		}
		labTerms.db = db
	}
	return projector.TermsView(labTerms.db), nil
}

func openLabProject(ctx context.Context) (*projectdb.DB, error) {
	if err := os.MkdirAll(labProjectDir, 0o755); err != nil {
		return nil, fmt.Errorf("lab project: %w", err)
	}
	recipe := filepath.Join(labProjectDir, "kapi.yaml")
	if err := os.WriteFile(recipe, []byte(labRecipe), 0o644); err != nil {
		return nil, fmt.Errorf("lab project: %w", err)
	}
	p, err := app.Projector(ctx, labProjectDir)
	if err != nil {
		return nil, fmt.Errorf("lab project: %w", err)
	}
	if _, err := cli.ImportKTBFile(ctx, p.With(projector.Origin{By: "lab"}).Terms(), bytes.NewReader(fixtureTerms)); err != nil {
		return nil, fmt.Errorf("lab project: import terms: %w", err)
	}
	db, err := app.ProjectDB(ctx, labProjectDir)
	if err != nil {
		return nil, fmt.Errorf("lab project: %w", err)
	}
	return db, nil
}
