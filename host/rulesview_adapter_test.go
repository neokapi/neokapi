package host

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/rulesview"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestRulesviewShapesMatch: the digest and decide types the desktop binds hold
// the fields core/rulesview's do, in the same order with the same JSON names,
// so a field added to one side and not the other fails here rather than going
// missing in a conversion.
func TestRulesviewShapesMatch(t *testing.T) {
	pairs := []struct{ host, model any }{
		{ContextDigest{}, rulesview.Digest{}},
		{DigestItem{}, rulesview.Item{}},
		{DigestUsage{}, rulesview.Usage{}},
		{DigestConflict{}, rulesview.Conflict{}},
		{DigestTheme{}, rulesview.Theme{}},
		{DigestGroup{}, rulesview.Group{}},
		{DigestDrift{}, rulesview.Drift{}},
		{DigestNumbers{}, rulesview.Numbers{}},
		{RuleDecision{}, rulesview.Decision{}},
		{RuleOption{}, rulesview.Option{}},
	}
	fields := func(v any) []string {
		typ := reflect.TypeOf(v)
		out := make([]string, typ.NumField())
		for i := range out {
			f := typ.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				tag = ""
			}
			out[i] = strings.ToLower(f.Name) + " " + tag
		}
		return out
	}
	for _, p := range pairs {
		assert.Equal(t, fields(p.model), fields(p.host), "%T and %T", p.host, p.model)
	}
}

// TestDigestConvertsWhole: a digest converted to the bound types encodes to
// the same JSON as the model's.
func TestDigestConvertsWhole(t *testing.T) {
	key := workspace.ProjectKey("acme/docs")
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	rule := &profile.TermRule{Replacement: "studio", Term: "business"}
	records := []contextop.Record{
		{ID: "03", Project: key, Kind: contextop.KindSignal, Target: "01", At: now.Add(-time.Hour), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalUsage, Preferred: 9, Rejected: 4}},
		{ID: "02", Project: key, Kind: contextop.KindSignal, Target: "01", At: now.Add(-30 * time.Hour), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalUsage, Preferred: 9, Rejected: 1}},
		{ID: "01", Project: key, Kind: contextop.KindObserve, At: now.Add(-48 * time.Hour), Status: contextop.StatusEstablished, Established: true,
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: rule}},
		{ID: "04", Project: key, Kind: contextop.KindObserve, At: now.Add(-2 * time.Hour), Status: contextop.StatusSuggested,
			Subject:  contextop.Subject{Kind: contextop.SubjectNote, Text: "Short sentences."},
			Evidence: []contextop.Evidence{{Path: "docs/a.md", Quote: "A short one."}}},
	}
	model := rulesview.BuildDigest(records, key, now.Add(-10*time.Hour), now, func(string) string { return "docs" })
	require.Len(t, model.Drift, 1)

	want, err := json.Marshal(model)
	require.NoError(t, err)
	got, err := json.Marshal(digestFrom(model))
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}
