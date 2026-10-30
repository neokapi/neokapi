package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

// fixVoice is the voice of the fix fixture. Its word rules move into the
// project's terms when the context is read: "utilize", "e-mail" and "mail
// address" fail and name a replacement, the last two over part of the same
// words; "login" is advisory. Its one pattern names no replacement.
const fixVoice = `name: Fix Voice
style:
  prohibited_patterns:
    - regex: '(?i)\bsimply\b'
      description: Say what to do without minimising it
terms:
  - term: utilize
    replacement: use
  - term: e-mail
    replacement: email
  - term: mail address
    replacement: postal address
  - term: login
    replacement: sign in
    advisory: true
`

const fixRecipe = `version: v1
id: %s
name: fixfixture
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: docs
    content:
      - path: "docs/*.md"
        target: "docs/{lang}/*.md"
      - path: "strings/*.json"
        target: "strings/{lang}/*.json"
`

const (
	fixGuide = "# Guide\n\nWe **utilize** the widget and utilize it, as the [manual](https://example.com/manual) says.\n\n" +
		"Simply send an e-mail address. Use your login.\n\n- Keep this line as it is.\n"
	fixGuideFixed = "# Guide\n\nWe **use** the widget and use it, as the [manual](https://example.com/manual) says.\n\n" +
		"Simply send an email address. Use your login.\n\n- Keep this line as it is.\n"
	fixStrings      = "{\n  \"title\": \"Please utilize the widget\",\n  \"body\":   \"Nothing to change here\"\n}\n"
	fixStringsFixed = "{\n  \"title\": \"Please use the widget\",\n  \"body\":   \"Nothing to change here\"\n}\n"
	fixFrench       = "# Guide\n\nNous utilize le widget.\n"
)

// fixFixture is a project whose voice and terms govern a markdown guide and a
// JSON file, with a French translation of the guide.
type fixFixture struct {
	app                 *App
	root, recipe        string
	guide, strs, french string
}

func newFixFixture(t *testing.T) fixFixture {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(strings.Replace(fixRecipe, "%s", projectIDFor("fix"+t.Name()), 1)), 0o644))
	f := fixFixture{
		root: root, recipe: recipe,
		guide:  filepath.Join(root, "docs", "guide.md"),
		strs:   filepath.Join(root, "strings", "en.json"),
		french: filepath.Join(root, "docs", "fr", "guide.md"),
	}
	for path, content := range map[string]string{f.guide: fixGuide, f.strs: fixStrings, f.french: fixFrench} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root), []byte(fixVoice), 0o644))
	readProjectContext(t, root)
	f.app = &App{SourceLang: "en"}
	t.Cleanup(f.app.Shutdown)
	t.Chdir(root)
	return f
}

// command is a kapi fix command in the fixture's project, printing to out.
func (f fixFixture) command(t *testing.T, out *bytes.Buffer) *EnvCommand {
	t.Helper()
	cmd := commitCommand(t, f.recipe)
	if out != nil {
		cmd.SetOut(out)
	}
	return cmd
}

func fixedBytes(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// kapi fix over the project applies every fix of a failing finding through
// the change service, and every byte outside the replaced words stays: the
// bold around a replaced word, the link, and the JSON
// alignment. A fix over the words another fix changed is refused and
// reported, the advisory term and the translation are left alone, and the
// finding with no replacement is listed as needing a rewrite.
func TestFixFiles_AppliesEachFixAndKeepsFormatting(t *testing.T) {
	f := newFixFixture(t)

	var out bytes.Buffer
	err := f.app.RunFix(f.command(t, &out), FixOptions{})
	assert.Equal(t, ExitError, ExitCode(nil, err), "a refused fix exits 1")

	assert.Equal(t, fixGuideFixed, fixedBytes(t, f.guide))
	assert.Equal(t, fixStringsFixed, fixedBytes(t, f.strs))
	assert.Equal(t, fixFrench, fixedBytes(t, f.french), "the translation is not touched")

	assert.Equal(t, `4 fixed in 2 files
  docs/guide.md    3 fixed: utilize → use, utilize → use, e-mail → email
  strings/en.json  1 fixed: utilize → use
1 finding needs a rewrite, with no automatic fix (voice.style). Edit by hand.
1 finding not fixed. Edit by hand:
  docs/guide.md guide/p#2 "mail address": another fix changes the same words
kapi check: 1 failing finding remains (voice)
`, out.String())
}

// The result names each file, its fixed pairs and its change set result, and
// counts what remains for a person.
func TestFixFiles_Result(t *testing.T) {
	f := newFixFixture(t)

	res, err := f.app.FixFiles(t.Context(), f.command(t, nil), FixOptions{})
	require.NoError(t, err)
	assert.Equal(t, 4, res.Fixed)
	require.Len(t, res.Files, 2)
	assert.Equal(t, "docs/guide.md", res.Files[0].Doc)
	assert.Equal(t, 3, res.Files[0].Fixed)
	assert.Equal(t, []FixPair{{"utilize", "use"}, {"utilize", "use"}, {"e-mail", "email"}}, res.Files[0].Pairs)
	assert.Equal(t, change.SetApplied, res.Files[0].Result.Status)
	assert.True(t, fileWritten(res.Files[0].Result))

	require.Len(t, res.NotFixed, 1)
	assert.Equal(t, "mail address", res.NotFixed[0].Finding.Location.Snippet)
	require.NotNil(t, res.NotFixed[0].Error)
	assert.Equal(t, change.SubcodeOverlap, res.NotFixed[0].Error.Subcode)

	require.Len(t, res.NeedsRewrite, 1)
	assert.Equal(t, "voice.style", res.NeedsRewrite[0].Rule)

	require.NotNil(t, res.Remaining)
	assert.Equal(t, 1, res.Remaining.Failing)
	assert.Equal(t, []string{"voice"}, res.Remaining.Checks)
}

// --dry-run computes every change set, prints each file's diff and writes
// nothing.
func TestFixFiles_DryRunWritesNothing(t *testing.T) {
	f := newFixFixture(t)

	var out bytes.Buffer
	cmd := f.command(t, &out)
	err := f.app.RunFix(cmd, FixOptions{DryRun: true})
	assert.Equal(t, ExitError, ExitCode(nil, err))

	assert.Equal(t, fixGuide, fixedBytes(t, f.guide))
	assert.Equal(t, fixStrings, fixedBytes(t, f.strs))
	text := out.String()
	assert.Contains(t, text, "-We **utilize** the widget")
	assert.Contains(t, text, "+We **use** the widget")
	assert.Contains(t, text, "4 to fix in 2 files; nothing written (--dry-run)\n")
	assert.NotContains(t, text, "kapi check:", "a dry run checks nothing again")

	res, err := f.app.FixFiles(t.Context(), f.command(t, nil), FixOptions{DryRun: true})
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Equal(t, 4, res.Fixed)
	assert.Nil(t, res.Remaining)
	for _, file := range res.Files {
		assert.Equal(t, change.SetPreviewed, file.Result.Status)
		assert.False(t, fileWritten(file.Result))
	}
	assert.Equal(t, fixGuide, fixedBytes(t, f.guide))
}

// The JSON result carries the schema, each file's change set result as kapi
// apply --json prints it, the findings not fixed with the refusal, and the
// findings that need a rewrite.
func TestFixFiles_JSON(t *testing.T) {
	f := newFixFixture(t)

	res, err := f.app.FixFiles(t.Context(), f.command(t, nil), FixOptions{})
	require.NoError(t, err)
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	var wire struct {
		Schema string `json:"schema"`
		Fixed  int    `json:"fixed"`
		Files  []struct {
			File   string `json:"file"`
			Doc    string `json:"doc"`
			Fixed  int    `json:"fixed"`
			Pairs  []struct{ From, To string }
			Result struct {
				Schema string `json:"schema"`
				Status string `json:"status"`
			} `json:"result"`
		} `json:"files"`
		NotFixed []struct {
			Finding struct {
				Rule string `json:"rule"`
			} `json:"finding"`
			Reason string `json:"reason"`
			Error  struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"not_fixed"`
		NeedsRewrite []struct {
			Rule string `json:"rule"`
		} `json:"needs_rewrite"`
		Remaining struct {
			Failing int `json:"failing"`
		} `json:"remaining"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire), "%s", raw)
	assert.Equal(t, FixResultSchemaID, wire.Schema)
	assert.Equal(t, 4, wire.Fixed)
	require.Len(t, wire.Files, 2)
	assert.Equal(t, change.ResultSchemaID, wire.Files[0].Result.Schema)
	assert.Equal(t, "applied", wire.Files[0].Result.Status)
	require.Len(t, wire.NotFixed, 1)
	assert.Equal(t, "terms.vocabulary", wire.NotFixed[0].Finding.Rule)
	assert.Equal(t, "guard", wire.NotFixed[0].Error.Code)
	assert.Equal(t, "another fix changes the same words", wire.NotFixed[0].Reason)
	require.Len(t, wire.NeedsRewrite, 1)
	assert.Equal(t, 1, wire.Remaining.Failing)
}

// kapi fix exits 0 when every fix landed, also when findings with no fix
// remain, and when there is nothing to fix.
func TestRunFix_ExitsZeroWhenEveryFixLanded(t *testing.T) {
	f := newFixFixture(t)

	var out bytes.Buffer
	require.NoError(t, f.app.RunFix(f.command(t, &out), FixOptions{Paths: []string{"strings/en.json"}}))
	assert.Equal(t, "1 fixed in 1 file\n  strings/en.json  1 fixed: utilize → use\nkapi check: no failing findings\n", out.String())
	assert.Equal(t, fixStringsFixed, fixedBytes(t, f.strs))
	assert.Equal(t, fixGuide, fixedBytes(t, f.guide), "a file kapi fix was not given is not written")

	out.Reset()
	require.NoError(t, f.app.RunFix(f.command(t, &out), FixOptions{Paths: []string{"strings/en.json"}}))
	assert.Equal(t, "Nothing to fix: no failing finding has an automatic fix.\nkapi check: no failing findings\n", out.String())
}

// A check that did not run exits as kapi check does, 4, and writes nothing.
func TestRunFix_CheckThatDidNotRun(t *testing.T) {
	f := newFixFixture(t)
	empty := filepath.Join(f.root, "docs", "empty.md")
	require.NoError(t, os.WriteFile(empty, nil, 0o644))

	err := f.app.RunFix(f.command(t, nil), FixOptions{Paths: []string{"docs/empty.md"}})
	require.ErrorIs(t, err, ErrCheckNotRun)
	assert.Equal(t, ExitNotRun, ExitCode(nil, err))
}

// The file of a translation, named explicitly, gets no fix: the check holds
// its words to the source language's rules, and the change service reads the
// file as the translation. Its finding is listed as needing a rewrite and the
// file stays as it is.
func TestFixFiles_ANamedTranslationIsNotWritten(t *testing.T) {
	f := newFixFixture(t)

	res, err := f.app.FixFiles(t.Context(), f.command(t, nil), FixOptions{Paths: []string{"docs/fr/guide.md"}})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Fixed)
	assert.Empty(t, res.Files)
	require.NotEmpty(t, res.NeedsRewrite)
	assert.Equal(t, "terms.vocabulary", res.NeedsRewrite[0].Rule)
	assert.Equal(t, fixFrench, fixedBytes(t, f.french))
}
