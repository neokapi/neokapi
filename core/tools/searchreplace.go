package tools

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

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
	// Precompile the regex pairs once at construction (per-block recompilation
	// was a hot-path cost) — mirrors patterncheck/segmentation. A compile error
	// is surfaced on first use, preserving the prior apply-time error behavior.
	compiled, compileErr := compilePairs(buildEffectivePairs(cfg))

	// Transform producer: returns the replacements as an edit plan; the
	// framework applier rewrites the block (AD-006).
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		if !v.Translatable() {
			return tool.EditPlan{}, nil
		}
		if compileErr != nil {
			return tool.EditPlan{}, fmt.Errorf("search-replace: %w", compileErr)
		}
		if len(compiled) == 0 {
			return tool.EditPlan{}, nil
		}

		conf := t.Cfg.(*SearchReplaceConfig)

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

		var targets []model.LocaleID
		if applyTarget && !conf.TargetLocale.IsEmpty() {
			targets = []model.LocaleID{conf.TargetLocale}
		}
		passes := make([]textRewrite, len(compiled))
		for i, pair := range compiled {
			passes[i] = pair.edits(replaceAll)
		}
		return textPlan(v, applySource, targets, passes...), nil
	}
	return t
}

// compiledPair is a search/replace pair with its regex compiled once.
// re is nil for literal (non-regex) pairs.
type compiledPair struct {
	re      *regexp.Regexp
	search  string
	replace string
}

// compilePairs precompiles the regex pairs, returning an error for the first
// invalid pattern (the same error the apply path used to return per block).
func compilePairs(pairs []ReplacePair) ([]compiledPair, error) {
	out := make([]compiledPair, len(pairs))
	for i, p := range pairs {
		out[i] = compiledPair{search: p.Search, replace: p.Replace}
		if p.IsRegex {
			re, err := regexp.Compile(p.Search)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %w", p.Search, err)
			}
			out[i].re = re
		}
	}
	return out, nil
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

// edits is the pair's pass: the edits that replace its matches in a text, every
// match when all is set and the first otherwise. Matches do not overlap, and a
// regular expression's replacement expands its groups ($1, ${name}) as
// regexp.Expand does.
func (p compiledPair) edits(all bool) textRewrite {
	return func(text string) []model.TextEdit {
		var matches [][]int
		switch {
		case p.re != nil && all:
			matches = p.re.FindAllStringSubmatchIndex(text, -1)
		case p.re != nil:
			if m := p.re.FindStringSubmatchIndex(text); m != nil {
				matches = [][]int{m}
			}
		default:
			matches = literalMatches(text, p.search, all)
		}
		if len(matches) == 0 {
			return nil
		}
		edits := make([]model.TextEdit, 0, len(matches))
		// The matches are byte offsets; a text edit counts code points.
		byteAt, runeAt := 0, 0
		toRunes := func(b int) int {
			runeAt += utf8.RuneCountInString(text[byteAt:b])
			byteAt = b
			return runeAt
		}
		for _, m := range matches {
			replacement := p.replace
			if p.re != nil {
				replacement = string(p.re.ExpandString(nil, p.replace, text, m))
			}
			start := toRunes(m[0])
			end := toRunes(m[1])
			edits = append(edits, model.TextEdit{Start: start, End: end, Replacement: replacement})
		}
		return edits
	}
}

// literalMatches returns the byte ranges of search in text, left to right and
// not overlapping: every one when all is set, the first otherwise. An empty
// search matches at the start and after each character, as strings.Replace
// treats it.
func literalMatches(text, search string, all bool) [][]int {
	var out [][]int
	for from := 0; from <= len(text); {
		i := strings.Index(text[from:], search)
		if i < 0 {
			break
		}
		start := from + i
		out = append(out, []int{start, start + len(search)})
		if !all {
			break
		}
		from = start + len(search)
		if search == "" {
			if from == len(text) {
				break
			}
			_, size := utf8.DecodeRuneInString(text[from:])
			from += size
		}
	}
	return out
}
