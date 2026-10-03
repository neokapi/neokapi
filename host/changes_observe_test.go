package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// readThrough reads doc through the project's change service with the qps
// edition, as kapi inspect does, and returns the blocks' read revisions.
func readThrough(t *testing.T, a *App, recipe, doc string) map[string]change.BlockRead {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	out := map[string]change.BlockRead{}
	_, err = svc.ReadEach(context.Background(), change.ReadRequest{Doc: doc, Editions: []model.EditionKey{{Locale: "qps"}}},
		func(_ *model.Block, r change.BlockRead) error {
			out[r.Ref.Block] = r
			return nil
		})
	require.NoError(t, err)
	return out
}

// An edit made outside kapi is recorded by the next read (design 5.7): one
// observed content.edit for the document, hash-only, whose actor is external,
// so the block history still says who last wrote each edition and from what.
func TestARead_RecordsAnEditMadeOutsideKapi(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	runOnePass(t, a, cmd, recipe)
	flowWrote := flowHistory(t, a, root)
	require.Len(t, flowWrote, 3, "the pass recorded its translations")
	var greeting history.Row
	for _, r := range flowWrote {
		if r.Block == "greeting" {
			greeting = r
		}
	}
	editsBefore := len(editOps(t, a, root))

	// A person rewrites one translation in their editor.
	qps := filepath.Join(root, "src", "qps.json")
	data, err := os.ReadFile(qps)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(qps, []byte(`{"greeting": "Hand-written greeting", "farewell": `+
		string(fieldOf(t, data, "farewell"))+`, "thanks": `+string(fieldOf(t, data, "thanks"))+"}\n"), 0o644))

	read := readThrough(t, a, recipe, "src/en.json")
	rows := flowHistory(t, a, root)
	require.Len(t, rows, 4, "the read recorded the one edition that changed")
	observed := rows[0]
	assert.Equal(t, "greeting", observed.Block)
	assert.Equal(t, "qps", observed.Edition)
	assert.Equal(t, history.ActorExternal, observed.Actor)
	assert.Equal(t, history.OriginObserved, observed.Origin)
	assert.Equal(t, greeting.After, observed.Before, "the change starts from the revision the flow left")
	assert.Equal(t, read["greeting"].Editions["qps"].Rev, observed.After, "and ends at the revision the read found")
	assert.Empty(t, observed.Basis, "nobody knows which source the hand-written text was made from")
	assert.Empty(t, observed.Ops, "no operation explains it")
	assert.Empty(t, read["greeting"].Editions["qps"].Basis, "a read shows no basis for it")

	ops := editOps(t, a, root)
	require.Len(t, ops, editsBefore+1, "one operation for the document")
	payload := string(ops[len(ops)-1].Payload)
	assert.NotContains(t, payload, "runs_", "an observed record keeps hashes only")
	assert.NotContains(t, payload, "Hand-written", "and no text")

	// The history names nobody, which is what a person reading it is told.
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	h, err := svc.History(context.Background(), change.HistoryRequest{Ref: change.Ref{Doc: "src/en.json", Block: "greeting", Edition: model.EditionKey{Locale: "qps"}}})
	require.NoError(t, err)
	require.NotEmpty(t, h.Entries)
	assert.Nil(t, h.Entries[0].Actor, "an edit made outside kapi has no author anybody knows")
	assert.Equal(t, history.OriginObserved, h.Entries[0].Origin)

	// The next read finds the chain whole and records nothing.
	readThrough(t, a, recipe, "src/en.json")
	assert.Len(t, flowHistory(t, a, root), 4)
	assert.Len(t, editOps(t, a, root), editsBefore+1)
}

// The read a flow makes before it runs is a read like any other: a source a
// person edited outside kapi is recorded as observed before the flow records
// what it wrote from it.
func TestAFlowPass_RecordsTheEditItFoundBeforeItsOwn(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	// The pass reads the source with its translation joined from the target
	// file, so its read covers both.
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	b := readThrough(t, a, recipe, "src/en.json")["greeting"]
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev,
		Body: &change.SetContent{Runs: []model.Run{{Text: &model.TextRun{Text: "Hello, world"}}}},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	// The person edits the source again, this time in their editor.
	src := filepath.Join(root, "src", "en.json")
	require.NoError(t, os.WriteFile(src,
		[]byte(`{"greeting": "Hello, everyone", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
	runOnePass(t, a, cmd, recipe)

	var sourceRows []history.Row
	for _, r := range flowHistory(t, a, root) {
		if r.Block == "greeting" && r.Edition == "en" {
			sourceRows = append(sourceRows, r)
		}
	}
	require.Len(t, sourceRows, 2, "the person's edit through kapi, then the one the pass's read found")
	assert.Equal(t, string(change.ActorPerson), sourceRows[1].Actor)
	assert.Equal(t, history.ActorExternal, sourceRows[0].Actor)
	assert.Equal(t, sourceRows[1].After, sourceRows[0].Before)
	assert.Equal(t, readThrough(t, a, recipe, "src/en.json")["greeting"].Rev, sourceRows[0].After)
}

// fieldOf returns the JSON value of key in a flat JSON object, as written.
func fieldOf(t *testing.T, data []byte, key string) []byte {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	v, ok := m[key]
	require.True(t, ok, "%s holds %s", data, key)
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return out
}

// A run that prints its change set records nothing, an edit made outside kapi
// included: its read before the run is left unobserved.
func TestAPrintingPass_RecordsNothingItFinds(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	runOnePass(t, a, cmd, recipe)
	require.Len(t, flowHistory(t, a, root), 3)

	// A person rewrites one translation in their editor.
	qps := filepath.Join(root, "src", "qps.json")
	data, err := os.ReadFile(qps)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(qps, []byte(`{"greeting": "Hand-written greeting", "farewell": `+
		string(fieldOf(t, data, "farewell"))+`, "thanks": `+string(fieldOf(t, data, "thanks"))+"}\n"), 0o644))
	// And a source edit gives the pass work in the document.
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting": "Hello world", "farewell": "Goodbye for now", "thanks": "Thank you"}`+"\n"), 0o644))
	set := printRun(t, a, cmd, func() error { return a.ExecuteUp(cmd, recipe) })
	require.NotEmpty(t, set.Ops, "the pass read the document and would change it")
	assert.Len(t, flowHistory(t, a, root), 3, "the printing run recorded nothing")
}
