package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// The plan lists every source an import would read with what it carries,
// approvals included, and writes nothing to the store: an import afterwards
// still reads all of it.
func TestPlanProjectContextImport_ListsWhatItWouldReadAndWritesNothing(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	ctx := context.Background()
	decided := state.UnitState{Unit: "greeting", Variant: model.Variant("nb"), Scope: "d-1",
		Status: model.TargetStatusEstablished, Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
		ContentHash: state.SourceHash("Hello"), TargetHash: state.TargetHash("Hei")}
	require.NoError(t, state.WriteCommitted(project.LayoutAt(root).Export().UnitStateDir(), []state.UnitState{decided}))

	plan, err := a.PlanProjectContextImport(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	require.True(t, plan.Reads())
	assert.Equal(t, 1, plan.Decisions)
	kinds := map[string]int{}
	for _, s := range plan.Sources {
		kinds[s.Kind]++
		if s.Kind == "terms" {
			assert.Positive(t, s.Concepts)
		}
	}
	assert.Equal(t, 1, kinds["terms"])
	assert.Equal(t, 2, kinds["voice"])
	var out bytes.Buffer
	require.NoError(t, plan.FormatText(&out))
	assert.Contains(t, out.String(), "term concept")
	assert.Contains(t, out.String(), "1 approval")

	again, err := a.PlanProjectContextImport(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, len(plan.Sources), len(again.Sources), "planning read nothing in")

	res, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Shown: &plan})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Decisions)
	assert.Equal(t, plan.Sources[0].Concepts, res.Concepts)
}

// A source whose bytes change between the listing and the read is refused, so
// the import reads only what the person confirmed.
func TestImportProjectContext_RefusesASourceThatChangedAfterItWasShown(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	ctx := context.Background()
	plan, err := a.PlanProjectContextImport(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)

	terms := filepath.Join(root, ".kapi", "terms.json")
	data, err := os.ReadFile(terms)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(terms, []byte(strings.Replace(string(data), "{", "{\n", 1)), 0o644))

	_, err = a.ImportProjectContext(ctx, recipe, ContextImportRequest{Shown: &plan})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed after it was listed")
}

// With no terminal and no --yes, the confirmation refuses rather than reading.
func TestConfirmContextImport_RefusesWithNoTerminal(t *testing.T) {
	a, _, recipe := writePortableProject(t)
	a.isTTY = func() bool { return false }
	plan, err := a.PlanProjectContextImport(context.Background(), recipe, ContextImportRequest{})
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd := NewEnvCommand(context.Background(), "store import")
	cmd.SetErr(&stderr)
	ok, err := a.ConfirmContextImport(cmd, plan)
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "--yes")
	assert.Contains(t, stderr.String(), "kapi would read these")

	a.AssumeYes = true
	ok, err = a.ConfirmContextImport(cmd, plan)
	require.NoError(t, err)
	assert.True(t, ok)
}
