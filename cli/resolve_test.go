package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/host"
)

// resolveProject is a project whose directory holds work.kpz, extracted with
// messages.json as its source, and an edit to its greeting nobody packed.
func resolveProject(t *testing.T) (*App, string, string) {
	t.Helper()
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en.json"), []byte(`{"title": "Tide window"}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: resolve
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - path: "locales/en.json"
    target: "locales/{lang}.json"
`), 0o644))
	a := processOnlyApp(t)
	work := filepath.Join(root, "work.kpz")
	extractInto(t, a, work, `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n")

	svc, err := a.ChangeService(context.Background(), host.ChangeServiceOptions{Project: recipe, Origin: "apply"})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "work.kpz!messages.json", Blocks: []string{"greeting"}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	text := "Hello, edited here"
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.SetContent{Text: &text}}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	return a, recipe, work
}

// extractInto writes to work a KPZ that carries source as messages.json.
func extractInto(t *testing.T, a *App, work, source string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(source), 0o644))
	out := filepath.Join(dir, "out.kpz")
	require.NoError(t, a.ExtractToKpz(context.Background(), []string{src}, out, "fr", "", true))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(work, data, 0o644))
}

// jsonPart is the JSON document a verb printed, after any line before it.
func jsonPart(t *testing.T, out string) []byte {
	t.Helper()
	i := strings.Index(out, "{")
	require.GreaterOrEqual(t, i, 0, "no JSON document in %q", out)
	return []byte(out[i:])
}

// statusConflicts runs kapi status --json and returns its conflicts.
func statusConflicts(t *testing.T, a *App, recipe string) []host.StatusConflict {
	t.Helper()
	out, err := runCLI(t, NewStatusCmd(a), "--project", recipe, "--json")
	require.NoError(t, err, out)
	var status host.StatusOutput
	require.NoError(t, json.Unmarshal(jsonPart(t, out), &status), out)
	return status.Conflicts
}

// TestResolve_StatusNamesTheNextStepAndResolveTakesIt: a replaced KPZ whose
// document holds an unpacked edit is a "document" conflict in kapi status,
// as text and as JSON, with the kapi resolve line that settles it. A rebase
// adds the block the new version added, removes the one it removed and takes
// its change to another block, and leaves the block both changed, which
// status then lists with the line that keeps the document's wording.
func TestResolve_StatusNamesTheNextStepAndResolveTakesIt(t *testing.T) {
	a, recipe, work := resolveProject(t)
	assert.Empty(t, statusConflicts(t, a, recipe))

	extractInto(t, a, work, `{"greeting": "Hello from the team", "welcome": "Welcome aboard", "farewell": "Goodbye from the team"}`+"\n")
	conflicts := statusConflicts(t, a, recipe)
	require.Len(t, conflicts, 1)
	c := conflicts[0]
	assert.Equal(t, host.ConflictDocument, c.Kind)
	assert.Equal(t, "work.kpz!messages.json", c.Doc)
	assert.False(t, c.Rebased)
	assert.Empty(t, c.Blocks)
	assert.Equal(t, "kapi resolve 'work.kpz!messages.json' --rebase "+c.Edit+" (or --discard "+c.Edit+" to keep the document as it stands)", c.Next)

	text, err := runCLI(t, NewStatusCmd(a), "--project", recipe)
	require.NoError(t, err, text)
	assert.Contains(t, text, "Conflict: work.kpz!messages.json holds another version, written from an earlier one, that did not land")
	assert.Contains(t, text, "Next: kapi resolve 'work.kpz!messages.json' --rebase "+c.Edit)

	out := runVerb(t, a, NewResolveCmd, "resolve", "work.kpz!messages.json", "--rebase", c.Edit, "--project", recipe, "--json")
	var res host.ResolveOutput
	require.NoError(t, json.Unmarshal(jsonPart(t, out), &res), out)
	assert.Equal(t, "rebase", res.Action)
	assert.Equal(t, []string{"greeting"}, res.Contested)
	assert.Equal(t, 3, res.Carried, "the farewell's change, the welcome added and the thanks removed")
	assert.Contains(t, res.Next, "--discard "+c.Edit)

	conflicts = statusConflicts(t, a, recipe)
	require.Len(t, conflicts, 1)
	assert.True(t, conflicts[0].Rebased)
	assert.Equal(t, []string{"greeting"}, conflicts[0].Blocks)
	assert.Equal(t, "kapi apply a set_content to each block, or kapi resolve 'work.kpz!messages.json' --discard "+c.Edit+" to keep the document's wording", conflicts[0].Next)

	out, err = runCLI(t, NewResolveCmd(a), "work.kpz!messages.json", "--discard", c.Edit, "--project", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "Discarded the version of work.kpz!messages.json")
	assert.Empty(t, statusConflicts(t, a, recipe))

	svc, err := a.ChangeService(context.Background(), host.ChangeServiceOptions{Project: recipe, Origin: "status"})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "work.kpz!messages.json"})
	require.NoError(t, err)
	var got []string
	for _, b := range page.Blocks {
		got = append(got, b.Ref.Block+"="+b.Text)
	}
	assert.Equal(t, []string{"greeting=Hello, edited here", "welcome=Welcome aboard", "farewell=Goodbye from the team"}, got)

	_, err = runCLI(t, NewResolveCmd(a), "work.kpz!messages.json", "--discard", c.Edit, "--project", recipe)
	require.Error(t, err, "a settled version is no longer divergent")
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
}
