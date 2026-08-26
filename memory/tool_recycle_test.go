package memory_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTool pushes one part through a tool and returns the result.
func runTool(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	result := <-out
	require.NotNil(t, result)
	return result
}

// TestTMLeverageToolPlaceholderIntegrity pins the invariant that a content-aware
// content-memory fill must carry its source's inline codes, driving the single
// framework recycle tool through the memory-backed provider — the same pairing
// the CLI and the platform now share.
//
// The plain tier matches on flattened text — inline codes contribute no
// characters — so a legacy entry recorded without placeholders is an "exact"
// match for a source that has them. Applying its runs used to drop the
// placeholder and publish a sentence with a hole in it. Now such a match is
// recorded as a reviewable candidate but never committed as the target.
func TestTMLeverageToolPlaceholderIntegrity(t *testing.T) {
	countSource := []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{documentedCount}", Equiv: "documentedCount"}},
		{Text: &model.TextRun{Text: " documented formats"}},
	}

	tests := []struct {
		name string
		// entry variants keyed by locale — the TM content the block matches.
		en, nb []model.Run
		// source runs of the block being leveraged.
		source   []model.Run
		wantFill string
		wantPh   bool
	}{
		{
			name:     "entry keeping the placeholder fills",
			en:       countSource,
			nb:       []model.Run{{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{documentedCount}", Equiv: "documentedCount"}}, {Text: &model.TextRun{Text: " dokumenterte formater"}}},
			source:   countSource,
			wantFill: " dokumenterte formater",
			wantPh:   true,
		},
		{
			name:   "entry stored without the placeholder is never applied",
			en:     []model.Run{{Text: &model.TextRun{Text: " documented formats"}}},
			nb:     []model.Run{{Text: &model.TextRun{Text: " dokumenterte formater"}}},
			source: countSource,
		},
		{
			// The entry's target carries paired markup the code-free source does
			// not. The structure-aware path rejects those runs (unbalanced against
			// the source), but the text path recovers the entry by its flattened
			// text and fills the plain translation — the source has no codes to
			// drop, and the target should mirror the source's structure, not the
			// entry's incidental markup.
			name:     "entry with extra codes recovers as flattened text",
			en:       []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			nb:       []model.Run{{PcOpen: &model.PcOpenRun{ID: "9"}}, {Text: &model.TextRun{Text: "Installer"}}, {PcClose: &model.PcCloseRun{ID: "9"}}},
			source:   []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			wantFill: "Installer",
		},
		{
			name:     "code-free entry against a code-free source still fills",
			en:       []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			nb:       []model.Run{{Text: &model.TextRun{Text: "Installer"}}},
			source:   []model.Run{{Text: &model.TextRun{Text: "Install"}}},
			wantFill: "Installer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tm := memory.NewInMemoryStore()
			require.NoError(t, tm.Add(context.Background(), memory.Entry{
				ID:       "e1",
				Variants: map[model.LocaleID][]model.Run{"en": tc.en, "nb": tc.nb},
			}))
			tl := tools.NewMemoryLeverageTool(&tools.MemoryLeverageConfig{
				SourceLocale:        "en",
				TargetLocale:        "nb",
				Memory:              leverage.NewProvider(tm),
				FuzzyThreshold:      70,
				FillTarget:          true,
				FillTargetThreshold: 0,
			})

			block := model.NewBlock("tu1", "")
			block.Source = tc.source
			result := runTool(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
			rb := result.Resource.(*model.Block)

			if tc.wantFill == "" {
				assert.False(t, rb.HasTarget("nb"),
					"a match whose inline codes do not line up with the source must not be applied")
				assert.NotEmpty(t, rb.AltTranslations(),
					"the rejected candidate stays on record for review")
				return
			}
			require.True(t, rb.HasTarget("nb"))
			assert.Equal(t, tc.wantFill, rb.TargetText("nb"))
			if tc.wantPh {
				runs := rb.TargetRuns("nb")
				require.NotEmpty(t, runs)
				require.NotNil(t, runs[0].Ph, "placeholder run preserved")
				assert.Equal(t, "documentedCount", runs[0].Ph.Equiv)
			}
		})
	}
}
