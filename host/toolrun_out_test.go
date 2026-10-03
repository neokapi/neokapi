package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

const execPage = `<html><body><p>Visit the <a href="https://example.com">shop</a> today.</p><p>The shop opens at nine.</p></body></html>` + "\n"

// upperRun is `kapi exec case-transform <file> --mode upper` over file, in
// place.
func upperRun(a *App, file string) ToolRunConfig {
	return ToolRunConfig{
		ToolName:      "case-transform",
		Files:         []string{file},
		DefaultLayout: true,
		NewTool: func() (tool.Tool, error) {
			return a.ToolReg.NewToolWithConfig(registry.ToolID("case-transform"), map[string]any{"mode": "upper", "applySource": true}, "")
		},
	}
}

func TestToolRun_PrintsTheChangeSetKapiApplyAppliesToTheSameBytes(t *testing.T) {
	isolateKapiEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	printed := filepath.Join(dir, "printed.html")
	ran := filepath.Join(dir, "ran.html")
	for _, p := range []string{printed, ran} {
		require.NoError(t, os.WriteFile(p, []byte(execPage), 0o644))
	}
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	cmd := NewEnvCommand(context.Background(), "exec")
	cmd.Flags().Bool(printOpsFlag, false, "")
	require.NoError(t, cmd.Flags().Set(printOpsFlag, "true"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, a.WithPrintedOps(cmd, func() error { return a.RunToolOnFiles(t.Context(), upperRun(a, printed)) }))
	got, err := os.ReadFile(printed)
	require.NoError(t, err)
	assert.Equal(t, execPage, string(got), "a printing run writes nothing")

	var set change.Set
	require.NoError(t, json.Unmarshal(out.Bytes(), &set), "%s", out.String())
	require.Len(t, set.Ops, 2, "one operation per paragraph the run changed")
	for _, op := range set.Ops {
		assert.Equal(t, "printed.html", op.At.Doc)
		assert.True(t, op.At.Edition.IsZero(), "the run changed the document's own edition")
		assert.NotEqual(t, change.AnyRevision, op.IfMatch)
	}

	svc, err := a.ChangeService(t.Context(), ChangeServiceOptions{Root: dir, SourceLocale: "en"})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), set, change.Actor{Kind: change.ActorPerson, Name: "tester"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	require.NoError(t, a.RunToolOnFiles(t.Context(), upperRun(a, ran)))
	want, err := os.ReadFile(ran)
	require.NoError(t, err)
	applied, err := os.ReadFile(printed)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(applied))
	assert.Contains(t, string(want), `<a href="https://example.com">SHOP</a>`)
}

// pageProject writes a project whose collection holds one HTML page at
// docs/<name>, and returns its recipe.
func pageProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", name), []byte(execPage), 0o644))
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:     project.CurrentVersion,
		Name:        "exec",
		Defaults:    project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{{Name: "docs", Path: "docs/*.html"}},
	}))
	return recipe
}

func TestToolRun_InAProjectCommitsAndRecordsAsKapiApplyDoes(t *testing.T) {
	// kapi exec over a page of a project, from a directory inside it: the
	// page is the project's document, named as kapi apply names it, locked
	// where kapi apply locks it, and what the run changed is recorded.
	isolateKapiEnv(t)
	printing, running := pageProject(t, "page.html"), pageProject(t, "page.html")
	a := &App{}
	a.InitRegistries()

	t.Chdir(filepath.Join(filepath.Dir(printing), "docs"))
	cfg := upperRun(a, "page.html")
	cfg.Project = printing
	cmd := NewEnvCommand(context.Background(), "exec")
	cmd.Flags().Bool(printOpsFlag, false, "")
	require.NoError(t, cmd.Flags().Set(printOpsFlag, "true"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, a.WithPrintedOps(cmd, func() error { return a.RunToolOnFiles(t.Context(), cfg) }))
	var set change.Set
	require.NoError(t, json.Unmarshal(out.Bytes(), &set), "%s", out.String())
	require.Len(t, set.Ops, 2)
	for _, op := range set.Ops {
		assert.Equal(t, "docs/page.html", op.At.Doc, "the project names the page from its root")
	}
	svc, err := a.ChangeService(t.Context(), ChangeServiceOptions{Project: printing})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), set, change.Actor{Kind: change.ActorPerson, Name: "tester"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	root := filepath.Dir(running)
	t.Chdir(filepath.Join(root, "docs"))
	cfg = upperRun(a, "page.html")
	cfg.Project = running
	require.NoError(t, a.RunToolOnFiles(t.Context(), cfg))
	ran, err := os.ReadFile(filepath.Join(root, "docs", "page.html"))
	require.NoError(t, err)
	applied, err := os.ReadFile(filepath.Join(filepath.Dir(printing), "docs", "page.html"))
	require.NoError(t, err)
	assert.Equal(t, string(ran), string(applied), "kapi apply of what was printed writes the run's bytes")

	locks, err := os.ReadDir(filepath.Join(project.LayoutAt(root).WorkDir(), "locks"))
	require.NoError(t, err, "the run locks the page where kapi apply in the project locks it")
	assert.NotEmpty(t, locks)
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	rows, err := db.History().Document(t.Context(), a.documentIndexOrEmpty(t.Context(), root).Key("docs/page.html"))
	require.NoError(t, err)
	require.NotEmpty(t, rows, "the run records what it changed")
	assert.Equal(t, string(change.ActorTool), rows[0].Actor)
	assert.Equal(t, "case-transform", rows[0].ActorName)
}

// saveBeforeWrite is a tool that saves data at path as it sees the first part.
type saveBeforeWrite struct {
	tool.BaseTool
	path, data string
	done       bool
}

func (s *saveBeforeWrite) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		if !s.done {
			s.done = true
			_ = os.WriteFile(s.path, []byte(s.data), 0o644)
		}
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestToolRun_KeepsAFileSavedWhileTheRunWorked(t *testing.T) {
	isolateKapiEnv(t)
	dir := t.TempDir()
	page := filepath.Join(dir, "page.html")
	require.NoError(t, os.WriteFile(page, []byte(execPage), 0o644))
	saved := `<html><body><p>Saved by a person.</p></body></html>` + "\n"
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	err := a.RunToolOnFiles(t.Context(), ToolRunConfig{
		ToolName:      "save",
		Files:         []string{page},
		DefaultLayout: true,
		NewTool: func() (tool.Tool, error) {
			return &saveBeforeWrite{ToolName: "save", path: page, data: saved}, nil
		},
	})
	require.Error(t, err)
	assert.True(t, flow.IsMoved(err), "%v", err)
	got, rerr := os.ReadFile(page)
	require.NoError(t, rerr)
	assert.Equal(t, saved, string(got))
}
