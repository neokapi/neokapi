package memory

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codedRuns is a source carrying paired markup: its plain key collides with the
// bare text while its structural key does not.
func codedRuns(text string) []model.Run {
	return []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "m0", Data: "**"}},
		{Text: &model.TextRun{Text: text}},
		{PcClose: &model.PcCloseRun{ID: "m0", Data: "**"}},
	}
}

// TestFullScoreEntries pins the set the ambiguity rule compares — the entries a
// lookup would return at score 1.0, which is what a caller resolving a
// disagreement (rather than avoiding it) has to see. Lookup itself cannot report
// them: once demoted they come back below full score or not at all.
func TestFullScoreEntries(t *testing.T) {
	tests := []struct {
		name    string
		seed    []Entry
		query   []model.Run
		wantIDs []string
	}{
		{
			name:    "nothing in the store",
			query:   []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			wantIDs: nil,
		},
		{
			name: "every entry that agrees on the text and the structure",
			seed: []Entry{
				textEntry("e-a", "Install", "Installer"),
				textEntry("e-b", "Install", "Innstaller"),
				textEntry("e-c", "Uninstall", "Avinstaller"),
			},
			query:   []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			wantIDs: []string{"e-a", "e-b"},
		},
		{
			name: "a plain hit whose inline codes differ is below full score",
			seed: []Entry{
				textEntry("e-plain", "Install", "Installer"),
				{ID: "e-coded", Variants: map[model.LocaleID][]model.Run{
					"en": codedRuns("Install"),
					"nb": codedRuns("Installer"),
				}},
			},
			query:   []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			wantIDs: []string{"e-plain"},
		},
		{
			name: "a coded query matches the coded entry, not the bare one",
			seed: []Entry{
				textEntry("e-plain", "Install", "Installer"),
				{ID: "e-coded", Variants: map[model.LocaleID][]model.Run{
					"en": codedRuns("Install"),
					"nb": codedRuns("Installer"),
				}},
			},
			query:   codedRuns("Install"),
			wantIDs: []string{"e-coded"},
		},
		{
			name:    "an empty source asks nothing",
			seed:    []Entry{textEntry("e-a", "Install", "Installer")},
			query:   nil,
			wantIDs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tm := newExactMemory(t)
			for _, e := range tc.seed {
				require.NoError(t, tm.Add(ctx, e))
			}

			got, err := tm.FullScoreEntries(ctx, tc.query, "en")
			require.NoError(t, err)
			ids := make([]string, 0, len(got))
			for _, e := range got {
				ids = append(ids, e.ID)
			}
			assert.Equal(t, tc.wantIDs, nilIfEmpty(ids))
		})
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
