package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rerun manifest names every task of the corpus: the WP5 seven and the
// variants a unique string cannot reach.
func TestPairedRerunManifestNamesEveryTask(t *testing.T) {
	m, err := readPairedManifest("testdata/paired-rerun.json")
	require.NoError(t, err)
	var ids []string
	for _, task := range pairedTasks() {
		ids = append(ids, task.ID)
	}
	assert.ElementsMatch(t, ids, m.Tasks)
	assert.Len(t, pairedSchedule(m, "pilot"), len(m.Agents)*len(m.Conditions)*len(ids)*m.Repetitions)
}
