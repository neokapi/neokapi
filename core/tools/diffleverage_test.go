package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffLeverageTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale:  model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	assert.Equal(t, "diff-leverage", tl.Name())
	assert.Contains(t, tl.Description(), "previous version")
}

func TestDiffLeverageUnchangedSource(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "unchanged", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))

	// An identical source keeps its prior translation as-is: no draft downgrade.
	// (Coverage falls to the presence baseline of `translated`.)
	if tgt := resultBlock.Target(model.LocaleFrench); assert.NotNil(t, tgt) {
		assert.Empty(t, tgt.Status, "unchanged leverage must not downgrade to draft")
	}
}

func TestDiffLeverageModifiedSource(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello new world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "modified", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestDiffLeverageNewBlock(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu2", "Goodbye world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "new", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestDiffLeverageFuzzyMatch(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
		FuzzyMatch:    true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	// Small change — should be above 70% similarity.
	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "leveraged", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))

	score := resultBlock.Properties[tools.PropDiffLeverageScore]
	require.NotEmpty(t, score, "Expected a similarity score")

	// The leveraged target rode over onto a changed source, so it needs review:
	// it must be stamped `draft`, not counted as fully `translated`.
	tgt := resultBlock.Target(model.LocaleFrench)
	if assert.NotNil(t, tgt) {
		assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	}
}

func TestDiffLeverageFuzzyMatchBelowThreshold(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
		FuzzyMatch:    true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	// Completely different text — below 70%.
	block := model.NewBlock("tu1", "Something entirely different here now")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "modified", resultBlock.Properties[tools.PropDiffLeverageStatus])
	_, hasScore := resultBlock.Properties[tools.PropDiffLeverageScore]
	assert.False(t, hasScore, "Should not have a score for non-leveraged blocks")
}

func TestDiffLeverageSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasStatus := resultBlock.Properties[tools.PropDiffLeverageStatus]
	assert.False(t, hasStatus)
}

func TestDiffLeverageCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello World", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: false,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "unchanged", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestDiffLeverageConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.DiffLeverageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.DiffLeverageConfig{PreviousTexts: map[string]tools.PreviousBlock{}},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name:    "missing previous texts",
			cfg:     tools.DiffLeverageConfig{TargetLocale: model.LocaleFrench},
			wantErr: true,
			errMsg:  "PreviousTexts",
		},
		{
			name: "valid config",
			cfg: tools.DiffLeverageConfig{
				TargetLocale:  model.LocaleFrench,
				PreviousTexts: map[string]tools.PreviousBlock{},
			},
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

func TestDiffLeverageConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale:  model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{"tu1": {}},
		CaseSensitive: false,
		FuzzyMatch:    true,
	}
	cfg.Reset()

	assert.True(t, cfg.TargetLocale.IsEmpty())
	assert.Nil(t, cfg.PreviousTexts)
	assert.True(t, cfg.CaseSensitive)
	assert.False(t, cfg.FuzzyMatch)
}

// TestDiffLeverageRejectsCodeLoss: diff-leverage keys on flattened text, so a
// previous version's plain target text carries no inline codes. Writing it onto
// a block whose source holds a placeholder Run would drop that placeholder —
// the same silent content loss the TM path guards against — so the carry-over
// is refused and the block falls back to source. The status property still
// records what the diff found.
func TestDiffLeverageRejectsCodeLoss(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		source     []model.Run
		prev       tools.PreviousBlock
		fuzzy      bool
		wantStatus string
		wantTarget string
	}{
		{
			name:       "unchanged source, code-free: carry-over allowed",
			source:     []model.Run{{Text: &model.TextRun{Text: "Hello world"}}},
			prev:       tools.PreviousBlock{SourceText: "Hello world", TargetText: "Bonjour le monde"},
			wantStatus: "unchanged",
			wantTarget: "Bonjour le monde",
		},
		{
			name: "unchanged source with a placeholder run: carry-over refused",
			source: []model.Run{
				{Ph: &model.PlaceholderRun{ID: "1", Equiv: "count", Data: "{count}"}},
				{Text: &model.TextRun{Text: " files"}},
			},
			prev:       tools.PreviousBlock{SourceText: " files", TargetText: " fichiers"},
			wantStatus: "unchanged",
		},
		{
			name: "fuzzy leverage onto a placeholder-bearing source: refused",
			source: []model.Run{
				{Ph: &model.PlaceholderRun{ID: "1", Equiv: "count", Data: "{count}"}},
				{Text: &model.TextRun{Text: " files selected"}},
			},
			prev:       tools.PreviousBlock{SourceText: " file selected", TargetText: " fichier sélectionné"},
			fuzzy:      true,
			wantStatus: "leveraged",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &tools.DiffLeverageConfig{
				TargetLocale:  model.LocaleFrench,
				PreviousTexts: map[string]tools.PreviousBlock{"tu1": tc.prev},
				CaseSensitive: true,
				FuzzyMatch:    tc.fuzzy,
			}
			tl := tools.NewDiffLeverageTool(cfg)

			block := model.NewBlock("tu1", "")
			block.Source = tc.source
			result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
			rb := result.Resource.(*model.Block)

			assert.Equal(t, tc.wantStatus, rb.Properties[tools.PropDiffLeverageStatus])
			if tc.wantTarget == "" {
				assert.False(t, rb.HasTarget(model.LocaleFrench),
					"a carry-over that would drop the source placeholder must not be written")
				return
			}
			assert.Equal(t, tc.wantTarget, rb.TargetText(model.LocaleFrench))
		})
	}
}
