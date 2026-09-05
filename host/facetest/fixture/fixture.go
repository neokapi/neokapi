// Package fixture is the face parity conformance project and the record every
// face is held to, written with no dependency on the host layer.
//
// kapi answers the same questions at three local faces (CLI, MCP, desktop) and
// the platform answers some of them from its own graph. The faces' suites live
// in four modules, and the platform's cannot link the host layer the local ones
// answer through, so what travels between all of them is this package: one
// fixture written from one description, one set of answers embedded as JSON,
// and the extraction the framework performs over the fixture's content. The
// host layer's own projections of its answer types live in the parent package,
// host/facetest, which the three local suites use.
package fixture

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/require"
)

// Project is the conformance fixture on disk, with the inputs of every question
// the faces are asked about it.
type Project struct {
	// Root is the project directory.
	Root string
	// Recipe is the path of the project's recipe.
	Recipe string
	// ContextPath is the file `context <path>` is asked about, relative to Root.
	ContextPath string
	// SearchQuery is the query `context search` is asked.
	SearchQuery string
	// CheckPath is the file `check` is run on, relative to Root.
	CheckPath string
	// SearchLimit and ContextLimit pin the result sizes, so a face that
	// defaults differently is a difference the contract sees.
	ContextLimit int
	SearchLimit  int
}

// Abs resolves a fixture-relative path.
func (p Project) Abs(rel string) string {
	return filepath.Join(p.Root, filepath.FromSlash(rel))
}

// Write builds the fixture in a fresh directory, seeds its terms store and
// returns it.
//
// Everything is real and on disk: a recipe with two collections at two points,
// a voice profile, a terms file with a preferred and a deprecated spelling, a
// source catalog with a partial target beside it, and a document that violates
// the vocabulary. No face is given a fake source. The content is written and
// not extracted: host/facetest.Write extracts it into the project store through
// the host path a run takes, and Extract runs the framework's extraction for a
// suite that holds a store of its own.
//
// Isolation is by working directory rather than by opting out of discovery.
// A face that takes no project path finds one by walking up from the cwd (the
// MCP resources and the CLI verbs both do), so the fixture becomes the cwd, and
// the walk from a temporary directory reaches this repo's dogfood recipe never.
// KAPI_NO_PROJECT is pinned empty rather than left to the caller, because a
// suite that inherited it would disable the discovery every face depends on and
// then compare three answers about no project at all.
func Write(t *testing.T) Project {
	t.Helper()
	isolate(t)

	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	write("kapi.yaml", `version: v1
name: Fjordline
defaults:
  source_language: en
  target_languages: [nb]
  source_gate: none
  voice: .kapi/voice.yaml
  terms_source: .kapi/terms.json
profiles:
  support:
    channels: [docs]
collections:
  - name: Docs
    channel: support/docs
    content:
      - path: "docs/**/*.md"
  - name: App
    content:
      - path: app/en.json
        target: app/{lang}.json
`)

	// forbidden_terms, not preferred_terms: the vocabulary gate is built from
	// the forbidden and competitor sets (profile.VocabularyRuleSets), and a rule
	// under preferred_terms reaches the guide and the prompt without ever
	// reaching a check.
	write(".kapi/voice.yaml", `name: Fjordline
description: Plain, exact writing for people in a hurry.
tone:
  personality: [clear, restrained]
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: translation memory
      replacement: content memory
      severity: major
`)

	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-memory",
      "definition": "The store of approved prior wording.",
      "terms": [
        {"text": "content memory", "locale": "en", "status": "preferred"},
        {"text": "translation memory", "locale": "en", "status": "deprecated"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    },
    {
      "id": "c-berth",
      "definition": "The place a vessel ties up.",
      "terms": [
        {"text": "berth", "locale": "en", "status": "preferred"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)

	write("docs/guide.md", "# Guide\n\nReuse comes from the content memory.\n")
	write("docs/legacy.md", "# Legacy\n\nWe recycle from the translation memory.\n")
	write("app/en.json", `{"berth":"Berth","depart":"Departing","arrive":"Arriving"}`)
	// One of three strings carried into nb, so coverage is a number a face can
	// get wrong rather than 0 or 100.
	write("app/nb.json", `{"berth":"Kai"}`)

	p := Project{
		Root:         root,
		Recipe:       filepath.Join(root, "kapi.yaml"),
		ContextPath:  "docs/guide.md",
		SearchQuery:  "memory",
		CheckPath:    "docs/legacy.md",
		ContextLimit: 10,
		SearchLimit:  10,
	}
	seedTerms(t, p)
	t.Chdir(root)
	return p
}

// WritePosix builds a project declaring its locales in POSIX style and returns
// it, for the faces to be asked the same questions about.
//
// The recipe is the one place a locale is written by hand, so it is the one
// place a style other than BCP-47 legitimately appears. What the faces answer
// about it must not depend on how it was spelled: a project declaring en_US and
// nb_NO is the same project as one declaring en-US and nb-NO, and every face
// says so.
func WritePosix(t *testing.T) Project {
	t.Helper()
	isolate(t)

	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	write("kapi.yaml", `version: v1
name: Posixline
defaults:
  source_language: en_US
  target_languages: [nb_NO]
  source_gate: none
collections:
  - name: App
    content:
      - path: app/en.json
        target: app/{lang}.json
`)
	write("app/en.json", `{"berth":"Berth","depart":"Departing","arrive":"Arriving"}`)
	// The target sits at the canonical path, because {lang} renders the locale
	// the recipe declared and the recipe declares no locale_format.
	write("app/nb_NO.json", `{"berth":"Kai"}`)

	p := Project{
		Root:         root,
		Recipe:       filepath.Join(root, "kapi.yaml"),
		ContextPath:  "app/en.json",
		ContextLimit: 10,
		SearchLimit:  10,
	}
	t.Chdir(root)
	return p
}

// isolate points every kapi location at throwaway directories and re-enables
// project discovery, so a face resolves the fixture and nothing else.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_NO_PROJECT", "")
}

// PosixSourceLocale and PosixTargetLocale are what every face must answer about
// the WritePosix project, whatever it was asked.
const (
	PosixSourceLocale = model.LocaleID("en-US")
	PosixTargetLocale = model.LocaleID("nb-NO")
)

// Concepts is the fixture's vocabulary as its terms store holds it: the same
// concepts the terms file declares, for a suite that seeds a store of its own
// (the platform's) with what the local faces were given.
func Concepts() []terms.Concept {
	return []terms.Concept{
		{
			ID:         "c-memory",
			Definition: "The store of approved prior wording.",
			Terms: []terms.Term{
				{Text: "content memory", Locale: "en", Status: model.TermPreferred},
				{Text: "translation memory", Locale: "en", Status: model.TermDeprecated},
			},
		},
		{
			ID:         "c-berth",
			Definition: "The place a vessel ties up.",
			Terms: []terms.Term{
				{Text: "berth", Locale: "en", Status: model.TermPreferred},
			},
		},
	}
}

// seedTerms writes the fixture's concepts into the project's own terms store.
//
// The two halves of retrieval read different sources: by-location resolves the
// recipe's declared terms file, and by-content searches the store a project
// accumulates. A fixture that declared terms only in the file would leave every
// face's search empty, and a contract satisfied by three empty answers proves
// nothing.
func seedTerms(t *testing.T, p Project) {
	t.Helper()
	ctx := context.Background()
	layout := project.LayoutAt(p.Root)
	require.NoError(t, project.EnsureLayout(layout))
	db, err := projectdb.Open(ctx, layout)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	for _, c := range Concepts() {
		require.NoError(t, db.Terms().AddConcept(ctx, c))
	}
}

// ExtractedBlock is one block the framework's extraction of the fixture
// produces, with the collection and document it was read from.
type ExtractedBlock struct {
	Collection string
	Document   string
	Block      *blockstore.Block
}

// Extract runs the framework's extraction over the fixture's content and returns
// the blocks it produces, in collection then store order. It is the same
// extraction the local faces' store receives (project.ExtractToBlockStore, over
// the built-in formats), run into a store of its own, so a suite holding a
// different store, the platform's, can be given exactly the blocks the local
// faces counted.
func Extract(t *testing.T, p Project) []ExtractedBlock {
	t.Helper()
	ctx := context.Background()
	proj, err := project.Load(p.Recipe)
	require.NoError(t, err)
	pctx := project.NewProjectContext(proj, p.Recipe)
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	resolved, err := pctx.ResolveContent(reg)
	require.NoError(t, err)

	store := blockstore.NewMemoryStore()
	t.Cleanup(func() { _ = store.Close() })
	stats, err := project.ExtractToBlockStore(ctx, reg, pctx, store, noStamps{}, resolved)
	require.NoError(t, err)
	require.Positive(t, stats.Blocks, "the fixture has content to extract")

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()
	var out []ExtractedBlock
	for _, c := range proj.Collections {
		for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: project.CollectionLabel(c.Name)}) {
			require.NoError(t, berr)
			out = append(out, ExtractedBlock{Collection: c.Name, Document: b.Properties.File, Block: b})
		}
	}
	require.Len(t, out, stats.Blocks, "every extracted block sits in a declared collection")
	return out
}

// noStamps is the stamper for a store nobody will check for drift: the blocks
// are read back once and the store discarded.
type noStamps struct{}

func (noStamps) StampBlockStoreVersion(context.Context) error { return nil }
func (noStamps) SaveSourceStamps(context.Context, map[string]project.SourceStamp) error {
	return nil
}
