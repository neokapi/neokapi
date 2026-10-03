package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
)

// TestCheck_AFindingWithAReplacementCarriesItsFix pins the loop `kapi check
// --json | jq … | kapi apply -` runs: a term finding whose rule names a
// replacement carries the replace_text that applies it, guarded by the
// revision the check read, and that operation lands through the change
// service as it is, through JSON both ways.
func TestCheck_AFindingWithAReplacementCarriesItsFix(t *testing.T) {
	f := newCommitFixture(t)
	guide := filepath.Join(f.root, "docs", "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\nWe utilize the widget every day.\n"), 0o644))

	report, err := f.app.ComputeCheck(f.command(t), []string{guide})
	require.NoError(t, err)
	var found *check.Diagnostic
	for i, d := range report.Findings {
		if d.Rule == "terms.vocabulary" && d.Location.Snippet == "utilize" {
			found = &report.Findings[i]
		}
	}
	require.NotNil(t, found, "the check finds the failing term: %+v", report.Findings)
	require.NotNil(t, found.Fix, "a finding whose rule names a replacement carries its fix")
	assert.Equal(t, change.KindReplaceText, found.Fix.Kind)

	raw, err := json.Marshal(report)
	require.NoError(t, err)
	var wire struct {
		Findings []struct {
			Rule string          `json:"rule"`
			Fix  json.RawMessage `json:"fix"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(raw, &wire))
	var fix json.RawMessage
	for _, d := range wire.Findings {
		if d.Rule == "terms.vocabulary" && len(d.Fix) > 0 {
			fix = d.Fix
		}
	}
	require.NotEmpty(t, fix)
	assert.Contains(t, string(fix), `"op":"replace_text"`)
	assert.Contains(t, string(fix), `"text":"use"`)

	set, err := change.Decode(bytes.NewReader([]byte(`{"ops": [` + string(fix) + `]}`)))
	require.NoError(t, err, "the fix decodes as a change set's operation")
	svc, err := f.app.ChangeService(t.Context(), ChangeServiceOptions{Project: f.recipe, Origin: "test"})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), set, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got, err := os.ReadFile(guide)
	require.NoError(t, err)
	assert.Equal(t, "# Guide\n\nWe use the widget every day.\n", string(got))
}
