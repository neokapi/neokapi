package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fresh copy of samples/northsea is what a reader clones: a recipe binding a
// voice profile and a vocabulary that sit in the checkout under context/, and a
// store that has never held either. kapi names those files and the command that
// reads them, and answers from the store once it has.
//
// This is the sample's README as a test. A sample that stops teaching the
// import, or a notice that stops naming what it names, fails here.

func TestNorthsea_ContextArrivesThroughAnImport(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := northseaCopy(t)
	// The import is a person's decision about the project, and this test is
	// one person following the README.
	t.Setenv("KAPI_ACTOR", "person")

	// Before the import: the checkout carries the files and nothing is in force.
	before, err := runContextE(t, a, "docs/berths.md", "-p", recipe)
	require.NoError(t, err, before)
	assert.Contains(t, before, "context/terms.json")
	assert.Contains(t, before, "context/voice.yaml")
	assert.Contains(t, before, ContextImportCommandText,
		"the notice names the command that reads them")

	// The import: one command, and the sample is governed.
	out := runContext(t, a, "import", filepath.Join(root, "context"), "-p", recipe)
	assert.Contains(t, out, "concept")
	assert.Contains(t, out, "voice profile")

	// After it: the gate answers from the store, and the notice is done.
	after, err := runContextE(t, a, "docs/berths.md", "-p", recipe)
	require.NoError(t, err, after)
	assert.NotContains(t, after, ContextImportCommandText,
		"a store holding the project's context has nothing to read in")
	assert.Contains(t, after, "Northsea", "the voice the sample ships governs its documentation")

	search := runContext(t, a, "search", "mooring", "-p", recipe)
	assert.Contains(t, search, "berth", "the vocabulary the sample ships answers for the retired word")
}

// ContextImportCommandText is what every notice tells a reader to run.
const ContextImportCommandText = "kapi context import"
