package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/neokapi/neokapi/core/formats" // register markdown
)

// Journey 1, continued — the quality of what the front door says. The verbs run
// (see monolingual_test.go); these hold what they report: the gate matches whole
// words and prefers the longest term the project declared, a decision handed to
// `kapi apply` reaches the finding it answers, the vocabulary in force is
// resolved where the file sits, and a location reads the same in every mode.

// governedProject writes a project with two points and a vocabulary decision at
// each: the default profile retires "mooring" in favour of "berth" and declares
// the API field "mooring_id" a concept of its own, while a second profile
// governs the reference surface with its own terms store admitting "mooring"
// outright.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func governedProject(t *testing.T) (recipe, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	write := func(rel, body string) {
		path := filepath.Join(real, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	write("kapi.yaml", `version: v1
name: northsea
defaults:
  source_language: en-GB
  voice: .kapi/voice.yaml
  terms_source: .kapi/terms.json
  source_gate: none
profiles:
  northsea:
    channels: [docs, reference]
  northsea-record:
    channels: [reference]
    termstore: .kapi/profiles/northsea-record/terms.json
collections:
  - name: northsea-docs
    channel: northsea/docs
    content:
      - path: "docs/api/*.md"
        channel: northsea-record/reference
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", `name: North Sea
description: Plain, exact writing for harbour operations.
tone:
  personality: [clear, restrained]
  formality: neutral
`)
	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-berth",
      "definition": "The place a vessel is secured alongside.",
      "domain": "operations",
      "terms": [
        {"text": "berth", "locale": "en-GB", "status": "preferred"},
        {"text": "mooring", "locale": "en-GB", "status": "deprecated"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    },
    {
      "id": "c-mooring-id",
      "definition": "The API field that identifies a berth.",
      "domain": "api",
      "terms": [
        {"text": "mooring_id", "locale": "en-GB", "status": "admitted"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)
	// The reference surface's own vocabulary: the retired name is admitted here,
	// because the published contract keeps it.
	write(".kapi/profiles/northsea-record/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-berth-record",
      "definition": "The place a vessel is secured alongside, as the contract names it.",
      "domain": "operations",
      "terms": [
        {"text": "mooring", "locale": "en-GB", "status": "admitted"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)
	write("docs/berths.md", `# Berths

Release a mooring earlier than the plan allows.
`)
	write("docs/api/reference.md", `# API reference

The mooring_id field names the place alongside, and every mooring keeps it.
`)

	return filepath.Join(real, project.RecipeFileName), real
}

// upThenCheck converges the project and returns the check output.
func upThenCheck(t *testing.T, recipe string, checkArgs ...string) string {
	t.Helper()
	a := processOnlyApp(t)
	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	out, cerr := runCLI(t, NewCheckCmd(a), append([]string{"--project", recipe, "--no-fail"}, checkArgs...)...)
	require.NoError(t, cerr, out)
	return out
}

// TestCheck_LongerDeclaredTermWins is #1914: the terms half of the gate matched
// substrings, so `mooring_id` — a concept the project separately declares — was
// reported as the retired `mooring` inside it, twice per paragraph, while the
// voice-profile half of the same gate matched whole words.
func TestCheck_LongerDeclaredTermWins(t *testing.T) {
	recipe, _ := governedProject(t)
	out := upThenCheck(t, recipe)

	assert.Contains(t, out, "berths.md", "a bare use of the retired term is still a finding")
	assert.NotContains(t, out, "reference.md",
		"the reference surface's own vocabulary admits the retired name, and mooring_id is a term of its own")
}

// TestCheck_VocabularyIsResolvedWhereTheFileSits: the voice half of the gate is
// resolved per file, at the point that file sits at. The terms half consulted a
// single project-wide store, so a profile that bound its own vocabulary — the
// declarable, recipe-level way to say "these files keep the old name" — loaded
// cleanly and changed not one finding.
func TestCheck_VocabularyIsResolvedWhereTheFileSits(t *testing.T) {
	recipe, root := governedProject(t)

	// Same content, same word, two points: the reference surface admits the
	// retired name; the documentation surface does not.
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "api", "reference.md"),
		[]byte("# API reference\n\nEvery mooring keeps its name here.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "berths.md"),
		[]byte("# Berths\n\nEvery mooring keeps its name here.\n"), 0o644))

	out := upThenCheck(t, recipe)

	assert.Contains(t, out, "berths.md", "the documentation surface retired the name")
	assert.NotContains(t, out, "reference.md", "the reference surface's profile admits it")
}

// TestApplyThenCheck_ReplacementReachesTheFinding is #1915, end to end: a
// decision handed to `kapi apply` with a replacement produced a finding that
// named no alternative, because the concept apply created held the term alone
// and the replacement sat in its note. The correction loop's most important beat
// landed without its fix.
func TestApplyThenCheck_ReplacementReachesTheFinding(t *testing.T) {
	recipe, root := governedProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "berths.md"),
		[]byte("# Berths\n\nThe vessel could not dock when the pilot called.\n"), 0o644))

	decisions := filepath.Join(root, "decisions.jsonl")
	require.NoError(t, os.WriteFile(decisions,
		[]byte(`{"kind":"term","op":"upsert","term":"dock","locale":"en-GB","status":"forbidden","replacement":"berth"}`+"\n"), 0o644))

	a := processOnlyApp(t)
	applyOut, err := runCLI(t, NewApplyCmd(a), decisions, "--project", recipe)
	require.NoError(t, err, applyOut)
	assert.Contains(t, applyOut, "applied")

	out := upThenCheck(t, recipe)
	assert.Contains(t, out, `Forbidden term "dock"`)
	assert.Contains(t, out, `Use "berth" instead`,
		"the replacement the decision carried must reach the finding it answers")
}

// TestCheckShip_LocationsAreProjectRelative is #1906 item 5: the ship gate
// printed absolute paths and no block where `kapi check` printed a
// project-relative path with one. Besides the inconsistency, an absolute path
// names a machine rather than a repository — unusable in a CI log or a
// recording, and it leaks the directory layout.
func TestCheckShip_LocationsAreProjectRelative(t *testing.T) {
	recipe, root := governedProject(t)

	a := processOnlyApp(t)
	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	out, cerr := runCLI(t, NewCheckCmd(a), "--project", recipe, "--ship", "--no-fail")
	require.NoError(t, cerr, out)

	require.Contains(t, out, "mooring", "the ship run reports the vocabulary finding")
	assert.NotContains(t, out, root, "a finding location must not carry the machine's directory layout")
	assert.Contains(t, out, filepath.ToSlash(filepath.Join("docs", "berths.md"))+":",
		"the location is the project-relative path and the block, as `kapi check` prints it")
}

// TestCheckShip_GateSelection is #1906 item 2: `--terms` and `--qa` were read by
// gate selection and never registered, and `--voice` was double-booked as both
// the style-similarity opt-in and a gate selector. One flag names gates now, and
// an unknown name is refused with the list.
func TestCheckShip_GateSelection(t *testing.T) {
	recipe, _ := governedProject(t)

	a := processOnlyApp(t)
	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	out, cerr := runCLI(t, NewCheckCmd(a), "--project", recipe, "--ship", "--gate", "voice", "--no-fail")
	require.NoError(t, cerr, out)
	assert.Contains(t, out, "voice")
	assert.NotContains(t, out, "qa", "only the named gate ran")

	_, gerr := runCLI(t, NewCheckCmd(a), "--project", recipe, "--ship", "--gate", "terms", "--no-fail")
	require.Error(t, gerr, "a gate name that is not a gate must be refused")
	assert.Contains(t, gerr.Error(), "terminology", "the error names the gates that exist")
}

// TestCheck_ProjectFlagTakesADirectory is #1906 item 1: the flag help offers "or
// its directory" and the resolver returned the flag verbatim, so every command
// given a directory failed on "read project file: … is a directory".
func TestCheck_ProjectFlagTakesADirectory(t *testing.T) {
	recipe, root := governedProject(t)

	a := processOnlyApp(t)
	byDir, err := runCLI(t, NewCheckCmd(a), "--project", root, "--no-fail")
	require.NoError(t, err, byDir)

	byFile, err := runCLI(t, NewCheckCmd(a), "--project", recipe, "--no-fail")
	require.NoError(t, err, byFile)

	assert.Equal(t, strings.TrimSpace(byFile), strings.TrimSpace(byDir),
		"a directory and the recipe inside it name the same project")
}
