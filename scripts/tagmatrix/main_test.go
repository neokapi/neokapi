package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/scripts/internal/buildconfigs"
)

func TestTriggering(t *testing.T) {
	assert.Equal(t, "host/go.sum", triggering([]string{"docs/a.md", "host/go.sum"}))
	assert.Equal(t, ".golangci.yml", triggering([]string{".golangci.yml"}))
	assert.Equal(t, "scripts/internal/buildconfigs/buildconfigs.go", triggering([]string{"scripts/internal/buildconfigs/buildconfigs.go"}))
	assert.Empty(t, triggering([]string{"core/model/block.go", "web/docs/a.md"}))
}

// The windows run of the root module compiles core/storage, which imports
// other framework packages and none of bowrain.
func TestAffectedFollowsImports(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	cfg := buildconfigs.All[buildconfigs.Index("windows")]
	r := run{cfg: cfg, patterns: []string{"./core/storage"}}
	env := cfg.Env

	hit, err := affected(root, root, env, r, []string{"bowrain/server/server.go", "web/docs/intro.md"})
	require.NoError(t, err)
	assert.Empty(t, hit, "bowrain and the docs are outside what the run compiles")

	hit, err = affected(root, root, env, r, []string{"core/storage/README.md"})
	require.NoError(t, err)
	assert.Equal(t, "core/storage/README.md", hit, "any file in a compiled package counts")

	hit, err = affected(root, root, env, r, []string{"core/storage/filelock/lock_windows.go"})
	require.NoError(t, err)
	assert.Equal(t, "core/storage/filelock/lock_windows.go", hit, "a dependency counts")
}
