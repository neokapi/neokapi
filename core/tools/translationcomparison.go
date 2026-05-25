package tools

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Translation comparison property keys stored on Block.Properties.
const (
	PropComparisonStatus = "comparison-status" // "identical", "different", "missing-locale1", "missing-locale2", "missing-both"
	PropComparisonDiff   = "comparison-diff"   // Simple diff description
)

// TranslationComparisonConfig holds configuration for the translation comparison tool.
type TranslationComparisonConfig struct {
	Locale1 model.LocaleID `json:"locale1,omitempty" schema:"-"`
	Locale2 model.LocaleID `json:"locale2,omitempty" schema:"-"`

	// Comparison sensitivity settings.
	CaseSensitive        bool `json:"caseSensitive,omitempty"        schema:"title=Case Sensitive,description=Take case differences into account when comparing,default=true"`
	WhitespaceSensitive  bool `json:"whitespaceSensitive,omitempty"  schema:"title=Whitespace Sensitive,description=Take whitespace differences into account when comparing,default=true"`
	PunctuationSensitive bool `json:"punctuationSensitive,omitempty" schema:"title=Punctuation Sensitive,description=Take punctuation differences into account when comparing,default=true"`

	// Report labels for identifying compared translations.
	Document1Label string `json:"document1Label,omitempty" schema:"title=Document 1 Label,description=Label for the first translation in reports,default=Trans1"`
	Document2Label string `json:"document2Label,omitempty" schema:"title=Document 2 Label,description=Label for the second translation in reports,default=Trans2"`

	// Output options.
	GenericCodes bool `json:"genericCodes,omitempty" schema:"title=Use Generic Codes,description=Use generic numbered tags (e.g. <1>...</1>) instead of original inline codes in reports,default=true"`
}

// ToolName returns the tool name this config applies to.
func (c *TranslationComparisonConfig) ToolName() string { return "translation-comparison" }

// Reset restores default values.
func (c *TranslationComparisonConfig) Reset() {
	c.Locale1 = ""
	c.Locale2 = ""
	c.CaseSensitive = true
	c.WhitespaceSensitive = true
	c.PunctuationSensitive = true
	c.Document1Label = "Trans1"
	c.Document2Label = "Trans2"
	c.GenericCodes = true
}

// Validate checks configuration validity.
func (c *TranslationComparisonConfig) Validate() error {
	if c.Locale1.IsEmpty() {
		return errors.New("translation-comparison: Locale1 is required")
	}
	if c.Locale2.IsEmpty() {
		return errors.New("translation-comparison: Locale2 is required")
	}
	return nil
}

// TranslationComparisonSchema returns the auto-generated schema for the translation-comparison tool.
func TranslationComparisonSchema() *schema.ComponentSchema {
	return schema.FromStruct(&TranslationComparisonConfig{}, schema.ToolMeta{
		ID:          "translation-comparison",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Translation Comparison",
		Description: "Compare translations across locales or versions",
		Inputs:      []string{schema.PartTypeBlock},
	})
}

// NewTranslationComparisonFromConfig creates a translation-comparison tool from a config map.
func NewTranslationComparisonFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg TranslationComparisonConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("translation-comparison config: %w", err)
	}
	return NewTranslationComparisonTool(&cfg), nil
}

// NewTranslationComparisonTool creates a tool that compares translations across
// two target locales for the same source text and reports differences.
// Results are stored in Block.Properties using PropComparisonStatus and PropComparisonDiff.
func NewTranslationComparisonTool(cfg *TranslationComparisonConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "translation-comparison",
		ToolDescription: "Compares translations across two target locales and reports differences",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*TranslationComparisonConfig)

		has1 := v.HasTarget(conf.Locale1)
		has2 := v.HasTarget(conf.Locale2)

		switch {
		case !has1 && !has2:
			v.SetProperty(PropComparisonStatus, "missing-both")
			v.SetProperty(PropComparisonDiff, fmt.Sprintf(
				"Both %s and %s translations are missing", conf.Locale1, conf.Locale2))

		case !has1:
			v.SetProperty(PropComparisonStatus, "missing-locale1")
			v.SetProperty(PropComparisonDiff, fmt.Sprintf(
				"Translation for %s is missing", conf.Locale1))

		case !has2:
			v.SetProperty(PropComparisonStatus, "missing-locale2")
			v.SetProperty(PropComparisonDiff, fmt.Sprintf(
				"Translation for %s is missing", conf.Locale2))

		default:
			text1 := v.TargetText(conf.Locale1)
			text2 := v.TargetText(conf.Locale2)

			cmp1, cmp2 := text1, text2
			if !conf.CaseSensitive {
				cmp1 = strings.ToLower(cmp1)
				cmp2 = strings.ToLower(cmp2)
			}
			if !conf.WhitespaceSensitive {
				cmp1 = normalizeComparisonWhitespace(cmp1)
				cmp2 = normalizeComparisonWhitespace(cmp2)
			}
			if !conf.PunctuationSensitive {
				cmp1 = stripPunctuation(cmp1)
				cmp2 = stripPunctuation(cmp2)
			}

			label1 := conf.Document1Label
			if label1 == "" {
				label1 = string(conf.Locale1)
			}
			label2 := conf.Document2Label
			if label2 == "" {
				label2 = string(conf.Locale2)
			}

			if cmp1 == cmp2 {
				v.SetProperty(PropComparisonStatus, "identical")
				v.SetProperty(PropComparisonDiff, fmt.Sprintf(
					"Translations for %s and %s are identical", label1, label2))
			} else {
				v.SetProperty(PropComparisonStatus, "different")
				v.SetProperty(PropComparisonDiff, fmt.Sprintf(
					"%s: %q vs %s: %q", label1, text1, label2, text2))
			}
		}

		return nil
	}
	return t
}

// normalizeComparisonWhitespace collapses all whitespace runs to a single space and trims.
func normalizeComparisonWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stripPunctuation removes all Unicode punctuation characters from the string.
func stripPunctuation(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !unicode.IsPunct(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
