//go:build js && wasm

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// entityRun is a character reference as the HTML reader keeps it.
func entityRun(id, data string) model.Run {
	return model.PhR(model.PlaceholderRun{ID: id, Type: "code:entity", Data: data})
}

// The lab's word-rule overlay finds a term beside a character reference the
// way `kapi check` does, and a term a reference spells an edge of carries the
// page's spelling.
func TestVoiceOverlayReadsCharacterReferences(t *testing.T) {
	cases := []struct {
		name    string
		runs    []model.Run
		spelled string
	}{
		{"before a reference", []model.Run{model.TextR("utilize"), entityRun("1", "&amp;"), model.TextR("more")}, ""},
		{"after a reference", []model.Run{model.TextR("word"), entityRun("1", "&nbsp;"), model.TextR("login")}, ""},
		{"between references", []model.Run{entityRun("1", "&amp;"), model.TextR("Acme"), entityRun("2", "&nbsp;")}, ""},
		{"a reference spells the last letter", []model.Run{model.TextR("logi"), entityRun("1", "&#110;")}, "login"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := model.RunsText(tc.runs)
			hits := profile.MatchTermRulesInRuns(profile.CarriedRuleSets(voiceProfile), source, tc.runs)
			ov := voiceOverlay(tc.runs, source)
			if !assert.NotNil(t, ov) || !assert.Len(t, ov.Spans, len(hits)) || !assert.Len(t, hits, 1) {
				return
			}
			assert.Equal(t, hits[0].Term, ov.Spans[0].Props["term"])
			assert.Equal(t, model.RangeAnchorForBytes(tc.runs, hits[0].Start, hits[0].End), ov.Spans[0].Range)
			assert.Equal(t, tc.spelled, ov.Spans[0].Props["spelled"])
		})
	}
}
