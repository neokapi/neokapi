package tools

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ReplacePair defines a single search-and-replace operation.
type ReplacePair struct {
	Search  string // The text or regex pattern to search for
	Replace string // The replacement text
	IsRegex bool   // If true, Search is treated as a regular expression
}

// SearchReplaceConfig holds configuration for the search-and-replace tool.
type SearchReplaceConfig struct {
	Pairs        []ReplacePair  `json:"pairs,omitempty"        schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`

	// Schema-visible properties matching the bridge schema.
	RegEx      bool `json:"regEx,omitempty"           schema:"title=Use Regular Expressions,description=Enable regular expression mode for all search patterns"`
	DotAll     bool `json:"dotAll,omitempty"          schema:"title=Dot Also Matches Line-Feed,description=Make the period character match every character including line-feed"`
	IgnoreCase bool `json:"ignoreCase,omitempty"      schema:"title=Ignore Case Differences,description=Ignore case when matching search patterns"`
	MultiLine  bool `json:"multiLine,omitempty"       schema:"title=Multiline Mode,description=Make ^ and $ match at the beginning and end of each line"`
	Target     bool `json:"target,omitempty"          schema:"title=Replace in Target Content,description=Perform search and replace on target content,default=true"`
	Source     bool `json:"source,omitempty"          schema:"title=Replace in Source Content,description=Perform search and replace on source content"`
	ReplaceAll bool `json:"replaceAll,omitempty"      schema:"title=Replace All Instances,description=Replace all matches instead of only the first,default=true"`
}

// ToolName returns the tool name this config applies to.
func (c *SearchReplaceConfig) ToolName() string { return "search-replace" }

// Reset restores default values.
func (c *SearchReplaceConfig) Reset() {
	c.Pairs = nil
	c.TargetLocale = ""
	c.RegEx = false
	c.DotAll = false
	c.IgnoreCase = false
	c.MultiLine = false
	c.Target = true
	c.Source = false
	c.ReplaceAll = true
}

// Validate checks configuration validity.
func (c *SearchReplaceConfig) Validate() error {
	for i, pair := range c.Pairs {
		if pair.Search == "" {
			return fmt.Errorf("search-replace: pair %d has empty search string", i)
		}
		if pair.IsRegex {
			if _, err := regexp.Compile(pair.Search); err != nil {
				return fmt.Errorf("search-replace: pair %d has invalid regex %q: %w", i, pair.Search, err)
			}
		}
	}
	return nil
}

// SearchReplaceSchema returns the auto-generated schema for the search-replace tool.
func SearchReplaceSchema() *schema.ComponentSchema {
	cfg := &SearchReplaceConfig{}
	cfg.Reset()
	return schema.FromStruct(cfg, schema.ToolMeta{
		ID:          "search-replace",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Search Replace",
		Description: "Find and replace patterns (literal or regex)",
		Inputs:      []string{schema.PartTypeBlock},
	})
}

// NewSearchReplaceFromConfig creates a search-replace tool from a config map.
func NewSearchReplaceFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := &SearchReplaceConfig{}
	cfg.Reset()
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("search-replace config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewSearchReplaceTool(cfg), nil
}

// NewSearchReplaceTool creates a new search-and-replace tool.
// It performs search and replace operations on Block source and target text.
func NewSearchReplaceTool(cfg *SearchReplaceConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "search-replace",
		ToolDescription: "Performs search and replace on block text",
		Cfg:             cfg,
	}
	// Transform: search-replace may rewrite source and/or target text.
	t.Transform = func(v tool.SourceView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*SearchReplaceConfig)
		if len(conf.Pairs) == 0 {
			return nil
		}

		// Build effective pairs, applying config-level regex/case/dotAll/multiLine flags.
		effectivePairs := buildEffectivePairs(conf)

		// replaceAll defaults to true for backward compat when not set via schema.
		replaceAll := conf.ReplaceAll
		if !conf.Source && !conf.Target {
			// Legacy mode: neither scope flag set, replace all by default.
			replaceAll = true
		}

		// Determine scope: if neither Source nor Target is explicitly set,
		// apply to both for backward compatibility with programmatic usage.
		applySource := conf.Source
		applyTarget := conf.Target
		if !applySource && !applyTarget {
			applySource = true
			applyTarget = true
		}

		// Apply to source text if enabled.
		if applySource {
			sourceText := v.SourceText()
			newSource, err := applyReplacements(sourceText, effectivePairs, replaceAll)
			if err != nil {
				return fmt.Errorf("search-replace source: %w", err)
			}
			if newSource != sourceText {
				v.SetSourceText(newSource)
			}
		}

		// Apply to target text if enabled and locale is set and target exists.
		if applyTarget && !conf.TargetLocale.IsEmpty() && v.HasTarget(conf.TargetLocale) {
			targetText := v.TargetText(conf.TargetLocale)
			newTarget, err := applyReplacements(targetText, effectivePairs, replaceAll)
			if err != nil {
				return fmt.Errorf("search-replace target: %w", err)
			}
			if newTarget != targetText {
				v.SetTargetText(conf.TargetLocale, newTarget)
			}
		}

		return nil
	}
	return t
}

// buildEffectivePairs creates effective pairs from config, applying the config-level
// regex, case-insensitive, dotAll, and multiLine flags to each pair.
func buildEffectivePairs(conf *SearchReplaceConfig) []ReplacePair {
	pairs := make([]ReplacePair, len(conf.Pairs))
	for i, p := range conf.Pairs {
		pairs[i] = ReplacePair{
			Search:  p.Search,
			Replace: p.Replace,
			IsRegex: p.IsRegex || conf.RegEx,
		}
		// If config-level regex mode is on, apply regex flags.
		if pairs[i].IsRegex {
			var prefix string
			if conf.IgnoreCase {
				prefix += "(?i)"
			}
			if conf.DotAll {
				prefix += "(?s)"
			}
			if conf.MultiLine {
				prefix += "(?m)"
			}
			if prefix != "" {
				pairs[i].Search = prefix + pairs[i].Search
			}
		}
	}
	return pairs
}

// applyReplacements applies all replacement pairs to the given text.
// If replaceAll is false, only the first match is replaced.
func applyReplacements(text string, pairs []ReplacePair, replaceAll bool) (string, error) {
	result := text
	for _, pair := range pairs {
		if pair.IsRegex {
			re, err := regexp.Compile(pair.Search)
			if err != nil {
				return "", fmt.Errorf("invalid regex %q: %w", pair.Search, err)
			}
			if replaceAll {
				result = re.ReplaceAllString(result, pair.Replace)
			} else {
				loc := re.FindStringIndex(result)
				if loc != nil {
					result = result[:loc[0]] + re.ReplaceAllString(result[loc[0]:loc[1]], pair.Replace) + result[loc[1]:]
				}
			}
		} else {
			if replaceAll {
				result = strings.ReplaceAll(result, pair.Search, pair.Replace)
			} else {
				result = strings.Replace(result, pair.Search, pair.Replace, 1)
			}
		}
	}
	return result, nil
}
