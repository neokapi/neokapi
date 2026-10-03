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

	"github.com/neokapi/neokapi/core/project"
)

// statusOfProject runs kapi status on the project at recipe and returns what
// it prints, as JSON or as text.
func statusOfProject(t *testing.T, recipe string, asJSON bool) string {
	t.Helper()
	a := &App{}
	t.Cleanup(a.Shutdown)
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	if asJSON {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	return out
}

func historyNotPulledIn(t *testing.T, recipe string) bool {
	t.Helper()
	var parsed StatusOutput
	out := statusOfProject(t, recipe, true)
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), out)
	return parsed.HistoryNotPulled
}

// The loop's basis is in the project's context. A checkout of a project that
// shares its context, whose translations exist and whose block history
// records none, has not read that context, and kapi up and kapi status each
// say so in one line naming kapi context pull.
func TestUpAndStatus_SayWhenTheTranslationsHaveNoRecordedHistory(t *testing.T) {
	a, cmd, recipe := newFlowProjectWith(t, project.MaterializeManual, func(p *project.KapiProject) {
		p.Context = &project.ContextBackend{Backend: project.ContextBackendFile, Path: "shared-context"}
	})
	root := filepath.Dir(recipe)

	assert.False(t, historyNotPulledIn(t, recipe), "a project with no translations on disk has nothing to read")

	// A fresh clone: the translations the repository carries, and no record of
	// the source each was made from. One string has no translation yet.
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "qps.json"),
		[]byte(`{"greeting": "Ĥéļļö ŵöŕļð", "farewell": "Ĝööðƀýé ñöŵ"}`+"\n"), 0o644))
	assert.True(t, historyNotPulledIn(t, recipe))
	assert.Contains(t, statusOfProject(t, recipe, false), "run `kapi context pull`")

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	runOnePass(t, a, cmd, recipe)
	assert.Contains(t, stderr.String(), "note: "+HistoryNotPulledNote, "kapi up says it before the pass grades them")

	// The pass recorded the translation it wrote, so neither says it again.
	assert.False(t, historyNotPulledIn(t, recipe))
	stderr.Reset()
	runOnePass(t, a, cmd, recipe)
	assert.NotContains(t, stderr.String(), "kapi context pull")
}

// A project whose context stays on this machine has no record to pull: its
// passes record the translations they write, and neither command names a pull
// that would fail.
func TestUpAndStatus_SayNothingOfAPullForAProjectWithNoContextBackend(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "qps.json"),
		[]byte(`{"greeting": "Ĥéļļö ŵöŕļð", "farewell": "Ĝööðƀýé ñöŵ"}`+"\n"), 0o644))

	assert.False(t, historyNotPulledIn(t, recipe))
	assert.NotContains(t, statusOfProject(t, recipe, false), "kapi context pull")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	runOnePass(t, a, cmd, recipe)
	assert.NotContains(t, stderr.String(), "kapi context pull")
}
