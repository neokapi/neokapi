package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi context <path>` — the by-location half of the retrieval surface
// (AD-037), CLI leg. These assert that the verb prints the host answer's own
// rendering rather than a second one: the `context://` MCP resources serve the
// same bytes, and the parity between the two surfaces is a test, not a
// convention.

// writeGovernedProject builds one governed collection with a voice and a
// vocabulary, plus a file no collection claims.
func writeGovernedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	write("kapi.yaml", `version: v1
name: acme
defaults:
  source_language: en
  target_languages: [nb]
  voice: .kapi/voice.yaml
  terms_source: .kapi/terms.json
profiles:
  acme:
    channels: [docs]
collections:
  - name: acme-docs
    channel: acme/docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", `name: Acme
description: Plain, exact writing.
tone:
  personality: [clear, restrained]
  formality: neutral
`)
	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-memory",
      "terms": [
        {"text": "content memory", "locale": "en", "status": "preferred"},
        {"text": "translation memory", "locale": "en", "status": "deprecated"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)
	write("docs/guide.md", "# Guide\n")
	t.Chdir(root)
	return root
}

// runContext executes `kapi context` with the given arguments and returns what
// it wrote to stdout.
func runContext(t *testing.T, a *App, args ...string) string {
	t.Helper()
	out, err := runContextE(t, a, args...)
	require.NoError(t, err)
	return out
}

// runContextE is runContext for the cases where the refusal is the result.
func runContextE(t *testing.T, a *App, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewContextCmd(a))
	root.SetArgs(append([]string{"context"}, args...))
	root.SilenceUsage, root.SilenceErrors = true, true

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

// TestContextPath_PrintsTheHostRender is the CLI leg of the parity contract:
// the verb prints the answer's own FormatText, which is the body the
// `context://` resource serves. Two renderings of one answer is how the
// surfaces came to disagree before.
func TestContextPath_PrintsTheHostRender(t *testing.T) {
	writeGovernedProject(t)
	a := &App{}

	got := runContext(t, a, "docs/guide.md")

	cmd := host.NewEnvCommand(t.Context(), "context")
	req := host.ContextPointRequest{Path: "docs/guide.md"}
	src, cleanup := a.ContextSourcesAt(cmd, req)
	defer cleanup()
	answer, err := host.ResolveContextAt(t.Context(), src, req)
	require.NoError(t, err)

	var want bytes.Buffer
	require.NoError(t, answer.FormatText(&want))
	assert.Equal(t, want.String(), got)
}

// TestContextPath_AnswersWhatAppliesHere: the point, the voice in force and the
// vocabulary bound there, in one document — the question a writer actually has.
func TestContextPath_AnswersWhatAppliesHere(t *testing.T) {
	writeGovernedProject(t)

	got := runContext(t, &App{}, "docs/guide.md")

	assert.Contains(t, got, "# Context at docs/guide.md")
	assert.Contains(t, got, "Point `acme/docs`: profile `acme`, channel `docs`, collection `acme-docs`.")
	assert.Contains(t, got, "## Voice Guide: Acme")
	assert.Contains(t, got, "~~translation memory~~ → say **content memory**")
	assert.Contains(t, got, "Answered from this project alone")
}

// TestContextPath_JSONIsTheSameDocument: --json is the structured rendering of
// the answer the markdown renders, so a program and a model read one thing.
func TestContextPath_JSONIsTheSameDocument(t *testing.T) {
	writeGovernedProject(t)

	raw := runContext(t, &App{}, "docs/guide.md", "--json")

	var got host.ContextAnswer
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	assert.Equal(t, "acme/docs", got.Point.Ref)
	assert.Equal(t, host.ScopeProject, got.Scope)
	require.NotNil(t, got.Voice)
	assert.Equal(t, "Acme", got.Voice.Name)
	assert.NotEmpty(t, got.Voice.Guide, "the guidance travels in the structured shape too")
}

// TestContextPath_UnclaimedPathSaysSo: a location no collection claims is
// governed by the project default, which is an answer — and a different one
// from "no governance exists".
func TestContextPath_UnclaimedPathSaysSo(t *testing.T) {
	writeGovernedProject(t)

	got := runContext(t, &App{}, "README.md")

	assert.Contains(t, got, "This location sits at the project's default point: no profile claims it.")
}

// TestContextPath_NoAddressShowsHelp: `kapi context` on its own is not a
// question, so it explains the two that are rather than answering neither.
func TestContextPath_NoAddressShowsHelp(t *testing.T) {
	writeGovernedProject(t)

	got := runContext(t, &App{})

	assert.Contains(t, got, "kapi context <path>")
	assert.Contains(t, got, "kapi context search <query>")
}

// writeTwoVoiceProject builds a project whose profile binds its own voice file,
// distinct from the one `defaults.voice` binds. Two voices is what makes a
// misresolved point visible: an answer at the wrong point does not merely name
// the wrong ref, it hands back the wrong guidance under the same confident
// wording. It leaves the working directory alone — these tests choose it.
func writeTwoVoiceProject(t *testing.T) string {
	t.Helper()
	// Every root the run could otherwise inherit is pinned to a throwaway dir,
	// so a profile in the developer's own voice store can never answer here.
	// KAPI_NO_PROJECT is deliberately NOT set: discovery from cwd is half of
	// what these tests compare, and every one of them stands in a temp
	// directory, where an upward walk reaches no recipe of ours.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())

	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	write("kapi.yaml", `version: v1
name: acme
defaults:
  source_language: en
  target_languages: [nb]
  voice: .kapi/voice.yaml
profiles:
  guides:
    channels: [docs]
    voice: .kapi/profiles/guides/voice.yaml
collections:
  - name: acme-guides
    channel: guides/docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", `name: Acme Default
description: The voice at the project's default point.
`)
	write(".kapi/profiles/guides/voice.yaml", `name: Acme Guides
description: The voice the guides profile binds.
`)
	write("docs/guide.md", "# Guide\n")
	return root
}

// foreignCwd puts the test in a directory with nothing to do with the project —
// the position of every caller that binds a project by -p: an agent runner, a
// CI step, an editor extension.
func foreignCwd(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

// TestContextPath_RelativePathBoundByFlagResolvesAgainstTheProject: a relative
// location named alongside -p means what the recipe means by it. Read against a
// foreign working directory it lands outside the project, and answering it from
// the default point returns another location's voice — the failure this holds
// shut, because the caller that binds by -p is the one that cannot see it.
func TestContextPath_RelativePathBoundByFlagResolvesAgainstTheProject(t *testing.T) {
	root := writeTwoVoiceProject(t)
	foreignCwd(t)

	got := runContext(t, &App{}, "docs/guide.md", "-p", root)

	assert.Contains(t, got, "# Context at docs/guide.md")
	assert.Contains(t, got, "Point `guides/docs`: profile `guides`, channel `docs`, collection `acme-guides`.")
	assert.Contains(t, got, "Voice `Acme Guides`, bound by `profiles.guides.voice`")
	assert.NotContains(t, got, "default point")
	assert.NotContains(t, got, "Acme Default")
}

// TestContextPath_FlagAndCwdAgree: the same location asked about from inside the
// project answers the same point, so -p is a way to stand somewhere rather than
// a second resolution.
func TestContextPath_FlagAndCwdAgree(t *testing.T) {
	root := writeTwoVoiceProject(t)
	foreignCwd(t)
	byFlag := runContext(t, &App{}, "docs/guide.md", "-p", root)

	t.Chdir(root)
	byCwd := runContext(t, &App{}, "docs/guide.md")

	assert.Equal(t, byCwd, byFlag)
}

// TestContextPath_RelativePathFromASubdirectoryStillReadsAgainstCwd: standing in
// the tree, a relative location means what a person standing there means by it.
func TestContextPath_RelativePathFromASubdirectoryStillReadsAgainstCwd(t *testing.T) {
	root := writeTwoVoiceProject(t)
	t.Chdir(filepath.Join(root, "docs"))

	got := runContext(t, &App{}, "guide.md", "-p", root)

	assert.Contains(t, got, "# Context at docs/guide.md")
	assert.Contains(t, got, "Point `guides/docs`: profile `guides`, channel `docs`, collection `acme-guides`.")
}

// TestContextPath_LocationOutsideTheProjectIsRefused: a location the recipe's
// coordinate space does not contain has no point, and the project's default
// point is a different location's answer. Refusing says so; answering does not.
func TestContextPath_LocationOutsideTheProjectIsRefused(t *testing.T) {
	root := writeTwoVoiceProject(t)
	outside := foreignCwd(t)

	_, err := runContextE(t, &App{}, filepath.Join(outside, "elsewhere.md"), "-p", root)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "elsewhere.md is not inside this project")
	assert.Contains(t, err.Error(), "Name a location inside it")
}

// TestContextPath_EscapingRelativePathIsRefused: `../` out of the project is the
// same refusal, whichever directory the relative form was read against.
func TestContextPath_EscapingRelativePathIsRefused(t *testing.T) {
	root := writeTwoVoiceProject(t)
	foreignCwd(t)

	_, err := runContextE(t, &App{}, "../elsewhere.md", "-p", root)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not inside this project")
}
