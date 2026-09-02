package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/neokapi/neokapi/core/formats" // register markdown
)

// Journey 1 — the monolingual front door. A team with one language, a bound
// voice profile and a committed vocabulary drives discover → context → check →
// converge with no target language anywhere in the recipe. These tests hold the
// verb surface to that: `up` completes, `status` reports the one axis the
// project has, `check` enforces the vocabulary, and both retrieval primitives
// answer the same question the same way.

// monolingualProject writes the front-door project: markdown content in one
// language, a voice profile that forbids a word, a committed terms source that
// retires another, and no `target_languages` in defaults, on a collection or on
// an item.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func monolingualProject(t *testing.T) (recipe, root string) {
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
    channels: [docs]
collections:
  - name: northsea-docs
    channel: northsea/docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", `name: North Sea
description: Plain, exact writing for harbour operations.
tone:
  personality: [clear, restrained]
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: seamless
      replacement: uninterrupted
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
    }
  ]
}
`)
	write("docs/berths.md", `# Berths

Every mooring is allocated on arrival, and the handover is seamless.
`)

	return filepath.Join(real, project.RecipeFileName), real
}

// runCLI executes one cobra command and returns its combined output. The recipe
// is always named explicitly by the caller, because discovery is off under the
// isolation contract.
func runCLI(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// TestUp_MonolingualProjectConverges is the front-door criterion: `kapi up` on a
// project with no target language runs to completion. It seeds the committed
// context, extracts the working tree into the store and refreshes the occurrence
// graph; only the per-locale fan-out is skipped, because there is no locale to
// fan out to. The verb used to refuse to start, which left a one-language
// project with no way to compile its own sources into the store at all.
func TestUp_MonolingualProjectConverges(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := monolingualProject(t)

	out, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, out)

	assert.Contains(t, out, "No target languages are configured")
	assert.Contains(t, out, "Reconciled the source")
	assert.Contains(t, out, "Up to date")

	assert.FileExists(t, project.LayoutAt(root).StorePath(),
		"the run compiled the committed sources into the project store")
}

// TestUp_MonolingualNeedsNoAIProvider: a monolingual run makes no provider call,
// so it must not demand a provider. Nothing configures one here — the fixture's
// config dir is empty — and the run must still complete.
func TestUp_MonolingualNeedsNoAIProvider(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := monolingualProject(t)

	out, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, out)
	assert.NotContains(t, out, "provider")
}

// TestUpPlan_MonolingualWritesNothing: the plan is a dry run on every project
// shape. It must neither error on a monolingual recipe nor leave a store behind,
// and it must say what the run would do rather than "every unit has a committed
// target" — a sentence about units that do not exist.
func TestUpPlan_MonolingualWritesNothing(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := monolingualProject(t)

	out, err := runCLI(t, NewUpCmd(a), "--project", recipe, "--plan")
	require.NoError(t, err, out)

	assert.Contains(t, out, "No target languages configured")
	assert.NotContains(t, out, "every unit has a committed target")
	assert.NoFileExists(t, project.LayoutAt(root).StorePath(), "a dry run created a project store")
}

// TestStatus_MonolingualReportsTheSource: a monolingual project has real
// standing — how much source it tracks — and reported none of it, because source
// readiness was derived from the per-locale units a project with no locale never
// resolves.
func TestStatus_MonolingualReportsTheSource(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := monolingualProject(t)

	out, err := runCLI(t, NewStatusCmd(a), "--project", recipe)
	require.NoError(t, err, out)

	assert.Contains(t, out, "source", "the source-readiness line is the project's one axis")
	assert.Contains(t, out, "Monolingual project")
	assert.NotContains(t, out, "No target content tracked")
}

// TestCheck_MonolingualFiresTheVoiceViolation: journey 1 step 3 — the bound
// gates run on a project with no target language, and a forbidden word in the
// source is a finding.
func TestCheck_MonolingualFiresTheVoiceViolation(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := monolingualProject(t)

	out, err := runCLI(t, NewCheckCmd(a), "--project", recipe, "--no-fail")
	require.NoError(t, err, out)
	assert.Contains(t, out, "seamless")
}

// TestCheck_MonolingualFiresTheTermsViolation: the vocabulary the project
// committed is enforced, not merely retrievable. A term retired in
// `.kapi/terms.json` was visible to `kapi context search` and invisible to every
// gate, so one decision had to be written twice — once as a term, once as a
// voice rule — to be both recorded and held to.
func TestCheck_MonolingualFiresTheTermsViolation(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := monolingualProject(t)

	// The terms store is the store's projection of the committed source, so the
	// gate reads what a converged project holds.
	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	out, cerr := runCLI(t, NewCheckCmd(a), "--project", recipe, "--no-fail")
	require.NoError(t, cerr, out)
	assert.Contains(t, out, "mooring", "the retired term must be reported")
	assert.Contains(t, out, "berth", "with the preferred term as the fix")
}

// TestContext_BothRetrievalPrimitivesAgree: journey 1 step 2 gives a caller two
// ways to ask about the same graph — by location and by content — and they must
// not disagree. By-location answered from the committed terms document while
// by-content answered from the derived store, so on a monolingual project (whose
// store nothing seeded, because `up` refused to run) one was complete and the
// other empty.
func TestContext_BothRetrievalPrimitivesAgree(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := monolingualProject(t)

	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	byLocation, err := runCLI(t, NewContextCmd(a), filepath.Join(root, "docs", "berths.md"), "--project", recipe)
	require.NoError(t, err, byLocation)
	assert.Contains(t, byLocation, "mooring")

	bySearch, err := runCLI(t, NewContextCmd(a), "search", "mooring", "--project", recipe)
	require.NoError(t, err, bySearch)
	assert.Contains(t, bySearch, "mooring")
	assert.NotContains(t, bySearch, "Nothing in this project's context matches")
}

// TestContextSearch_ReportsAnUnseededStore: before anything compiles the
// committed sources, the derived store is empty — and an empty store must not
// answer like a project that holds no such term. A caller cannot tell "no
// answer" from "nowhere to look" unless the answer says so.
func TestContextSearch_ReportsAnUnseededStore(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := monolingualProject(t)

	out, err := runCLI(t, NewContextCmd(a), "search", "mooring", "--project", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "kapi up", "an unseeded store must name the verb that seeds it")
}

// TestMonolingual_IsNotAssumedFromDefaultsAlone: a recipe that names its
// languages on a collection instead of in defaults is multilingual, and must not
// be read as the front-door shape. The fan-out and the units its coverage is
// derived from resolve languages identically, so a declaration in either place
// counts.
func TestMonolingual_IsNotAssumedFromDefaultsAlone(t *testing.T) {
	recipe, _ := monolingualProject(t)
	body, err := os.ReadFile(recipe)
	require.NoError(t, err)
	multi := bytes.Replace(body,
		[]byte("      - path: \"docs/**/*.md\"\n"),
		[]byte("      - path: \"docs/**/*.md\"\n        target: \"i18n/{lang}/{path}.md\"\n"), 1)
	multi = bytes.Replace(multi,
		[]byte("    channel: northsea/docs\n"),
		[]byte("    channel: northsea/docs\n    target_languages: [nb]\n"), 1)
	require.NoError(t, os.WriteFile(recipe, multi, 0o644))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	assert.True(t, proj.DeclaresTargetLanguages(),
		"a collection-level target language is a declaration like any other")

	a := processOnlyApp(t)
	out, serr := runCLI(t, NewStatusCmd(a), "--project", recipe)
	require.NoError(t, serr, out)
	assert.NotContains(t, out, "Monolingual project")
	assert.Contains(t, out, "nb", "the collection's language must appear in the coverage grid")
}
