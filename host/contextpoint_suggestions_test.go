package host

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The operation an answer names for a suggestion is one that holds where the
// answer stands, so keeping that id keeps the rule shown. A newer statement of
// the same word for another market, or one whose window has not opened, is
// passed over.
func TestContextSuggestions_NameTheOperationThatHoldsHere(t *testing.T) {
	space := axis.Space{"market": {Kind: axis.KindGeo, Groups: map[string][]string{"nordics": {"no", "se"}}}}
	rule := coreprofile.TermRule{Term: "utilise", Replacement: "use"}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	later := at.AddDate(0, 1, 0)
	record := func(id, market string, from *time.Time) contextop.Record {
		r := rule
		return contextop.Record{
			ID: id, Kind: contextop.KindObserve, Status: contextop.StatusSuggested,
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &r},
			Scope:   contextop.Scope{Coordinates: map[string]string{"market": market}, ValidFrom: from},
		}
	}
	// Newest first, as the log reads them.
	records := []contextop.Record{
		record("op-upcoming", "no", &later),
		record("op-germany", "de", nil),
		record("op-nordics", "nordics", nil),
	}

	got := contextSuggestions([]coreprofile.TermRule{rule}, records, map[string]string{"market": "no"}, space, at)
	require.Len(t, got, 1)
	assert.Equal(t, "op-nordics", got[0].Operation)
}
