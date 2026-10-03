package host

import (
	"context"
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

// staleAfterSourceEdit edits the greeting's source and returns how many
// translations the plan of the next pass reads as stale.
func staleAfterSourceEdit(t *testing.T, a *App, recipe string) int {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), "src", "en.json"),
		[]byte(`{"greeting": "Hello, world!", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
	plan, err := a.UpPlan(context.Background(), recipe, "en")
	require.NoError(t, err)
	return plan.Totals.Stale
}

// The block history is shared by every branch of a checkout. A checkout of
// another branch brings back what a pass wrote there, and a read after the
// switch records nothing: some recorded change left each revision the read
// finds. The loop's record of each translation answers for it again, so a
// source edit on the branch is drift the loop owes a draft for.
func TestABranchSwitch_RecordsNothingAndKeepsTheLoopsBasis(t *testing.T) {
	sources := map[string]string{
		"a": `{"greeting": "Hello world", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
		"b": `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
	}
	cases := []struct {
		name      string
		passes    []string
		checkouts []string
	}{
		{"back to the branch written last", []string{"b", "a"}, []string{"b", "a"}},
		{"back to the branch written first", []string{"a", "b"}, []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
			root := filepath.Dir(recipe)
			en, qps := filepath.Join(root, "src", "en.json"), filepath.Join(root, "src", "qps.json")
			trees := map[string][2][]byte{}
			for _, branch := range tc.passes {
				require.NoError(t, os.WriteFile(en, []byte(sources[branch]), 0o644))
				runOnePass(t, a, cmd, recipe)
				enData, err := os.ReadFile(en)
				require.NoError(t, err)
				qpsData, err := os.ReadFile(qps)
				require.NoError(t, err)
				trees[branch] = [2][]byte{enData, qpsData}
			}
			require.NotEqual(t, trees["a"][1], trees["b"][1])
			recorded := len(flowHistory(t, a, root))

			for _, branch := range tc.checkouts {
				require.NoError(t, os.WriteFile(en, trees[branch][0], 0o644))
				require.NoError(t, os.WriteFile(qps, trees[branch][1], 0o644))
				readThrough(t, a, recipe, "src/en.json")
			}
			rows := flowHistory(t, a, root)
			assert.Len(t, rows, recorded, "a checkout of a branch records nothing")
			for _, r := range rows {
				assert.NotEqual(t, history.ActorExternal, r.Actor, "%s@%s", r.Block, r.Edition)
			}

			plan, err := a.UpPlan(context.Background(), recipe, "en")
			require.NoError(t, err)
			assert.Zero(t, plan.Totals.Stale, "each translation is the loop's translation of the source in front of it")
			assert.Equal(t, 1, staleAfterSourceEdit(t, a, recipe), "the loop's translation of the old wording is stale")
		})
	}
}

// greetingBlock reads the greeting block of the project's catalog, with its
// qps translation, without the read being observed.
func greetingBlock(t *testing.T, a *App, recipe string) *model.Block {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	var out *model.Block
	_, err = svc.ReadEach(change.Unobserved(context.Background()), change.ReadRequest{Doc: "src/en.json", Editions: []model.EditionKey{{Locale: "qps"}}},
		func(b *model.Block, r change.BlockRead) error {
			if r.Ref.Block == "greeting" {
				out = b
			}
			return nil
		})
	require.NoError(t, err)
	require.NotNil(t, out)
	return out
}

// A read that overlaps another writer's commit sees the writer's content
// before its record lands. The read asks the history when it ends, so a
// writer that recorded meanwhile explains the change and the read records
// nothing.
func TestARead_YieldsToAWriterThatRecordedWhileItRan(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	runOnePass(t, a, cmd, recipe)
	ctx := context.Background()

	obs := &editObserver{hist: a.changeHistory(root), rec: a.changeRecorder(ctx, root)}
	read := obs.Observe(ctx, change.DocInfo{Doc: "src/en.json"})
	require.NotNil(t, read, "the read starts from the pass's record")

	// While the read runs, a person's edit commits and records.
	b := readThrough(t, a, recipe, "src/en.json")["greeting"]
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: change.Ref{Doc: "src/en.json", Block: "greeting", Edition: model.EditionKey{Locale: "qps"}},
		IfMatch: b.Editions["qps"].Rev,
		Body:    &change.SetContent{Runs: []model.Run{{Text: &model.TextRun{Text: "Hand-written greeting"}}}},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	recorded := len(flowHistory(t, a, root))

	// The read saw the content the edit left.
	read.Saw(greetingBlock(t, a, recipe), []model.EditionKey{{Locale: "qps"}})
	read.Done(ctx)
	rows := flowHistory(t, a, root)
	assert.Len(t, rows, recorded, "the person's record explains what the read found")
	assert.Equal(t, string(change.ActorPerson), rows[0].Actor)
}

// When the read's record lands after the writer's anyway, it never takes the
// revision from the writer: the loop's record still answers for the
// translation it wrote, and the read still shows its basis.
func TestAnObservedRecord_LeavesARecordedWriteItsRevision(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	runOnePass(t, a, cmd, recipe)
	ctx := context.Background()
	b := greetingBlock(t, a, recipe)
	k := model.EditionKey{Locale: "qps"}
	rev := model.EditionRevision(b, k)
	basis := readThrough(t, a, recipe, "src/en.json")["greeting"].Editions["qps"].Basis
	require.NotEmpty(t, basis, "the pass recorded the source it translated")

	// A read that overlapped the pass's commit records the revision the pass
	// left, after the pass's own record.
	_, err := a.changeRecorder(ctx, root).Record(ctx, change.Record{
		Actor:  change.Actor{Kind: change.ActorKind(history.ActorExternal)},
		Origin: history.OriginObserved,
		Docs:   []change.DocResult{{Doc: "src/en.json", Home: "file"}},
		Transitions: []change.Transition{{EditionChange: change.EditionChange{
			Ref: change.Ref{Doc: "src/en.json", Block: "greeting", Edition: k}, Role: change.RoleDerived,
			BeforeRev: model.AbsentRevision, AfterRev: rev, Block: b,
		}}},
	})
	require.NoError(t, err)
	require.Equal(t, history.ActorExternal, flowHistory(t, a, root)[0].Actor, "the observed record is the latest")

	assert.Equal(t, basis, readThrough(t, a, recipe, "src/en.json")["greeting"].Editions["qps"].Basis,
		"the read shows the basis the pass recorded")
	assert.Equal(t, 1, staleAfterSourceEdit(t, a, recipe), "the loop's translation of the old wording is stale")
}
