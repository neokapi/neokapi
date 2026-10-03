package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The run command in docs/internals/evals.md stages the study: a first stage
// of eight sessions chosen by name, then the rest of the grid. The first
// stage must be grid sessions that put the plural task, both hosts and every
// arm in front of a live agent; the second stage's ceiling must be the grid,
// since the ceiling counts the first stage's attempts too. The seeded
// schedule's own first eight are all one host on two other tasks.
func TestPairedDocumentedStagesCoverTheGrid(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "internals", "evals.md"))
	require.NoError(t, err)
	_, running, ok := strings.Cut(string(doc), "### Running it")
	require.True(t, ok, "evals.md has a Running it section")
	_, block, ok := strings.Cut(running, "```bash\n")
	require.True(t, ok)
	block, _, _ = strings.Cut(block, "```")

	m, err := readPairedManifest("testdata/paired-study.json")
	require.NoError(t, err)
	pilot := pairedSchedule(m, "pilot")

	selection := regexp.MustCompile(`-paired-sessions ([^"\s]+)`).FindAllStringSubmatch(block, -1)
	require.Len(t, selection, 1, "one stage is chosen by name")
	first, err := selectPairedSessions(pilot, selection[0][1])
	require.NoError(t, err, "the first stage names sessions of the grid")
	require.Len(t, first, 8)
	cells := map[string]bool{}
	for _, s := range first {
		assert.Equal(t, "edit-plural-branch", s.Task)
		cells[s.Agent.Host+"/"+s.Condition] = true
	}
	assert.Len(t, cells, len(m.Agents)*len(m.Conditions), "both hosts, every arm")

	ceilings := regexp.MustCompile(`PAIRED_EVAL_MAX_ATTEMPTS=(\d+)`).FindAllStringSubmatch(block, -1)
	require.Len(t, ceilings, 2, "two stages")
	stage1, _ := strconv.Atoi(ceilings[0][1])
	stage2, _ := strconv.Atoi(ceilings[1][1])
	assert.Equal(t, len(first), stage1, "the first stage starts its eight sessions and no more")
	assert.Equal(t, len(pilot), stage2, "the second stage's ceiling is the grid: the first stage's attempts count")

	// Without the selection, the first stage would be the schedule's head.
	head := map[string]bool{}
	for _, s := range pilot[:8] {
		head[s.Agent.Host+"/"+s.Task] = true
	}
	assert.NotContains(t, head, "codex/edit-plural-branch")
}
