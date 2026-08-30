// Package facetest carries the conformance fixture and the answer shapes the
// kapi faces are held to.
//
// kapi answers the same questions at three faces: a CLI verb, an MCP tool or
// resource, and the desktop's backend method. They read one graph through one
// host layer, and the promise is that a person at a terminal, an agent over
// MCP, and someone in the app cannot learn three different models of a project.
// Drift between the faces was the root of four of the five known loop defects,
// so parity is a test rather than a convention.
//
// The faces live in three Go modules, and the desktop's module pulls in Wails,
// so no single test binary can call all three. What travels between them
// instead is this package: one fixture written from one description, and one
// set of answers embedded as JSON. Each face's own suite builds the fixture,
// asks its own entry point, projects the reply into the shapes below, and
// compares against the same embedded answers. Agreeing with the record is
// transitively agreeing with each other.
//
// The shapes are projections, not the faces' own structs. Three of the four
// questions are answered by types that differ per face — the desktop counts
// coverage from the block store while `kapi status` counts from the working
// tree, and the desktop's findings carry a category where the report carries a
// rule — so the contract is the facts every face must agree on, and each
// suite's projection is where a face states what it believes it is saying.
package facetest

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
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

// Write builds the fixture in a fresh directory and returns it.
//
// Everything is real and on disk: a recipe with two collections at two points,
// a voice profile, a terms file with a preferred and a deprecated spelling, a
// source catalog with a partial target beside it, and a document that violates
// the vocabulary. No face is given a fake source.
//
// Isolation is by working directory rather than by opting out of discovery.
// A face that takes no project path finds one by walking up from the cwd — the
// MCP resources and the CLI verbs both do — so the fixture becomes the cwd, and
// the walk from a temporary directory reaches this repo's dogfood recipe never.
// KAPI_NO_PROJECT is pinned empty rather than left to the caller, because a
// suite that inherited it would disable the discovery every face depends on and
// then compare three answers about no project at all.
func Write(t *testing.T) Project {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_NO_PROJECT", "")

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
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_NO_PROJECT", "")

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

// PosixSourceLocale and PosixTargetLocale are what every face must answer about
// the WritePosix project, whatever it was asked.
const (
	PosixSourceLocale = model.LocaleID("en-US")
	PosixTargetLocale = model.LocaleID("nb-NO")
)

// seedTerms writes the fixture's concepts into the project's own terms store.
//
// The two halves of retrieval read different sources: by-location resolves the
// recipe's declared terms file, and by-content searches the store a project
// accumulates. A fixture that declared terms only in the file would leave every
// face's search empty, and a contract satisfied by three empty answers proves
// nothing.
func seedTerms(t *testing.T, p Project) {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	defer a.Shutdown()

	db, err := a.ProjectDB(t.Context(), p.Root)
	require.NoError(t, err)
	for _, c := range []terms.Concept{
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
	} {
		require.NoError(t, db.Terms().AddConcept(t.Context(), c))
	}
}

// Answers is the record every face is held to.
type Answers struct {
	ContextAt     ContextFacts `json:"context_at"`
	ContextSearch SearchFacts  `json:"context_search"`
	Status        StatusFacts  `json:"status"`
	Check         CheckFacts   `json:"check"`
}

// Findings is the recorded check findings, in their recorded order.
func (a Answers) Findings() []FindingFacts { return a.Check.Findings }

// PointFacts is where in the project a question landed.
type PointFacts struct {
	Path       string `json:"path"`
	Profile    string `json:"profile"`
	Channel    string `json:"channel"`
	Collection string `json:"collection"`
	Default    bool   `json:"default"`
}

// VoiceFacts is the voice in force at a point.
type VoiceFacts struct {
	Name  string `json:"name"`
	Field string `json:"field"`
}

// TermFacts is one term a face reported. Uses is deliberately absent: it is
// derived from occurrences a face may or may not have a store for, and holding
// three faces to a count computed two ways is what the occurrence issue tracks.
type TermFacts struct {
	ConceptID   string `json:"concept_id"`
	Term        string `json:"term"`
	Status      string `json:"status"`
	Discouraged bool   `json:"discouraged"`
	Replacement string `json:"replacement"`
}

// ContextFacts is the answer to "what applies here".
type ContextFacts struct {
	Point      PointFacts  `json:"point"`
	Voice      VoiceFacts  `json:"voice"`
	Terms      []TermFacts `json:"terms"`
	TermsTotal int         `json:"terms_total"`
}

// SearchFacts is the answer to "what do we know about this".
type SearchFacts struct {
	Query     string      `json:"query"`
	Scope     string      `json:"scope"`
	Terms     []TermFacts `json:"terms"`
	Precedent []string    `json:"precedent"`
}

// LocaleFacts is one locale's standing.
type LocaleFacts struct {
	Locale     string `json:"locale"`
	Translated int    `json:"translated_pct"`
}

// StatusFacts is the answer to "where does this project stand".
type StatusFacts struct {
	Project     string        `json:"project"`
	Collections []string      `json:"collections"`
	Locales     []LocaleFacts `json:"locales"`
}

// FindingFacts is one thing a check objected to.
type FindingFacts struct {
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// CheckFacts is what a check objected to.
//
// The verdict is deliberately absent. Each face gates on its own bar — the CLI
// takes it from flags, the MCP tools apply check.DefaultGate, and the desktop
// reports a score with no gate at all — so a contract that included pass/fail
// would fail on a difference the faces are entitled to have. What they may not
// disagree about is what is wrong with the content.
type CheckFacts struct {
	Findings []FindingFacts `json:"findings"`
}

// ContextFactsFrom projects the host's by-location answer.
//
// The CLI verb, the `context://` resource and the desktop's ContextAt all
// return this type unchanged, so for this question the projection is the whole
// comparison and a face that reshaped the answer would fail here.
func ContextFactsFrom(a *host.ContextAnswer) ContextFacts {
	f := ContextFacts{
		Point: PointFacts{
			Path:       filepath.ToSlash(a.Point.Path),
			Profile:    a.Point.Profile,
			Channel:    a.Point.Channel,
			Collection: a.Point.Collection,
			Default:    a.Point.Default,
		},
		TermsTotal: a.TermsTotal,
		Terms:      termFacts(a.Terms),
	}
	if a.Voice != nil {
		f.Voice = VoiceFacts{Name: a.Voice.Name, Field: a.Voice.Field}
	}
	return f
}

// SearchFactsFrom projects the host's by-content answer.
func SearchFactsFrom(r *host.ContextSearchResult) SearchFacts {
	f := SearchFacts{
		Query: r.Query,
		Scope: string(r.Scope),
		Terms: termFacts(r.Terms),
	}
	for _, p := range r.Precedent {
		f.Precedent = append(f.Precedent, p.Text)
	}
	sort.Strings(f.Precedent)
	return f
}

// CheckFactsFrom projects a check report.
//
// Severity, message and suggestion are the facts all three faces carry. The
// report's rule ids and gate live on two of them, and holding the third to a
// field it has no place to put would fail the contract for a shape difference
// rather than a disagreement.
func CheckFactsFrom(r check.Report) CheckFacts {
	var f CheckFacts
	for _, d := range r.Findings {
		f.Findings = append(f.Findings, FindingFacts{
			Severity:   string(d.Severity),
			Message:    d.Message,
			Suggestion: d.Suggestion,
		})
	}
	SortFindings(f.Findings)
	return f
}

// SortFindings orders findings so a face that collects them in a different
// order still agrees about what it found.
func SortFindings(f []FindingFacts) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].Message != f[j].Message {
			return f[i].Message < f[j].Message
		}
		return f[i].Severity < f[j].Severity
	})
}

// SortLocales orders locales by name.
func SortLocales(l []LocaleFacts) {
	sort.Slice(l, func(i, j int) bool { return l[i].Locale < l[j].Locale })
}

func termFacts(in []host.ContextTermHit) []TermFacts {
	var out []TermFacts
	for _, h := range in {
		out = append(out, TermFacts{
			ConceptID:   h.ConceptID,
			Term:        h.Term,
			Status:      h.Status,
			Discouraged: h.Discouraged,
			Replacement: h.Replacement,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConceptID != out[j].ConceptID {
			return out[i].ConceptID < out[j].ConceptID
		}
		return out[i].Term < out[j].Term
	})
	return out
}

//go:embed testdata/answers.json
var goldenAnswers []byte

// Golden is the recorded answer set, embedded so a suite in any module reads
// the same bytes without reaching across module boundaries for a testdata
// directory it cannot see.
func Golden(t *testing.T) Answers {
	t.Helper()
	var a Answers
	require.NoError(t, json.Unmarshal(goldenAnswers, &a), "decode the recorded answers")
	return a
}

// GoldenPath is where the recorded answers live, for the suite that rewrites
// them under -update.
func GoldenPath() string { return filepath.Join("testdata", "answers.json") }

// Marshal renders an answer set the way the record stores it.
func Marshal(t *testing.T, a Answers) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(a, "", "  ")
	require.NoError(t, err)
	return append(raw, '\n')
}
