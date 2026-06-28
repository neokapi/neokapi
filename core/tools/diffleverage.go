package tools

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Diff leverage property keys stored on Block.Properties.
const (
	PropDiffLeverageStatus = "diff-leverage-status" // "unchanged", "modified", "new", "leveraged"
	PropDiffLeverageScore  = "diff-leverage-score"  // Similarity score (0-100) for fuzzy matches
)

// PreviousBlock holds the previous version of a block's source and target text.
type PreviousBlock struct {
	SourceText string
	TargetText string
}

// DiffLeverageConfig holds configuration for the diff leverage tool.
type DiffLeverageConfig struct {
	TargetLocale  model.LocaleID           `json:"targetLocale,omitempty"  schema:"-"`
	PreviousTexts map[string]PreviousBlock `json:"previousTexts,omitempty" schema:"-"`
	CaseSensitive bool                     `json:"caseSensitive,omitempty" schema:"title=Case Sensitive,description=Whether comparison is case-sensitive,default=true"`
	FuzzyMatch    bool                     `json:"fuzzyMatch,omitempty"    schema:"title=Fuzzy Match,description=Enable fuzzy matching for similar texts"`
}

// ToolName returns the tool name this config applies to.
func (c *DiffLeverageConfig) ToolName() string { return "diff-leverage" }

// Reset restores default values.
func (c *DiffLeverageConfig) Reset() {
	c.TargetLocale = ""
	c.PreviousTexts = nil
	c.CaseSensitive = true
	c.FuzzyMatch = false
}

// Validate checks configuration validity.
func (c *DiffLeverageConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("diff-leverage: TargetLocale is required")
	}
	if c.PreviousTexts == nil {
		return errors.New("diff-leverage: PreviousTexts is required")
	}
	return nil
}

// DiffLeverageSchema returns the auto-generated schema for the diff-leverage tool.
func DiffLeverageSchema() *schema.ComponentSchema {
	return schema.FromStruct(&DiffLeverageConfig{}, schema.ToolMeta{
		ID:          "diff-leverage",
		Category:    schema.CategoryTranslation,
		DisplayName: "Diff Leverage",
		Description: "Leverage translations from previous versions using diff analysis",
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewDiffLeverageFromConfig creates a diff-leverage tool from a config map.
func NewDiffLeverageFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg DiffLeverageConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("diff-leverage config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	if cfg.PreviousTexts == nil {
		cfg.PreviousTexts = map[string]PreviousBlock{}
	}
	return NewDiffLeverageTool(&cfg), nil
}

// NewDiffLeverageTool creates a diff leverage tool that compares blocks between
// an old document version and the current pipeline. It preserves existing
// translations for unchanged source text, avoiding re-translation of unchanged content.
func NewDiffLeverageTool(cfg *DiffLeverageConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "diff-leverage",
		ToolDescription: "Compares blocks against previous version, preserving translations for unchanged text",
		Cfg:             cfg,
	}
	// Translate: diff-leverage writes a target from previous versions; source is read-only.
	t.Produce = func(v tool.VariantView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*DiffLeverageConfig)

		sourceText := v.SourceText()

		prev, found := conf.PreviousTexts[v.ID()]
		if !found {
			v.SetProperty(PropDiffLeverageStatus, "new")
			return nil
		}

		// Compare source texts.
		srcMatch := sourceText == prev.SourceText
		if !conf.CaseSensitive {
			srcMatch = strings.EqualFold(sourceText, prev.SourceText)
		}

		if srcMatch {
			v.SetProperty(PropDiffLeverageStatus, "unchanged")
			if prev.TargetText != "" {
				v.SetTargetText(conf.TargetLocale, prev.TargetText)
			}
			return nil
		}

		// Source text differs.
		if conf.FuzzyMatch {
			a, b := sourceText, prev.SourceText
			if !conf.CaseSensitive {
				a = strings.ToLower(a)
				b = strings.ToLower(b)
			}
			score := similarityScore(a, b)
			if score > 70 {
				v.SetProperty(PropDiffLeverageStatus, "leveraged")
				v.SetProperty(PropDiffLeverageScore, strconv.Itoa(score))
				if prev.TargetText != "" {
					v.SetTargetText(conf.TargetLocale, prev.TargetText)
					// A fuzzy-leveraged target was carried over onto a *changed*
					// source: it no longer exactly fits, so mark it `draft` —
					// it counts as work-in-progress, not as `translated`, until
					// a human or translator revisits it.
					v.StampTargetProvenance(conf.TargetLocale, model.TargetStatusDraft, model.Origin{Tool: t.ToolName, Kind: model.OriginTM})
				}
				return nil
			}
		}

		v.SetProperty(PropDiffLeverageStatus, "modified")
		return nil
	}
	return t
}

// levenshteinDistance computes the Levenshtein edit distance between two strings.
func levenshteinDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
		}
	}
	return d[la][lb]
}

// similarityScore returns a percentage (0-100) indicating how similar two strings are.
func similarityScore(a, b string) int {
	if a == b {
		return 100
	}
	maxLen := max(len([]rune(a)), len([]rune(b)))
	if maxLen == 0 {
		return 100
	}
	dist := levenshteinDistance(a, b)
	return int(float64(maxLen-dist) / float64(maxLen) * 100)
}
