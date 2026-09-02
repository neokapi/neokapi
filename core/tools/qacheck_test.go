package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qaFindings returns the unified check findings recorded on a block under the
// quality.findings annotation (the model every checker now writes).
func qaFindings(b *model.Block) []check.Finding {
	return check.Findings(tool.NewBlockView(b))
}

// findFinding returns the first finding with the given category, or false.
func findFinding(findings []check.Finding, category string) (check.Finding, bool) {
	for _, f := range findings {
		if f.Category == category {
			return f, true
		}
	}
	return check.Finding{}, false
}

func TestQACheckTool(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	assert.Equal(t, "qa", tl.Name())
	assert.Contains(t, tl.Description(), "quality")
}

func TestQACheckToolPassingBlock(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock), "a clean block records no findings")
}

func TestQACheckToolEmptyTarget(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "empty-target", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
}

func TestQACheckToolLeadingWhitespace(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "leading-whitespace")
	require.True(t, found, "Expected leading-whitespace finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolTrailingWhitespace(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde  ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "trailing-whitespace")
	require.True(t, found, "Expected trailing-whitespace finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolDoubleSpaces(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour  le  monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "double-spaces")
	require.True(t, found, "Expected double-spaces finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolTargetSameAsSource(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "target-same-as-source")
	require.True(t, found, "Expected target-same-as-source finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolMultipleIssues(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Hello  world ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	findings := qaFindings(resultBlock)
	// Should have at least leading whitespace, double spaces, and trailing whitespace findings.
	assert.GreaterOrEqual(t, len(findings), 2, "Expected multiple findings, got %d", len(findings))
}

func TestQACheckToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock))
}

func TestQACheckToolDisabledChecks(t *testing.T) {
	t.Parallel()
	cfg := &tools.QACheckConfig{
		TargetLocale:            model.LocaleFrench,
		CheckLeadingWhitespace:  false,
		CheckTrailingWhitespace: false,
		CheckDoubleSpaces:       false,
		CheckEmptyTarget:        false,
		CheckTargetSameAsSource: false,
	}
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Hello  world ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock))
}

func TestQACheckConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.QACheckConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.QACheckConfig{},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name: "valid config",
			cfg:  tools.QACheckConfig{TargetLocale: model.LocaleFrench},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestQACheckToolNonDeletableSpanMissing(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
		{Ph: &model.PlaceholderRun{
			ID: "1", Type: "struct:break", Data: "<br/>",
			Constraints: &model.RunConstraints{Deletable: false},
		}},
		{Text: &model.TextRun{Text: "world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target is missing the break placeholder.
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "non-deletable-span-missing")
	require.True(t, found, "Expected non-deletable-span-missing finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "struct:break")
}

func TestQACheckToolNonCloneableSpanDuplicated(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	nonCloneable := func() *model.PlaceholderRun {
		return &model.PlaceholderRun{
			ID: "1", Type: "code:variable", Data: "{name}",
			Constraints: &model.RunConstraints{Cloneable: false},
		}
	}

	// Source has one non-cloneable variable placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target duplicates the variable placeholder.
	targetRuns := []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " le "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " monde"}},
	}
	block.SetTargetRuns(model.LocaleFrench, targetRuns)
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "non-cloneable-span-duplicated")
	require.True(t, found, "Expected non-cloneable-span-duplicated finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "code:variable")
}

func TestQACheckToolDeletableSpanMissingNoConstraintError(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a deletable bold pair.
	deletable := &model.RunConstraints{Deletable: true}
	sourceRuns := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>", Constraints: deletable}},
		{Text: &model.TextRun{Text: "Hello"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target is missing the bold pair (which are deletable).
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, found := findFinding(qaFindings(resultBlock), "non-deletable-span-missing")
	assert.False(t, found, "Should not flag deletable span as non-deletable")
}

func TestQACheckToolSpanConstraintsDisabled(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	cfg.CheckSpanConstraints = false
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
		{Ph: &model.PlaceholderRun{
			ID: "1", Type: "struct:break", Data: "<br/>",
			Constraints: &model.RunConstraints{Deletable: false},
		}},
		{Text: &model.TextRun{Text: "world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target is missing the break placeholder, but check is disabled.
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, found := findFinding(qaFindings(resultBlock), "non-deletable-span-missing")
	assert.False(t, found, "Should not check span constraints when disabled")
}

func TestQACheckToolEmptyTargetText(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "empty-target", findings[0].Category)
}

// ─── Coverage gate for the retired Okapi check fragments ────────────────────
//
// chars-check, length-check, pattern-check, and inconsistency-check were
// CheckMate-era fragments of qa. TestQACoversRetiredFragments takes each
// fragment's test fixtures and proves the qa tool's config reproduces
// equivalent findings, so the fragments could be deleted without losing a rule
// family. Rule families that qa could not express natively were ported first:
// forbidden/required characters + charset checks (chars-check), mojibake and
// control-char corruption detection (chars-check), absolute max word count
// (length-check), forbidden target patterns (pattern-check), and the
// cross-block target/source consistency checks (inconsistency-check).

// qaBlock builds a translatable block with the given source and (optional)
// French target text.
func qaBlock(id, source, target string) *model.Part {
	b := model.NewBlock(id, source)
	if target != "" {
		b.SetTargetText(model.LocaleFrench, target)
	}
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func TestQACoversRetiredFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fragment  string
		configure func(cfg *tools.QACheckConfig)
		blocks    []*model.Part // processed in order; the LAST block is asserted
		want      []string      // categories that must be present on the last block
		wantNot   []string      // categories that must be absent from the last block
	}{
		// ── chars-check ─────────────────────────────────────────────
		{
			name:     "forbidden characters in target",
			fragment: "chars-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.ForbiddenChars = "{}[]"
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello world", "Bonjour {le} monde")},
			want:   []string{"forbidden-char"},
		},
		{
			name:     "required character missing from target",
			fragment: "chars-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.RequiredChars = ".!"
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello world!", "Bonjour le monde")},
			want:   []string{"required-char-missing"},
		},
		{
			name:      "mojibake detection",
			fragment:  "chars-check",
			configure: func(cfg *tools.QACheckConfig) {},
			blocks:    []*model.Part{qaBlock("tu1", "Hello", "Bonjour lÃ¤ monde")},
			want:      []string{"mojibake"},
		},
		{
			name:      "unicode replacement character",
			fragment:  "chars-check",
			configure: func(cfg *tools.QACheckConfig) {},
			blocks:    []*model.Part{qaBlock("tu1", "Hello", "Bonjour � monde")},
			want:      []string{"replacement-char"},
		},
		{
			name:      "stray control character",
			fragment:  "chars-check",
			configure: func(cfg *tools.QACheckConfig) {},
			blocks:    []*model.Part{qaBlock("tu1", "Hello", "Bonjour\x01monde")},
			want:      []string{"control-char"},
		},
		{
			name:      "tab newline and carriage return are not corruption",
			fragment:  "chars-check",
			configure: func(cfg *tools.QACheckConfig) {},
			blocks:    []*model.Part{qaBlock("tu1", "Hello", "Bonjour\tle\nmonde\r")},
			wantNot:   []string{"control-char", "mojibake", "replacement-char"},
		},
		{
			name:     "character not encodable in configured charset",
			fragment: "chars-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckCharset = true
				cfg.Charset = "ISO-8859-1"
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello", "Bonjour → monde")},
			want:   []string{"charset-violation"},
		},
		{
			name:     "unknown charset reports a lookup error",
			fragment: "chars-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckCharset = true
				cfg.Charset = "NOT-A-CHARSET"
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello", "Bonjour")},
			want:   []string{"charset-lookup-error"},
		},

		// ── length-check ────────────────────────────────────────────
		{
			name:     "absolute maximum characters",
			fragment: "length-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckAbsoluteMaxCharLength = true
				cfg.AbsoluteMaxCharLength = 10
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello", "Bonjour le monde entier")},
			want:   []string{"absolute-max-length"},
		},
		{
			name:     "absolute maximum words",
			fragment: "length-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckMaxWords = true
				cfg.MaxWords = 2
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello", "Bonjour le monde")},
			want:   []string{"max-words"},
		},
		{
			name:     "flat maximum percentage via ratio thresholds",
			fragment: "length-check",
			configure: func(cfg *tools.QACheckConfig) {
				// length-check's flat MaxPercentage=150 is qa's ratio check
				// with the same limit above and below the break.
				cfg.MaxCharLengthAbove = 150
				cfg.MaxCharLengthBelow = 150
			},
			blocks: []*model.Part{qaBlock("tu1", "Hi", "Bonjour le monde!")},
			want:   []string{"max-length"},
		},
		{
			name:     "flat minimum percentage via ratio thresholds",
			fragment: "length-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.MinCharLengthAbove = 50
				cfg.MinCharLengthBelow = 50
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello world how are you", "Bon")},
			want:   []string{"min-length"},
		},

		// ── pattern-check ───────────────────────────────────────────
		{
			name:     "must-match pattern count parity",
			fragment: "pattern-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.Patterns = []tools.QAPattern{{
					Enabled: true, Source: `%[sd]`, Target: `%[sd]`,
					Description: "printf placeholders must be preserved",
				}}
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello %s, you have %d items", "Bonjour %s")},
			want:   []string{"pattern-mismatch"},
		},
		{
			name:     "forbidden pattern present in target",
			fragment: "pattern-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.Patterns = []tools.QAPattern{{
					Enabled: true, Source: `(?i)todo`, Forbidden: true,
					Description: "TODO markers must not ship",
				}}
			},
			blocks: []*model.Part{qaBlock("tu1", "Hello world", "Bonjour TODO monde")},
			want:   []string{"forbidden-pattern"},
		},
		{
			name:     "forbidden pattern absent stays clean",
			fragment: "pattern-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.Patterns = []tools.QAPattern{{
					Enabled: true, Source: `(?i)todo`, Forbidden: true,
				}}
			},
			blocks:  []*model.Part{qaBlock("tu1", "Hello world", "Bonjour le monde")},
			wantNot: []string{"forbidden-pattern"},
		},

		// ── inconsistency-check ─────────────────────────────────────
		{
			name:     "same source with different targets",
			fragment: "inconsistency-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckTargetInconsistency = true
			},
			blocks: []*model.Part{
				qaBlock("tu1", "Hello world", "Bonjour le monde"),
				qaBlock("tu2", "Hello world", "Salut le monde"),
			},
			want: []string{"inconsistency"},
		},
		{
			name:     "different sources sharing one target",
			fragment: "inconsistency-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckSourceInconsistency = true
			},
			blocks: []*model.Part{
				qaBlock("tu1", "Hello world", "Bonjour le monde"),
				qaBlock("tu2", "Goodbye world", "Bonjour le monde"),
			},
			want: []string{"inconsistency"},
		},
		{
			name:     "case-insensitive consistency comparison",
			fragment: "inconsistency-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckTargetInconsistency = true
				cfg.ConsistencyCaseSensitive = false
			},
			blocks: []*model.Part{
				qaBlock("tu1", "Hello World", "Bonjour le monde"),
				qaBlock("tu2", "hello world", "Salut le monde"),
			},
			want: []string{"inconsistency"},
		},
		{
			name:     "consistent translations stay clean",
			fragment: "inconsistency-check",
			configure: func(cfg *tools.QACheckConfig) {
				cfg.CheckTargetInconsistency = true
				cfg.CheckSourceInconsistency = true
			},
			blocks: []*model.Part{
				qaBlock("tu1", "Hello world", "Bonjour le monde"),
				qaBlock("tu2", "Hello world", "Bonjour le monde"),
			},
			wantNot: []string{"inconsistency"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.fragment+"/"+tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := tools.NewQACheckConfig(model.LocaleFrench)
			tt.configure(cfg)
			tl := tools.NewQACheckTool(cfg)

			results := processMultipleParts(t, tl, tt.blocks)
			require.Len(t, results, len(tt.blocks))
			last := results[len(results)-1].Resource.(*model.Block)
			findings := qaFindings(last)

			for _, category := range tt.want {
				_, found := findFinding(findings, category)
				assert.True(t, found, "qa must reproduce the retired %s finding %q; got %+v", tt.fragment, category, findings)
			}
			for _, category := range tt.wantNot {
				_, found := findFinding(findings, category)
				assert.False(t, found, "qa must not emit %q here; got %+v", category, findings)
			}
		})
	}
}
