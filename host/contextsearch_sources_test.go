package host_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI verb and the context_search MCP tool assemble sources through one
// host method. A standalone terms/memory path (the MCP override) binds those
// stores; the returned cleanup releases them.
func TestContextSearchSourcesFor_StandalonePaths(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1") // no dogfood project resolution from the repo tree

	dir := t.TempDir()
	termsPath := filepath.Join(dir, "terms.db")
	memoryPath := filepath.Join(dir, "memory.db")
	tb, err := terms.NewSQLiteStore(termsPath)
	require.NoError(t, err)
	require.NoError(t, tb.Close())
	tm, err := memory.NewSQLiteStore(memoryPath)
	require.NoError(t, err)
	require.NoError(t, tm.Close())

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

// A standalone path that names no file is reported as no store, and the
// search creates nothing there.
func TestContextSearchSourcesFor_MissingStandalonePaths(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")

	dir := t.TempDir()
	termsPath := filepath.Join(dir, "terms.db")
	memoryPath := filepath.Join(dir, "memory.db")

	src, cleanup := (&host.App{}).ContextSearchSourcesFor(host.NewEnvCommand(context.Background(), "context-search-test"), termsPath, memoryPath)
	defer cleanup()

	var noStore *host.NoStoreError
	require.ErrorAs(t, src.TermsErr, &noStore)
	require.ErrorAs(t, src.MemoryErr, &noStore)
	assert.Nil(t, src.Terms)
	assert.Nil(t, src.Memory)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a search creates no store file")
}
