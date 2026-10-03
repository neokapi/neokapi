package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
)

// newFlowProject writes a project whose default flow pseudo-translates a JSON
// catalog into qps, which needs no key and gives the same text every run. It
// runs under the in-repo isolation contract (CLAUDE.md).
func newFlowProject(t *testing.T, materialize string) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting": "Hello world", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "FlowRecordTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"qps"},
			Flow:            "pseudo",
			TranslateAfter:  string(model.TranslateAfterNone),
			Materialize:     materialize,
		},
		Collections: []project.Collection{{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"}},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	t.Chdir(dir)
	a := &App{}
	t.Cleanup(a.Shutdown)
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe
}

// runOnePass runs one convergence pass of the project's default flow.
func runOnePass(t *testing.T, a *App, cmd *EnvCommand, recipe string) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{MaxPasses: 1, noChecks: true, capture: &out}))
}

// flowHistory is the block history of the project's source document.
func flowHistory(t *testing.T, a *App, root string) []history.Row {
	t.Helper()
	ctx := context.Background()
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	rows, err := db.History().Document(ctx, a.documentIndexOrEmpty(ctx, root).Key("src/en.json"))
	require.NoError(t, err)
	return rows
}

// sourceRevisions reads each block's source and qps revisions as the change
// service reads them.
func sourceRevisions(t *testing.T, a *App, recipe string) map[string][2]string {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	out := map[string][2]string{}
	_, err = svc.ReadEach(context.Background(), change.ReadRequest{Doc: "src/en.json", Editions: []model.EditionKey{{Locale: "qps"}}},
		func(b *model.Block, r change.BlockRead) error {
			out[r.Ref.Block] = [2]string{r.Rev, r.Editions["qps"].Rev}
			return nil
		})
	require.NoError(t, err)
	return out
}

func TestFlowRun_RecordsWhatItWroteAsOneEditPerDocument(t *testing.T) {
	cases := []struct {
		name        string
		materialize string
	}{
		{name: "a pass that writes where the recipe points", materialize: project.MaterializeManual},
		{name: "a pass whose drafts are delivered", materialize: project.MaterializeOnConverge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, cmd, recipe := newFlowProject(t, tc.materialize)
			root := filepath.Dir(recipe)
			runOnePass(t, a, cmd, recipe)
			require.FileExists(t, filepath.Join(root, "src", "qps.json"))

			rows := flowHistory(t, a, root)
			require.Len(t, rows, 3, "one transition per block the pass translated")
			revs := sourceRevisions(t, a, recipe)
			op := rows[0].Op
			for _, r := range rows {
				assert.Equal(t, op, r.Op, "the document's transitions are one content.edit")
				assert.Equal(t, "qps", r.Edition)
				assert.Equal(t, string(change.ActorTool), r.Actor)
				assert.Equal(t, "pseudo", r.ActorName)
				assert.Equal(t, "flow:pseudo", r.Origin)
				assert.Equal(t, model.AbsentRevision, r.Before, "the target file did not exist")
				assert.Equal(t, revs[r.Block][1], r.After, "the revision a read of %s finds", r.Block)
				assert.Equal(t, revs[r.Block][0], r.Basis, "the basis is the source the pass translated")
			}

			// A pass that writes the same bytes records nothing.
			runOnePass(t, a, cmd, recipe)
			assert.Len(t, flowHistory(t, a, root), 3)

			// A source edit moves the one block it touched.
			src := filepath.Join(root, "src", "en.json")
			require.NoError(t, os.WriteFile(src,
				[]byte(`{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
			runOnePass(t, a, cmd, recipe)
			rows = flowHistory(t, a, root)
			require.Len(t, rows, 4)
			revs = sourceRevisions(t, a, recipe)
			last := rows[0]
			assert.Equal(t, "greeting", last.Block)
			assert.NotEqual(t, op, last.Op)
			assert.Equal(t, revs["greeting"][0], last.Basis)
			assert.Equal(t, revs["greeting"][1], last.After)
		})
	}
}

// printRun runs a porcelain under --print-ops and returns the change set it
// printed.
func printRun(t *testing.T, a *App, cmd *EnvCommand, run func() error) change.Set {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.Flags().Set(printOpsFlag, "true"))
	require.NoError(t, a.WithPrintedOps(cmd, run))
	require.NoError(t, cmd.Flags().Set(printOpsFlag, "false"))
	var set change.Set
	require.NoError(t, json.Unmarshal(out.Bytes(), &set), "the output is a change set: %s", out.String())
	return set
}

func TestFlowRun_PrintsTheChangeSetKapiApplyAppliesToTheSameBytes(t *testing.T) {
	// One project prints its pass and applies what it printed; its twin runs
	// the pass. The two target files must be the same bytes.
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	set := printRun(t, a, cmd, func() error { return a.ExecuteUp(cmd, recipe) })
	assert.NoFileExists(t, filepath.Join(root, "src", "qps.json"), "a printing run writes nothing")
	assert.Empty(t, flowHistory(t, a, root), "and records nothing")
	require.Len(t, set.Ops, 3)
	revs := sourceRevisions(t, a, recipe)
	for _, op := range set.Ops {
		assert.Equal(t, change.KindSetContent, op.Kind)
		assert.Equal(t, "src/en.json", op.At.Doc)
		assert.Equal(t, model.EditionKey{Locale: "qps"}, op.At.Edition)
		assert.Equal(t, model.AbsentRevision, op.IfMatch)
		assert.Equal(t, revs[op.At.Block][0], op.Basis)
	}

	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en", Origin: "apply"})
	require.NoError(t, err)
	res, err := svc.Apply(context.Background(), set, change.Actor{Kind: change.ActorPerson, Name: "tester"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	applied, err := os.ReadFile(filepath.Join(root, "src", "qps.json"))
	require.NoError(t, err)

	b, bcmd, brecipe := newFlowProject(t, project.MaterializeManual)
	runOnePass(t, b, bcmd, brecipe)
	ran, err := os.ReadFile(filepath.Join(filepath.Dir(brecipe), "src", "qps.json"))
	require.NoError(t, err)
	assert.Equal(t, string(ran), string(applied))
}

// writeDuringRun passes every part through and, before the first, writes data
// to path: a person saving the file while the run works.
type writeDuringRun struct {
	tool.BaseTool
	path string
	data string
	once sync.Once
}

func (w *writeDuringRun) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		w.once.Do(func() { _ = os.WriteFile(w.path, []byte(w.data), 0o644) })
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// setTarget writes text as the qps target of the block named key.
type setTarget struct {
	tool.BaseTool
	key, text string
}

func (s *setTarget) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		if b, ok := p.Resource.(*model.Block); ok && b.Name == s.key {
			if err := tool.WriteAs(ctx, b, "set", func(v tool.VariantView) error {
				v.SetTargetText("qps", s.text)
				return nil
			}); err != nil {
				return err
			}
		}
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestFlowRun_AppliesItsChangesAgainToAFileThatMovedWhileItWorked(t *testing.T) {
	cases := []struct {
		name string
		// saved is what a person saves in the target file while the run works.
		saved string
		want  string
		moved bool
	}{
		{
			name:  "a save to another block keeps both",
			saved: `{"greeting": "Hei", "farewell": "Ha det", "thanks": "Takk av en person"}` + "\n",
			want:  `{"greeting": "Bonjour", "farewell": "Ha det", "thanks": "Takk av en person"}` + "\n",
		},
		{
			name:  "a save to the block the run wrote keeps the save",
			saved: `{"greeting": "Hallo fra en person", "farewell": "Ha det", "thanks": "Takk"}` + "\n",
			want:  `{"greeting": "Hallo fra en person", "farewell": "Ha det", "thanks": "Takk"}` + "\n",
			moved: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
			root := filepath.Dir(recipe)
			target := filepath.Join(root, "src", "qps.json")
			require.NoError(t, os.WriteFile(target, []byte(`{"greeting": "Hei", "farewell": "Ha det", "thanks": "Takk"}`+"\n"), 0o644))
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			a.ProjectContext = project.NewProjectContext(proj, recipe)

			home, docs := a.flowDocuments(context.Background(), cmd, root)
			require.NotNil(t, docs)
			runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: a.FormatReg, SourceLocale: "en", Home: home, Documents: docs})
			tools := []tool.Tool{
				&writeDuringRun{ToolName: "save", path: target, data: tc.saved},
				&setTarget{ToolName: "set", key: "greeting", text: "Bonjour"},
			}
			err = runner.RunFile(context.Background(), "edit", tools, filepath.Join(root, "src", "en.json"), target, "qps")
			if tc.moved {
				require.Error(t, err)
				assert.True(t, flow.IsMoved(err), "%v", err)
			} else {
				require.NoError(t, err)
			}
			got, rerr := os.ReadFile(target)
			require.NoError(t, rerr)
			assert.JSONEq(t, tc.want, string(got))

			rows := flowHistory(t, a, root)
			if tc.moved {
				assert.Empty(t, rows, "nothing landed, so nothing is recorded")
				return
			}
			require.Len(t, rows, 1, "the change applied again is recorded once")
			assert.Equal(t, "greeting", rows[0].Block)
			assert.Equal(t, "qps", rows[0].Edition)
			assert.Equal(t, "flow:edit", rows[0].Origin)
			assert.Equal(t, "edit", rows[0].ActorName)
			assert.Equal(t, sourceRevisions(t, a, recipe)["greeting"][0], rows[0].Basis)
		})
	}
}
