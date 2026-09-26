package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/ai/tools"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The draft's preferred forms join the candidate terms; its forbidden and
// competitor rules have no status to carry there and are left out.
func TestMergeContextScanTerms_FoldsTheDraftsPreferredForms(t *testing.T) {
	draft := (&coreprofile.VoiceProfile{Name: "Draft"}).Carry(tools.InferredTermsFrom, []coreprofile.TermRule{
		{Replacement: "workspace", Note: "where a team works"},
		{Term: "utilize", Replacement: "use"},
		{Term: "Globex", Competitor: true},
		{Replacement: "Workspace"},
	})
	extracted := []tools.TermEntry{{Term: "dashboard"}}

	got := mergeContextScanTerms(extracted, draft, "product")

	assert.Equal(t, []tools.TermEntry{
		{Term: "dashboard"},
		{Term: "workspace", Definition: "where a team works", Domain: "product"},
	}, got)
	assert.Equal(t, extracted, mergeContextScanTerms(extracted, nil, "product"))
}
