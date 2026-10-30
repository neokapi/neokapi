//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnect_DryRunThenTwice drives the built kapi through `kapi connect` in a
// folder with no recipe: a dry run lists what it would write and writes
// nothing, the first run writes the recipe, the rules files and Claude Code's
// files, and the second run keeps every one of them as it is.
func TestConnect_DryRunThenTwice(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	dry := kapiIn(t, dir, "connect", "--dry-run")
	assert.Contains(t, dry, "Claude Code   would connect · rules, MCP server, skill\n")
	assert.Regexp(t, `\n  kapi\.yaml +would create\n`, dry)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "--dry-run writes nothing")

	first := kapiIn(t, dir, "connect")
	assert.Contains(t, first, "Claude Code   connected · rules, MCP server, skill\n")
	for _, name := range []string{"kapi.yaml", "AGENTS.md", "CLAUDE.md", ".mcp.json", ".claude/skills/kapi/SKILL.md"} {
		_, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err, "kapi connect writes %s", name)
	}
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(agents), "## Writing rules")

	second := kapiIn(t, dir, "connect")
	for _, name := range []string{"kapi.yaml", "AGENTS.md", "CLAUDE.md", ".mcp.json", ".claude/skills/kapi"} {
		assert.Regexp(t, `\n  `+regexp.QuoteMeta(name)+` +kept\n`, second)
	}
	assert.NotRegexp(t, `created|updated`, second, "a second run writes nothing new")
}
