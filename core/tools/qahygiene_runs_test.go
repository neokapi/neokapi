package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// The bilingual `qa.*` family shares the source-side hygiene family's defect
// class: its whitespace, adjacency and emptiness predicates read
// SourceText()/TargetText(), which drop inline-code runs and so change the shape
// of the content being judged. These tests hold the shape rules to run-aware
// boundaries on both sides of the pair.
//
// The rules that are genuinely about *characters* — charset encodability,
// forbidden/required characters, corruption, length ratios — deliberately keep
// reading the plain text. A sentinel is not a character the content contains, so
// feeding it to them would invent violations (U+FFFC is not ISO-8859-1
// encodable, for one).

func qaTx(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func qaPh(id, equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "jsx:var", Data: "{" + equiv + "}", Equiv: equiv}}
}

// qaRunCategories runs the qa tool over a source/target run pair and returns the
// finding categories.
func qaRunCategories(t *testing.T, sourceRuns, targetRuns []model.Run) []string {
	t.Helper()
	b := &model.Block{ID: "u1", Translatable: true, Source: sourceRuns, Properties: map[string]string{}}
	b.SetTargetRuns(model.LocaleFrench, targetRuns)

	tl := tools.NewQACheckTool(tools.NewQACheckConfig(model.LocaleFrench))

	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)

	cats := make([]string, 0, 4)
	for _, f := range qaFindings(blk) {
		cats = append(cats, f.Category)
	}
	return cats
}

func TestQACheck_RunAwareShapeRules(t *testing.T) {
	tests := []struct {
		name    string
		source  []model.Run
		target  []model.Run
		want    []string
		wantNot []string
	}{
		{
			name:    "matching leading placeholders are not a whitespace mismatch",
			source:  []model.Run{qaPh("1", "price"), qaTx(" each")},
			target:  []model.Run{qaPh("1", "price"), qaTx(" chacun")},
			wantNot: []string{"leading-whitespace", "trailing-whitespace", "empty-target"},
		},
		{
			name:    "matching trailing placeholders are not a whitespace mismatch",
			source:  []model.Run{qaTx("Total: "), qaPh("1", "total")},
			target:  []model.Run{qaTx("Total : "), qaPh("1", "total")},
			wantNot: []string{"trailing-whitespace", "leading-whitespace"},
		},
		{
			name:    "a placeholder-only target is not empty",
			source:  []model.Run{qaPh("1", "price")},
			target:  []model.Run{qaPh("1", "price")},
			wantNot: []string{"empty-target", "empty-source"},
		},
		{
			name:    "space, placeholder, space in the target is not a double space",
			source:  []model.Run{qaTx("Hello "), qaPh("1", "name"), qaTx(" world")},
			target:  []model.Run{qaTx("Bonjour "), qaPh("1", "name"), qaTx(" monde")},
			wantNot: []string{"double-spaces"},
		},
		{
			name:    "the same word either side of a target placeholder is not a doubled word",
			source:  []model.Run{qaTx("the "), qaPh("1", "x"), qaTx(" the end")},
			target:  []model.Run{qaTx("le "), qaPh("1", "x"), qaTx(" le fin")},
			wantNot: []string{"doubled-word"},
		},
		{
			// Under the old flattening both sides read "Bonjour", so the target
			// looked identical to a source it does not resemble.
			name:    "text equal but placeholders moved is not same-as-source",
			source:  []model.Run{qaPh("1", "x"), qaTx("Bonjour")},
			target:  []model.Run{qaTx("Bonjour"), qaPh("1", "x")},
			wantNot: []string{"target-same-as-source"},
		},

		// Genuine positives.
		{
			name:   "a real leading-whitespace mismatch still fires",
			source: []model.Run{qaPh("1", "price"), qaTx(" each")},
			target: []model.Run{qaTx(" "), qaPh("1", "price"), qaTx(" chacun")},
			want:   []string{"leading-whitespace"},
		},
		{
			name:   "a real trailing-whitespace mismatch still fires",
			source: []model.Run{qaTx("Total: "), qaPh("1", "total")},
			target: []model.Run{qaTx("Total : "), qaPh("1", "total"), qaTx(" ")},
			want:   []string{"trailing-whitespace"},
		},
		{
			name:   "a real double space in the target still fires",
			source: []model.Run{qaTx("Hello "), qaPh("1", "name")},
			target: []model.Run{qaTx("Bonjour  "), qaPh("1", "name")},
			want:   []string{"double-spaces"},
		},
		{
			name:   "a real doubled word in the target still fires",
			source: []model.Run{qaPh("1", "x"), qaTx(" the end")},
			target: []model.Run{qaPh("1", "x"), qaTx(" le le fin")},
			want:   []string{"doubled-word"},
		},
		{
			// Two adjacent text runs have nothing between them: the join is
			// real text, so a defect spanning it is real. Both rules read the
			// same shared scanners as the source-side family (check.HygieneOverlay
			// / check.DoubledWord), so the two venues cannot diverge.
			name:   "a double space spanning two adjacent target text runs fires",
			source: []model.Run{qaTx("Hello world")},
			target: []model.Run{qaTx("Bonjour "), qaTx(" monde")},
			want:   []string{"double-spaces"},
		},
		{
			name:   "a doubled word spanning two adjacent target text runs fires",
			source: []model.Run{qaTx("the end")},
			target: []model.Run{qaTx("le "), qaTx("le fin")},
			want:   []string{"doubled-word"},
		},
		{
			name:   "a placeholder glued to the first of two identical target words does not hide the double",
			source: []model.Run{qaPh("1", "x"), qaTx("the end")},
			target: []model.Run{qaPh("1", "x"), qaTx("le le fin")},
			want:   []string{"doubled-word"},
		},
		{
			name:   "an identical target still reads as same-as-source",
			source: []model.Run{qaPh("1", "x"), qaTx(" Hello")},
			target: []model.Run{qaPh("1", "x"), qaTx(" Hello")},
			want:   []string{"target-same-as-source"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qaRunCategories(t, tt.source, tt.target)
			for _, w := range tt.want {
				assert.Contains(t, got, w, "missing expected finding")
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, got, w, "false positive")
			}
		})
	}
}

// TestQACheck_SentinelNeverReachesCharacterRules is the guard for the boundary
// this fix deliberately draws: a sentinel is a stand-in for a run, not a
// character in the content, so the character-level rules must never see it.
// U+FFFC is not ISO-8859-1 encodable, so a leak here would report a
// charset-violation on every block that contains a placeholder.
func TestQACheck_SentinelNeverReachesCharacterRules(t *testing.T) {
	b := &model.Block{
		ID: "u1", Translatable: true,
		Source:     []model.Run{qaPh("1", "x"), qaTx(" cafe")},
		Properties: map[string]string{},
	}
	b.SetTargetRuns(model.LocaleFrench, []model.Run{qaPh("1", "x"), qaTx(" cafe")})

	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	cfg.CheckCharset = true
	cfg.Charset = "ISO-8859-1"
	tl := tools.NewQACheckTool(cfg)

	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)

	for _, f := range qaFindings(blk) {
		assert.NotEqual(t, "charset-violation", f.Category,
			"the shape sentinel must not reach the charset rule: %s", f.Message)
	}
}
