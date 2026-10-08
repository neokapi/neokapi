package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kapi merge's "not merged:" lines count as refusals, by the contract code
// they name.
func TestPairedMergeRefusalsAreCounted(t *testing.T) {
	o := &pairedObserver{result: &PairedAgentResult{}}
	o.countRefusals([]string{
		"merge: nb.xliff: block welcome/p@nb not merged: guard: expected <x id=\"1\"/>, found none",
		"merge: nb.xliff: block intro/p@nb not merged: the file holds no such edition",
	})
	assert.Equal(t, map[string]int{"merge:guard": 1, "merge:refused": 1}, o.result.Refusals)
}

// A file written in the workspace root, and a write to the context store, are
// recorded from the call that made them, so a file deleted before the end
// still counts.
func TestPairedRootAndContextWrites(t *testing.T) {
	shell := func(c string) map[string]any { return map[string]any{"command": c} }
	ws := "/cell/workspace"
	assert.Equal(t, []string{"changes.json"}, pairedRootWrites("Bash", shell("cat > changes.json <<'EOF'\n{}\nEOF"), ws))
	assert.Equal(t, []string{"edit.json"}, pairedRootWrites("Write", map[string]any{"file_path": "/cell/workspace/edit.json"}, ws))
	assert.Empty(t, pairedRootWrites("Bash", shell("kapi check docs 2>&1 > /dev/null"), ws))
	assert.Empty(t, pairedRootWrites("Bash", shell("cat > \"$TMPDIR/changes.json\""), ws))
	assert.Empty(t, pairedRootWrites("Write", map[string]any{"file_path": "/cell/workspace/docs/en/a.md"}, ws))
	assert.Empty(t, pairedRootWrites("Bash", shell("kapi apply changes.json | tee"), ws), "tee with no file writes none")

	assert.Equal(t, "kapi context note", pairedContextWriteOf("Bash", shell("kapi context note --term X --seen-in a.md")))
	assert.Equal(t, "kapi terms add", pairedContextWriteOf("shell", shell("kapi terms add portal")))
	assert.Equal(t, "mcp__kapi__context_note", pairedContextWriteOf("mcp__kapi__context_note", nil))
	assert.Equal(t, "kapi context review", pairedContextWriteOf("Bash", shell("kapi context review --keep 0n794e2gk7")))
	assert.Empty(t, pairedContextWriteOf("mcp__kapi__context_read", nil))
	assert.Empty(t, pairedContextWriteOf("Bash", shell("kapi context log --session this")))
}

// A file left directly in the workspace root fails the root's scope check.
func TestPairedRootScope(t *testing.T) {
	task := pairedTaskByID(t, "recover-stale-read")
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("{}"), 0o600))
	result, err := validatePairedTask(dir, task, pairedLanded())
	require.NoError(t, err)
	assert.True(t, pairedCriterionOf(t, result, "scope:/").Passed, "a host's dotfile is not the agent's")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "changes.json"), []byte("{}"), 0o600))
	result, err = validatePairedTask(dir, task, pairedLanded())
	require.NoError(t, err)
	root := pairedCriterionOf(t, result, "scope:/")
	assert.False(t, root.Passed)
	assert.Contains(t, root.Detail, "changes.json")
}

func pairedCriterionOf(t *testing.T, v PairedValidation, id string) PairedCriterionResult {
	t.Helper()
	for _, c := range v.Criteria {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no criterion %s", id)
	return PairedCriterionResult{}
}

// Four words of the source in a row fail a translation, a link's text
// included, where a whole English sentence was needed before.
func TestPairedMarkdownTranslatedRefusesAFourWordRun(t *testing.T) {
	source := "Install the [Harbor Help app](https://harbor.example/app) and sign in.\n"
	passed, detail := pairedMarkdownTranslated(source, "Installer [Harbor Help app](https://harbor.example/app) og logg inn.\n")
	assert.True(t, passed, detail)
	passed, detail = pairedMarkdownTranslated(source, "Installer [the Harbor Help app](https://harbor.example/app) og logg inn.\n")
	assert.False(t, passed)
	assert.True(t, strings.HasPrefix(detail, "source text remains: "), detail)
}
