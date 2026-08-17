package backend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1468 item 1, desktop half. rewriteFile is what "apply this fix" in the check
// and review panels runs: it reads the user's file, edits one block, and writes
// the file back. It opened its skeleton store with `if store, serr := …; serr ==
// nil` and, on failure, rewrote the file anyway — reformatting everything it did
// not edit while the panel reported the single fix applied. The audit that
// produced #1468 missed this site and host/toolrun.go; both are the same swallow
// and are fixed through the same seam.

// markdownOnlyASkeletonPreserves carries a setext heading, a reference link and
// irregular spacing — structure that lives in the source bytes alone and does not
// survive a re-serialization from the content model.
const markdownOnlyASkeletonPreserves = `Release notes
=============

Please  utilize  the button.

See the [reference][1] for details.

[1]: https://example.com
`

// breakTempDir points TMPDIR at a directory that does not exist, so
// os.CreateTemp("") fails the way a missing, read-only or full TMPDIR makes it
// fail in the field. rewriteFile's own atomic write stages its temp file next to
// the target, not in TMPDIR, so this breaks the skeleton and nothing else.
func breakTempDir(t *testing.T) string {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "no-such-tmpdir")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	require.Equal(t, missing, os.TempDir(), "sanity: os.TempDir must follow the swapped TMPDIR")
	return missing
}

// TestRewriteFile_PreservesEverythingItDidNotEdit is the control and the property
// the swallow destroyed: applying one fix changes that text and nothing else.
func TestRewriteFile_PreservesEverythingItDidNotEdit(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	require.NoError(t, os.WriteFile(path, []byte(markdownOnlyASkeletonPreserves), 0o644))

	edited := false
	transform := func(b *model.Block) {
		text := model.RunsText(b.SourceRuns())
		if !strings.Contains(text, "utilize") {
			return
		}
		b.SetSourceText(strings.Replace(text, "utilize", "use", 1))
		edited = true
	}
	require.NoError(t, app.rewriteFile(context.Background(), path, "markdown", "en", nil, nil, transform))
	require.True(t, edited, "fixture assumption: the block was found")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `Release notes
=============

Please  use  the button.

See the [reference][1] for details.

[1]: https://example.com
`, string(got), "only the edited paragraph changes; the setext heading, reference link and spacing survive")
}

// TestRewriteFile_FailsWhenTheSkeletonStoreCannotBeCreated is the regression.
// What a user saw: clicking "apply fix" in Kapi Desktop reported success while
// the file it wrote back had been reformatted — the setext heading rewritten as
// ATX, the reference link inlined, the double spaces collapsed — an edit nobody
// asked for, applied to a file they had open.
func TestRewriteFile_FailsWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	require.NoError(t, os.WriteFile(path, []byte(markdownOnlyASkeletonPreserves), 0o644))

	breakTempDir(t)

	err := app.rewriteFile(context.Background(), path, "markdown", "en", nil, nil, func(*model.Block) {})
	require.Error(t, err, "a rewrite that cannot preserve the file must fail, not reformat it")
	assert.Contains(t, err.Error(), "notes.md", "the error must name the file")
	assert.Contains(t, err.Error(), "formatting", "and say what would have been lost")

	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, markdownOnlyASkeletonPreserves, string(got),
		"and the user's file must be untouched")
}

// ─── item 2, desktop half: ListOutputs' copy of the glob swallow ─────────────

// TestListOutputs_MalformedPatternFails is the regression. What a user saw: the
// outputs panel listing the target files of one collection and none of the
// other's, with no indication that a pattern in the recipe had failed — the
// panel reading as "these are all the outputs".
func TestListOutputs_MalformedPatternFails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "en.json"), []byte(`{"a":"b"}`), 0o644))

	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Test",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
		},
		Collections: []project.Collection{
			{Path: "input/*.json", Target: "output/{lang}", Format: &project.FormatSpec{Name: "json"}},
			{Path: "store/[a-.json", Target: "output/{lang}", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	_, err := app.ListOutputs(tab.ID)
	require.Error(t, err, "a pattern that cannot be expanded must fail the listing, not shorten it")
	assert.Contains(t, err.Error(), "store/[a-.json", "the error must name the pattern to fix")
}

// TestListOutputs_PatternMatchingNothingIsNotAnError is the discriminating
// control: a well-formed pattern with no matches yet is ordinary and must still
// list the rest.
func TestListOutputs_PatternMatchingNothingIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "en.json"), []byte(`{"a":"b"}`), 0o644))

	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Test",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
		},
		Collections: []project.Collection{
			{Path: "input/*.json", Target: "output/{lang}", Format: &project.FormatSpec{Name: "json"}},
			{Path: "not-written-yet/**/*.json", Target: "output/{lang}", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	outs, err := app.ListOutputs(tab.ID)
	require.NoError(t, err, "an empty collection is not a failed collection")
	assert.Contains(t, outs, "input/en.json")
}
