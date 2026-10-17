package host

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// Content writing a rule's forms is evidence about the rule only while the
// rule holds: a suggestion or a rule outside its window is not counted.
func TestContextUsage_CountsOnlyRulesInForce(t *testing.T) {
	const project = workspace.ProjectKey("p")
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ended := at.AddDate(0, 0, -1)
	rec := func(id string, term string, established bool, to *time.Time) contextop.Record {
		rule := coreprofile.TermRule{Term: term, Replacement: "use"}
		status := contextop.StatusSuggested
		if established {
			status = contextop.StatusEstablished
		}
		return contextop.Record{
			ID: id, Project: project, Kind: contextop.KindObserve, Status: status, Established: established,
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule},
			Scope:   contextop.Scope{ValidTo: to},
		}
	}
	u := newContextUsage([]contextop.Record{
		rec("open", "utilise", false, nil),
		rec("ended", "leverage", false, &ended),
		rec("ended-rule", "employ", true, &ended),
	}, project, axis.Space{}, at)

	assert.Contains(t, u.byTerm, suggestionKey("utilise"))
	assert.NotContains(t, u.byTerm, suggestionKey("leverage"))
	assert.Empty(t, u.established)
}
