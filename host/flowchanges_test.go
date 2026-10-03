package host

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
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
	return newFlowProjectWith(t, materialize, nil)
}

// newFlowProjectWith is newFlowProject with the recipe changed by edit first.
func newFlowProjectWith(t *testing.T, materialize string, edit func(*project.KapiProject)) (*App, *EnvCommand, string) {
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
	if edit != nil {
		edit(proj)
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
	set, _ := printRunNotes(t, a, cmd, run)
	return set
}

// printRunNotes is printRun, also returning what the run noted on standard
// error.
func printRunNotes(t *testing.T, a *App, cmd *EnvCommand, run func() error) (change.Set, string) {
	t.Helper()
	var out, notes bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&notes)
	require.NoError(t, cmd.Flags().Set(printOpsFlag, "true"))
	require.NoError(t, a.WithPrintedOps(cmd, run))
	require.NoError(t, cmd.Flags().Set(printOpsFlag, "false"))
	var set change.Set
	require.NoError(t, json.Unmarshal(out.Bytes(), &set), "the output is a change set: %s", out.String())
	return set, notes.String()
}

// applyPrinted applies a printed change set to the project at recipe through
// the change service, as kapi apply does.
func applyPrinted(t *testing.T, a *App, recipe string, set change.Set) {
	t.Helper()
	if len(set.Ops) == 0 {
		return
	}
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, SourceLocale: "en", Origin: "apply"})
	require.NoError(t, err)
	res, err := svc.Apply(context.Background(), set, change.Actor{Kind: change.ActorPerson, Name: "tester"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// readTarget reads the project's qps file, "" when there is none.
func readTarget(t *testing.T, recipe string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "qps.json"))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func TestFlowRun_PrintsWhatKapiApplyWritesToTheRunsBytes(t *testing.T) {
	// After a first pass, the twins' files change the same way. One prints
	// its next pass and applies what it printed; the other runs the pass.
	// A file whose operations are printed ends as the same bytes in both; a
	// file kapi apply would write otherwise is named and left out.
	writeSource := func(body string) func(t *testing.T, root string) {
		return func(t *testing.T, root string) {
			require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"), []byte(body), 0o644))
		}
	}
	cases := []struct {
		name    string
		prepare func(t *testing.T, root string)
		printed bool
	}{
		{
			name:    "a source edit to a block the target holds",
			prepare: writeSource(`{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n"),
			printed: true,
		},
		{
			name:    "a block the source gained",
			prepare: writeSource(`{"greeting": "Hello world", "farewell": "Goodbye now", "thanks": "Thank you", "welcome": "Welcome"}` + "\n"),
		},
		{
			name: "a target holding an entry of its own",
			prepare: func(t *testing.T, root string) {
				target := filepath.Join(root, "src", "qps.json")
				data, err := os.ReadFile(target)
				require.NoError(t, err)
				var catalog map[string]any
				require.NoError(t, json.Unmarshal(data, &catalog))
				catalog["extra"] = "Bare her"
				edited, err := json.Marshal(catalog)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(target, edited, 0o644))
				writeSource(`{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n")(t, root)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
			b, bcmd, brecipe := newFlowProject(t, project.MaterializeManual)
			runOnePass(t, a, cmd, recipe)
			runOnePass(t, b, bcmd, brecipe)
			tc.prepare(t, filepath.Dir(recipe))
			tc.prepare(t, filepath.Dir(brecipe))
			before := readTarget(t, recipe)

			set, notes := printRunNotes(t, a, cmd, func() error { return a.ExecuteUp(cmd, recipe) })
			assert.Equal(t, before, readTarget(t, recipe), "a printing run writes nothing")
			applyPrinted(t, a, recipe, set)
			runOnePass(t, b, bcmd, brecipe)
			ran := readTarget(t, brecipe)
			require.NotEqual(t, before, ran, "the pass writes the file")

			if tc.printed {
				require.NotEmpty(t, set.Ops)
				assert.Equal(t, ran, readTarget(t, recipe), "kapi apply of what was printed writes the run's bytes")
				assert.NotContains(t, notes, "src/qps.json")
				return
			}
			assert.Empty(t, set.Ops)
			assert.Contains(t, notes, "src/qps.json: the run writes the file whole")
			assert.Equal(t, before, readTarget(t, recipe), "nothing was printed, so nothing was applied")
		})
	}
}

func TestFlowRun_PrintsWhatDeliveryWouldCommit(t *testing.T) {
	cases := []struct {
		name      string
		shipGate  gate.Gate
		delivered bool
	}{
		{name: "a locale with no gate to clear", delivered: true},
		{name: "a locale short of its ship gate", shipGate: gate.Gate{"established": {Pct: 50}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edit := func(p *project.KapiProject) { p.ShipGate = tc.shipGate }
			a, cmd, recipe := newFlowProjectWith(t, project.MaterializeOnConverge, edit)
			b, bcmd, brecipe := newFlowProjectWith(t, project.MaterializeOnConverge, edit)

			set, notes := printRunNotes(t, a, cmd, func() error { return a.ExecuteUp(cmd, recipe) })
			assert.Empty(t, readTarget(t, recipe), "a printing run delivers nothing")
			assert.Empty(t, flowHistory(t, a, filepath.Dir(recipe)), "and records nothing")
			applyPrinted(t, a, recipe, set)
			runOnePass(t, b, bcmd, brecipe)

			if tc.delivered {
				require.Len(t, set.Ops, 3)
				require.NotEmpty(t, readTarget(t, brecipe))
				assert.Equal(t, readTarget(t, brecipe), readTarget(t, recipe), "kapi apply of what was printed is the delivery")
				return
			}
			assert.Empty(t, set.Ops, "a parked locale's drafts are not delivered, so none is printed")
			assert.Contains(t, notes, "qps: short of its ship gate")
			assert.Empty(t, readTarget(t, brecipe), "the run delivered nothing either")
		})
	}
}

func TestFlowRun_APrintingRunThatWritesNoFilePrintsNothing(t *testing.T) {
	// In a project, a built-in flow run without -o keeps what it produces in
	// the project's store. Printing it prints nothing, and writes nothing
	// there either.
	a, _, recipe := newFlowProject(t, project.MaterializeManual)
	cmd := NewEnvCommand(context.Background(), "pseudo-translate")
	fs := cmd.Flags()
	for _, name := range []string{"target-lang", "source-lang", "output", "encoding", "trace", "format"} {
		fs.String(name, "", "")
	}
	fs.StringSlice("input", nil, "")
	fs.Int("concurrency", 0, "")
	fs.Bool("explain", false, "")
	fs.Bool(printOpsFlag, false, "")
	require.NoError(t, fs.Set("input", filepath.Join(filepath.Dir(recipe), "src", "en.json")))
	require.NoError(t, fs.Set("target-lang", "qps"))
	a.TargetLang = "qps"

	set, notes := printRunNotes(t, a, cmd, func() error {
		return a.RunFromProject(cmd, "pseudo-translate", recipe, RunCmdOptions{Builtin: true})
	})
	assert.Empty(t, set.Ops, "the run writes no file, so it has no change to print")
	assert.Contains(t, notes, "src/en.json: in a project this run writes no file without -o")
	assert.Empty(t, readTarget(t, recipe))
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

// A printed change set states how the run's tool produced each translation,
// and kapi apply records the change as its applier's edit: it says so in one
// line, and the block history names the person, not the tool.
func TestFlowRun_APrintedSetStatesItsToolAndKapiApplyRecordsTheApplier(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	set := printRun(t, a, cmd, func() error { return a.ExecuteUp(cmd, recipe) })
	require.Len(t, set.Ops, 3)
	var tool string
	for _, op := range set.Ops {
		body, ok := op.Body.(*change.SetContent)
		require.True(t, ok)
		require.NotNil(t, body.Origin, "a printed translation states how it was produced")
		assert.NotEmpty(t, body.Origin.Tool)
		tool = body.Origin.Tool
	}

	write := func(set change.Set) string {
		raw, err := json.Marshal(set)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "change.json")
		require.NoError(t, os.WriteFile(path, raw, 0o600))
		return path
	}
	run := func(path string, opts ApplyOptions) (string, error) {
		apply := commitCommand(t, recipe)
		var stderr bytes.Buffer
		apply.SetOut(io.Discard)
		apply.SetErr(&stderr)
		err := a.RunApply(apply, path, opts)
		return stderr.String(), err
	}
	const note = "kapi apply records the change as yours"

	// A preview, and a change set refused as stale, record nothing and say
	// nothing about the origin.
	out, err := run(write(set), ApplyOptions{DryRun: true})
	require.NoError(t, err)
	assert.NotContains(t, out, note)
	stale := set
	stale.Ops = slices.Clone(set.Ops)
	stale.Ops[0].IfMatch = "r:0000000000000000"
	out, err = run(write(stale), ApplyOptions{})
	require.Error(t, err)
	assert.NotContains(t, out, note)

	out, err = run(write(set), ApplyOptions{})
	require.NoError(t, err)
	assert.Contains(t, out,
		"note: 3 operations state how "+tool+" produced their content; kapi apply records the change as yours, and that origin is not kept")
	assert.FileExists(t, filepath.Join(root, "src", "qps.json"))

	rows := flowHistory(t, a, root)
	require.NotEmpty(t, rows)
	for _, r := range rows {
		assert.Equal(t, string(change.ActorPerson), r.Actor, "the applier's edit, not the tool's")
	}
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
			assert.Equal(t, "set", rows[0].Producer.Tool, "the translation applied again keeps the stamp the tool left")
		})
	}
}

// runEditFlow runs tools over the project's source into its qps file, through
// the home and follower a flow of the project commits and records through.
func runEditFlow(t *testing.T, a *App, cmd *EnvCommand, recipe string, tools ...tool.Tool) error {
	t.Helper()
	root := filepath.Dir(recipe)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a.ProjectContext = project.NewProjectContext(proj, recipe)
	home, docs := a.flowDocuments(context.Background(), cmd, root)
	require.NotNil(t, docs)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: a.FormatReg, SourceLocale: "en", Home: home, Documents: docs})
	return runner.RunFile(context.Background(), "edit", tools,
		filepath.Join(root, "src", "en.json"), filepath.Join(root, "src", "qps.json"), "qps")
}

// lastWrite is the newest block history row of one block's qps edition.
func lastWrite(t *testing.T, a *App, root, block string) history.Row {
	t.Helper()
	for _, r := range flowHistory(t, a, root) {
		if r.Block == block && r.Edition == "qps" {
			return r
		}
	}
	t.Fatalf("no recorded write of %s", block)
	return history.Row{}
}

func TestFlowRun_RecordsTheSourceItTranslatedAsTheBasis(t *testing.T) {
	// The source is edited while the run translates it. The run commits its
	// target file, which nobody touched, and the record names the source the
	// run read as the translation's basis: the edit is drift against it.
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	read := sourceRevisions(t, a, recipe)
	err := runEditFlow(t, a, cmd, recipe,
		&writeDuringRun{ToolName: "edit-source", path: filepath.Join(root, "src", "en.json"),
			data: `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n"},
		&setTarget{ToolName: "set", key: "greeting", text: "Bonjour"})
	require.NoError(t, err)

	row := lastWrite(t, a, root, "greeting")
	assert.Equal(t, read["greeting"][0], row.Basis, "the basis is the source the run translated")
	assert.Equal(t, model.ComputeContentHash("Hello world"), row.ContentHash)
	assert.NotEqual(t, sourceRevisions(t, a, recipe)["greeting"][0], row.Basis, "the source has moved since")
	assert.Equal(t, 1, staleCount(t, a, recipe), "the edit made while the run worked is drift the loop owes a draft for")
}

func TestFlowRun_ReproducingAPersonsTranslationLeavesItTheirs(t *testing.T) {
	// A person writes a translation; a later run reproduces their wording (as
	// content memory recycling their edit does). The history still names the
	// person as the last writer of it.
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "qps.json"),
		[]byte(`{"greeting": "Hei", "farewell": "Ha det", "thanks": "Takk"}`+"\n"), 0o644))
	ctx := context.Background()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, SourceLocale: "en", Origin: "apply"})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: change.Ref{Doc: "src/en.json", Block: "greeting", Edition: model.EditionKey{Locale: "qps"}},
		IfMatch: sourceRevisions(t, a, recipe)["greeting"][1],
		Body:    &change.SetContent{Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
	}}}, change.Actor{Kind: change.ActorPerson, Name: "tester"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.Equal(t, string(change.ActorPerson), lastWrite(t, a, root, "greeting").Actor)

	require.NoError(t, runEditFlow(t, a, cmd, recipe, &setTarget{ToolName: "set", key: "greeting", text: "Bonjour"}))
	assert.Equal(t, string(change.ActorPerson), lastWrite(t, a, root, "greeting").Actor,
		"the run reproduced the person's wording, which stays theirs")
}

func TestFlowRun_DeliversADraftOfADocumentTheRunDoesNotFollow(t *testing.T) {
	// A gated pass drafts a file the run does not follow (here a conversion,
	// a draft in another format than its source), over a destination that
	// exists. Delivery commits it while the destination holds what the pass
	// found there.
	a, cmd, recipe := newFlowProject(t, project.MaterializeOnConverge)
	root := filepath.Dir(recipe)
	ctx := context.Background()
	dest := filepath.Join(root, "src", "qps.json")
	require.NoError(t, os.WriteFile(dest, []byte(`{"greeting": "Hei"}`+"\n"), 0o644))
	end, err := a.beginConvergeDrafts(recipe, root)
	require.NoError(t, err)
	defer end()
	draft, ok := a.draftPathFor("qps", dest)
	require.True(t, ok)

	home, docs := a.flowDocuments(ctx, cmd, root)
	require.NotNil(t, docs)
	run, err := docs.Open(ctx, flow.Document{Flow: "pseudo", InputPath: filepath.Join(root, "src", "en.json"),
		OutputPath: draft, TargetLocale: "qps", Format: "json", OutputFormat: "yaml"})
	require.NoError(t, err)
	p, err := home.Produce(ctx, draft, "", func(w io.Writer) error {
		_, werr := io.WriteString(w, "drafted\n")
		return werr
	})
	require.NoError(t, err)
	require.NoError(t, run.Commit(ctx, p))

	delivered, err := a.deliverDrafts(ctx, "qps")
	require.NoError(t, err, "the destination holds what the pass found there")
	assert.Equal(t, []string{dest}, delivered)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "drafted\n", string(got))

	// A destination saved while the run worked keeps the save.
	require.NoError(t, os.WriteFile(dest, []byte(`{"greeting": "Hei"}`+"\n"), 0o644))
	end()
	endAgain, err := a.beginConvergeDrafts(recipe, root)
	require.NoError(t, err)
	defer endAgain()
	home, docs = a.flowDocuments(ctx, cmd, root)
	run, err = docs.Open(ctx, flow.Document{Flow: "pseudo", InputPath: filepath.Join(root, "src", "en.json"),
		OutputPath: draft, TargetLocale: "qps", Format: "json", OutputFormat: "yaml"})
	require.NoError(t, err)
	p, err = home.Produce(ctx, draft, "", func(w io.Writer) error {
		_, werr := io.WriteString(w, "drafted\n")
		return werr
	})
	require.NoError(t, err)
	require.NoError(t, run.Commit(ctx, p))
	require.NoError(t, os.WriteFile(dest, []byte(`{"greeting": "Hei fra en person"}`+"\n"), 0o644))
	_, err = a.deliverDrafts(ctx, "qps")
	require.ErrorIs(t, err, filehome.ErrMoved)
	got, err = os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, `{"greeting": "Hei fra en person"}`+"\n", string(got))
}

func TestFlowRun_ADeliveredDraftRecordsTheSourceItsPassTranslated(t *testing.T) {
	// A gated run drafts in two passes and the source changes between them.
	// The delivered translation was made by the second pass, from the source
	// that pass read, and the record names that source as its basis.
	a, cmd, recipe := newFlowProject(t, project.MaterializeOnConverge)
	root := filepath.Dir(recipe)
	ctx := context.Background()
	src := filepath.Join(root, "src", "en.json")
	dest := filepath.Join(root, "src", "qps.json")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a.ProjectContext = project.NewProjectContext(proj, recipe)
	end, err := a.beginConvergeDrafts(recipe, root)
	require.NoError(t, err)
	defer end()
	draft, ok := a.draftPathFor("qps", dest)
	require.True(t, ok)

	pass := func(body string) {
		home, docs := a.flowDocuments(ctx, cmd, root)
		require.NotNil(t, docs)
		run, err := docs.Open(ctx, flow.Document{Flow: "pseudo", InputPath: src, OutputPath: draft, TargetLocale: "qps", Format: "json"})
		require.NoError(t, err)
		before, err := filehome.Digest(draft)
		require.NoError(t, err)
		p, err := home.Produce(ctx, draft, before, func(w io.Writer) error {
			_, werr := io.WriteString(w, body)
			return werr
		})
		require.NoError(t, err)
		require.NoError(t, run.Commit(ctx, p))
	}
	pass(`{"greeting": "Bonjour", "farewell": "Au revoir", "thanks": "Merci"}` + "\n")
	require.NoError(t, os.WriteFile(src,
		[]byte(`{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
	pass(`{"greeting": "Salut", "farewell": "Au revoir", "thanks": "Merci"}` + "\n")

	_, err = a.deliverDrafts(ctx, "qps")
	require.NoError(t, err)
	row := lastWrite(t, a, root, "greeting")
	assert.Equal(t, sourceRevisions(t, a, recipe)["greeting"][0], row.Basis,
		"the delivered translation was made from the source the second pass read")
	assert.Equal(t, model.ComputeContentHash("Hello there"), row.ContentHash)
}

// A flow's record says what its run did to each edition (design 10.4): the
// kind of operation the change comes to, and the tool that made it.
func TestFlowRun_RecordsTheOperationKindsAndTheTool(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	runOnePass(t, a, cmd, recipe)

	rows := flowHistory(t, a, root)
	require.Len(t, rows, 3)
	for _, r := range rows {
		assert.Equal(t, []string{string(change.KindSetContent)}, r.Ops, "the pass created the %s translation", r.Block)
		assert.Equal(t, "pseudo-translate", r.Tool, "the tool that wrote it, not only the flow")
	}

	// A source edit re-drafts one translation over the same codes.
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
	runOnePass(t, a, cmd, recipe)
	rows = flowHistory(t, a, root)
	require.Len(t, rows, 4)
	last := rows[0]
	assert.Equal(t, "greeting", last.Block)
	assert.Equal(t, []string{string(change.KindReplaceText)}, last.Ops, "the run's change to the translation is a text edit")
	assert.Equal(t, "pseudo-translate", last.Tool)
}
