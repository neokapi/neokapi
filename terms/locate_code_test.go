package terms_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// An occurrence is where a reader sees a term written, and a term inside inline
// code or a quoted command is written there too. The bound store and the
// caller's rules locate the same occurrences.
func TestLocate_CodeSpansHoldOccurrences(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{
		ID: "check",
		Terms: []terms.Term{
			{Text: "check", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "kontroll", Locale: "nb", Status: model.TermPreferred},
		},
	}))

	text := "Run `kapi check` or 'kapi check --staged', then check the file."
	var want []int
	for i := 0; ; {
		j := strings.Index(text[i:], "check")
		if j < 0 {
			break
		}
		want = append(want, i+j)
		i += j + len("check")
	}
	require.Len(t, want, 3)

	for name, req := range map[string]terms.LocateRequest{
		"the store": {Text: text, Store: store, Locale: model.LocaleEnglish},
		"the rules": {Text: text, RuleSets: []profile.TermRuleSet{{Rules: []profile.TermRule{{Term: "check", Replacement: "kontroll"}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := terms.Locate(ctx, req)
			require.NoError(t, err)
			starts := make([]int, 0, len(got))
			for _, o := range got {
				starts = append(starts, o.Start)
			}
			assert.ElementsMatch(t, want, starts, "the occurrences in code, in the quoted command and in the prose")
		})
	}
}
