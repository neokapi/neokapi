package host_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContextSearchSourcesFor_StandalonePaths is finding 8: the CLI verb and the
// context_search MCP tool now assemble sources through one host method. A
// standalone terms/memory path (the MCP override) binds those stores; the
// returned cleanup releases them.
func TestContextSearchSourcesFor_StandalonePaths(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1") // no dogfood project resolution from the repo tree

	dir := t.TempDir()
	termsPath := filepath.Join(dir, "terms.db")
	memoryPath := filepath.Join(dir, "memory.db")

	a := &host.App{}
	cmd := host.NewEnvCommand(context.Background(), "context-search-test")

	src, cleanup := a.ContextSearchSourcesFor(cmd, termsPath, memoryPath)
	defer cleanup()

	require.NotNil(t, src.Terms, "a standalone terms path binds the terms store")
	require.NoError(t, src.TermsErr)
	require.NotNil(t, src.Memory, "a standalone memory path binds the memory store")
	require.NoError(t, src.MemoryErr)
	assert.Equal(t, host.ScopeProject, src.Scope)
	assert.True(t, src.TermsStandalone, "a store named by path is not the project's own")
	assert.Nil(t, src.Graph, "no project, so no graph to count uses from")
	require.NoError(t, src.GraphErr)
}
