package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
)

func TestResolveParallelBlocks_ProjectOverride(t *testing.T) {
	app := &App{
		projectContext: &project.ProjectContext{
			ParallelBlocks: 10,
		},
	}
	// Project setting should win over flow defaults.
	assert.Equal(t, 10, app.resolveParallelBlocks("pseudo-translate"))
	assert.Equal(t, 10, app.resolveParallelBlocks("translate"))
}

func TestResolveParallelBlocks_NoProject(t *testing.T) {
	app := newTestApp()
	// Without project, falls back to flow defaults.
	assert.Equal(t, 0, app.resolveParallelBlocks("pseudo-translate"))
	assert.Greater(t, app.resolveParallelBlocks("translate"), 0)
}

func TestResolveParallelBlocks_ProjectZero(t *testing.T) {
	app := newTestApp()
	app.projectContext = &project.ProjectContext{
		ParallelBlocks: 0, // zero = use flow default
	}
	// Zero in project means "use flow default".
	assert.Greater(t, app.resolveParallelBlocks("translate"), 0)
}
