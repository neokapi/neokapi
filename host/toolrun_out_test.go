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
