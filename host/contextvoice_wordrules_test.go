package host

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// The word rules a voice file carries land in the terms store with what each
// says: two rules naming one replacement keep their own advisory marking, and
// a rule scoped to prose keeps its scope. Reading the file again changes
// nothing.
func TestLandWordRules_KeepsEachRulesMarkingAndScope(t *testing.T) {
	ctx := context.Background()
	a := &App{SourceLang: "en"}
	store := terms.NewInMemoryStore()
	rules := []coreprofile.TermRule{
		{Term: "utilize", Replacement: "use"},
		{Term: "leverage", Replacement: "use", Advisory: true},
		{Term: "synergy", Replacement: "cooperation", Scope: coreprofile.ScopeProse},
	}
	require.NoError(t, a.landWordRules(ctx, store, rules))
	concepts, err := store.Concepts(ctx)
	require.NoError(t, err)

	type outcome struct {
		replacement, scope string
		fails              bool
	}
	got := map[string]outcome{}
	for _, r := range terms.SourceWordRules(concepts, "en") {
		got[r.Term] = outcome{r.Replacement, r.Scope, !r.Advisory}
	}
	assert.Equal(t, map[string]outcome{
		"utilize":  {"use", "", true},
		"leverage": {"use", "", false},
		"synergy":  {"cooperation", coreprofile.ScopeProse, true},
	}, got)

	require.NoError(t, a.landWordRules(ctx, store, rules))
	again, err := store.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, again, len(concepts), "a second read opens no concept")
}
