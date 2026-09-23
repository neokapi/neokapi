package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fresh copy of samples/northsea is what a reader clones: a recipe binding a
// voice by name, the voice profile and the vocabulary in the checkout under
// context/, and a store that has never held either. kapi says the bound voice
// has not been brought to this machine and names the command that brings it,
// and answers from the store once it has.
//
// This is the sample's README as a test. A sample that stops teaching the
// import, or a notice that stops naming what it names, fails here.

func TestNorthsea_ContextArrivesThroughAnImport(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := northseaCopy(t)
	// The import is a person's decision about the project, and this test is
	// one person following the README.
	t.Setenv("KAPI_ACTOR", "person")

	// Before the import: the recipe binds a voice the store does not hold.
	before, err := runContextE(t, a, "docs/berths.md", "-p", recipe)
	require.NoError(t, err, before)
	assert.Contains(t, before, `binds voice profile "northsea"`)
	assert.Contains(t, before, "has not been imported or restored here")
	assert.Contains(t, before, ContextImportCommandText,
		"the answer names the command that brings the voice in")

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
