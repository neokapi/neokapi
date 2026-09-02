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

func checkTx(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func checkPh(id, equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "jsx:var", Data: "{" + equiv + "}", Equiv: equiv}}
}

// checkRunCategories runs the qa tool over a source/target run pair and returns the
// finding categories.
func checkRunCategories(t *testing.T, sourceRuns, targetRuns []model.Run) []string {
	t.Helper()
	b := &model.Block{ID: "u1", Translatable: true, Source: sourceRuns, Properties: map[string]string{}}
	b.SetTargetRuns(model.LocaleFrench, targetRuns)

	tl := tools.NewRuleCheckTool(tools.NewRuleCheckConfig(model.LocaleFrench))

	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)

	cats := make([]string, 0, 4)
	for _, f := range checkFindings(blk) {
		cats = append(cats, f.Category)
	}
	return cats
}

func TestRuleCheck_RunAwareShapeRules(t *testing.T) {
	tests := []struct {
		name    string
		source  []model.Run
		target  []model.Run
		want    []string
		wantNot []string
	}{
		{
			name:    "matching leading placeholders are not a whitespace mismatch",
			source:  []model.Run{checkPh("1", "price"), checkTx(" each")},
			target:  []model.Run{checkPh("1", "price"), checkTx(" chacun")},
			wantNot: []string{"leading-whitespace", "trailing-whitespace", "empty-target"},
		},
		{
			name:    "matching trailing placeholders are not a whitespace mismatch",
			source:  []model.Run{checkTx("Total: "), checkPh("1", "total")},
			target:  []model.Run{checkTx("Total : "), checkPh("1", "total")},
			wantNot: []string{"trailing-whitespace", "leading-whitespace"},
		},
		{
			name:    "a placeholder-only target is not empty",
			source:  []model.Run{checkPh("1", "price")},
			target:  []model.Run{checkPh("1", "price")},
			wantNot: []string{"empty-target", "empty-source"},
		},
		{
			name:    "space, placeholder, space in the target is not a double space",
			source:  []model.Run{checkTx("Hello "), checkPh("1", "name"), checkTx(" world")},
			target:  []model.Run{checkTx("Bonjour "), checkPh("1", "name"), checkTx(" monde")},
			wantNot: []string{"double-spaces"},
		},
		{
			name:    "the same word either side of a target placeholder is not a doubled word",
			source:  []model.Run{checkTx("the "), checkPh("1", "x"), checkTx(" the end")},
			target:  []model.Run{checkTx("le "), checkPh("1", "x"), checkTx(" le fin")},
			wantNot: []string{"doubled-word"},
		},
		{
			// Under the old flattening both sides read "Bonjour", so the target
			// looked identical to a source it does not resemble.
			name:    "text equal but placeholders moved is not same-as-source",
			source:  []model.Run{checkPh("1", "x"), checkTx("Bonjour")},
			target:  []model.Run{checkTx("Bonjour"), checkPh("1", "x")},
			wantNot: []string{"target-same-as-source"},
		},

		// Genuine positives.
		{
			name:   "a real leading-whitespace mismatch still fires",
			source: []model.Run{checkPh("1", "price"), checkTx(" each")},
			target: []model.Run{checkTx(" "), checkPh("1", "price"), checkTx(" chacun")},
			want:   []string{"leading-whitespace"},
		},
		{
			name:   "a real trailing-whitespace mismatch still fires",
			source: []model.Run{checkTx("Total: "), checkPh("1", "total")},
			target: []model.Run{checkTx("Total : "), checkPh("1", "total"), checkTx(" ")},
			want:   []string{"trailing-whitespace"},
		},
		{
			name:   "a real double space in the target still fires",
			source: []model.Run{checkTx("Hello "), checkPh("1", "name")},
			target: []model.Run{checkTx("Bonjour  "), checkPh("1", "name")},
			want:   []string{"double-spaces"},
		},
		{
			name:   "a real doubled word in the target still fires",
			source: []model.Run{checkPh("1", "x"), checkTx(" the end")},
			target: []model.Run{checkPh("1", "x"), checkTx(" le le fin")},
			want:   []string{"doubled-word"},
		},
		{
			// Two adjacent text runs have nothing between them: the join is
			// real text, so a defect spanning it is real. Both rules read the
			// same shared scanners as the source-side family (check.HygieneOverlay
			// / check.DoubledWord), so the two venues cannot diverge.
			name:   "a double space spanning two adjacent target text runs fires",
			source: []model.Run{checkTx("Hello world")},
			target: []model.Run{checkTx("Bonjour "), checkTx(" monde")},
			want:   []string{"double-spaces"},
		},
		{
			name:   "a doubled word spanning two adjacent target text runs fires",
			source: []model.Run{checkTx("the end")},
			target: []model.Run{checkTx("le "), checkTx("le fin")},
			want:   []string{"doubled-word"},
		},
		{
			name:   "a placeholder glued to the first of two identical target words does not hide the double",
			source: []model.Run{checkPh("1", "x"), checkTx("the end")},
			target: []model.Run{checkPh("1", "x"), checkTx("le le fin")},
			want:   []string{"doubled-word"},
		},
		{
			name:   "an identical target still reads as same-as-source",
			source: []model.Run{checkPh("1", "x"), checkTx(" Hello")},
			target: []model.Run{checkPh("1", "x"), checkTx(" Hello")},
			want:   []string{"target-same-as-source"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkRunCategories(t, tt.source, tt.target)
			for _, w := range tt.want {
				assert.Contains(t, got, w, "missing expected finding")
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, got, w, "false positive")
			}
		})
	}
}

// TestRuleCheck_TargetSameAsSourceWithCodes pins the knob that was declared,
// documented and defaulted true but read by nothing, so every comparison silently
// took the `false` branch (#1463). It is Okapi's targetSameAsSourceWithCodes:
// CODE_DATA_ONLY when set (text AND codes must match), IGNORE_CODE when clear
// (text and code positions only) — see GeneralChecker.java and its
// testTARGET_SAME_AS_SOURCE_WithDiffCodes / _WithDiffCodesTurnedOff pair.
func TestRuleCheck_TargetSameAsSourceWithCodes(t *testing.T) {
	// Same words, different placeholder: the shapes are equal (one code, same
	// position) but the codes are not.
	source := []model.Run{checkTx("src text "), checkPh("1", "code")}
	target := []model.Run{checkTx("src text "), checkPh("1", "etc")}

	t.Run("set (the default) — a different code means not identical", func(t *testing.T) {
		cfg := tools.NewRuleCheckConfig(model.LocaleFrench)
		require.True(t, cfg.TargetSameAsSourceWithCodes, "Reset must default the knob on")
		assert.NotContains(t, checkCategoriesWith(t, cfg, source, target), "target-same-as-source",
			"swapping {code} for {etc} is a change; reporting it as untranslated points a reviewer at the wrong defect")
	})

	t.Run("clear — codes ignored, so the text alone makes it identical", func(t *testing.T) {
		cfg := tools.NewRuleCheckConfig(model.LocaleFrench)
		cfg.TargetSameAsSourceWithCodes = false
		assert.Contains(t, checkCategoriesWith(t, cfg, source, target), "target-same-as-source")
	})

	t.Run("set — matching codes still report identical", func(t *testing.T) {
		cfg := tools.NewRuleCheckConfig(model.LocaleFrench)
		same := []model.Run{checkTx("src text "), checkPh("1", "code")}
		assert.Contains(t, checkCategoriesWith(t, cfg, source, same), "target-same-as-source")
	})
}

// checkCategoriesWith is checkRunCategories with a caller-supplied config.
func checkCategoriesWith(t *testing.T, cfg *tools.RuleCheckConfig, sourceRuns, targetRuns []model.Run) []string {
	t.Helper()
	b := &model.Block{ID: "u1", Translatable: true, Source: sourceRuns, Properties: map[string]string{}}
	b.SetTargetRuns(cfg.TargetLocale, targetRuns)

	out := processPart(t, tools.NewRuleCheckTool(cfg), &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)

	cats := make([]string, 0, 4)
	for _, f := range checkFindings(blk) {
		cats = append(cats, f.Category)
	}
	return cats
}

// TestRuleCheck_SentinelNeverReachesCharacterRules is the guard for the boundary
// this fix deliberately draws: a sentinel is a stand-in for a run, not a
// character in the content, so the character-level rules must never see it.
// U+FFFC is not ISO-8859-1 encodable, so a leak here would report a
// charset-violation on every block that contains a placeholder.
func TestRuleCheck_SentinelNeverReachesCharacterRules(t *testing.T) {
	b := &model.Block{
		ID: "u1", Translatable: true,
		Source:     []model.Run{checkPh("1", "x"), checkTx(" cafe")},
		Properties: map[string]string{},
	}
	b.SetTargetRuns(model.LocaleFrench, []model.Run{checkPh("1", "x"), checkTx(" cafe")})

	cfg := tools.NewRuleCheckConfig(model.LocaleFrench)
	cfg.CheckCharset = true
	cfg.Charset = "ISO-8859-1"
	tl := tools.NewRuleCheckTool(cfg)

	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)

	for _, f := range checkFindings(blk) {
		assert.NotEqual(t, "charset-violation", f.Category,
			"the shape sentinel must not reach the charset rule: %s", f.Message)
	}
}
