package workspace_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/workspace"
)

func resetOp(id, project, before string) workspace.Op {
	return workspace.Op{
		ID: id, Project: workspace.ProjectKey(project), Kind: workspace.OpContextReset,
		Payload: []byte(`{"before":"` + before + `"}`),
	}
}

func op(id, project, kind string) workspace.Op {
	return workspace.Op{ID: id, Project: workspace.ProjectKey(project), Kind: kind}
}

func TestAResetSetsAsideWhatCameAfterItsPoint(t *testing.T) {
	ops := []workspace.Op{
		op("a1", "p", "context.observe"),
		op("a2", "p", "terms.write"),
		op("a3", "p", "context.keep"),
		op("a4", "p", "content.edit"),
		op("a5", "q", "context.observe"),
		resetOp("a6", "p", "a2"),
		op("a7", "p", "context.observe"),
	}
	aside := workspace.SetAside(ops)
	assert.Equal(t, map[string]bool{"a2": true, "a3": true}, aside,
		"the reset sets aside its own project's context from its point up to itself, and nothing else")
}

func TestAResetBeforeAResetRestoresWhatTheFirstSetAside(t *testing.T) {
	ops := []workspace.Op{
		op("a1", "p", "context.observe"),
		op("a2", "p", "context.keep"),
		resetOp("a3", "p", "a2"),
		op("a4", "p", "context.observe"),
		resetOp("a5", "p", "a3"),
	}
	aside := workspace.SetAside(ops)
	assert.Equal(t, map[string]bool{"a3": true, "a4": true}, aside,
		"the second reset sets the first aside, so what the first set aside applies again")
}

func TestOpIDAtSortsAtTheStartOfItsMoment(t *testing.T) {
	at := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	point := workspace.OpIDAt(at)
	assert.Len(t, point, workspace.OpIDLength)
	assert.LessOrEqual(t, point, workspace.NewOpID(at, ""))
	assert.Greater(t, point, workspace.NewOpID(at.Add(-time.Millisecond), ""))
}

func TestNoResetSetsNothingAside(t *testing.T) {
	assert.Empty(t, workspace.SetAside([]workspace.Op{op("a1", "p", "context.observe")}))
}
