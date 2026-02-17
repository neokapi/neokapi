package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// Segmentation property keys stored on Block.Properties.
const (
	PropSegmentCount = "segment-count"
)

// SegmentationRule defines a break or no-break pattern for sentence segmentation.
// Break rules cause a split at the match position; no-break rules suppress a split.
type SegmentationRule struct {
	BeforeBreak string // Regex matching text before the break point
	AfterBreak  string // Regex matching text after the break point
	IsBreak     bool   // true = split here, false = do NOT split here
}

// SegmentationConfig holds configuration for the segmentation tool.
type SegmentationConfig struct {
	TargetLocale model.LocaleID
	Rules        []SegmentationRule // Custom rules; if empty, defaults are used
}

// ToolName returns the tool name this config applies to.
func (c *SegmentationConfig) ToolName() string { return "segmentation" }

// Reset restores default values.
func (c *SegmentationConfig) Reset() {
	c.TargetLocale = ""
	c.Rules = nil
}

// Validate checks configuration validity.
func (c *SegmentationConfig) Validate() error {
	for i, r := range c.Rules {
		if r.BeforeBreak == "" && r.AfterBreak == "" {
			return fmt.Errorf("segmentation: rule %d has no patterns", i)
		}
		if r.BeforeBreak != "" {
			if _, err := regexp.Compile(r.BeforeBreak); err != nil {
				return fmt.Errorf("segmentation: rule %d BeforeBreak: invalid regex: %w", i, err)
			}
		}
		if r.AfterBreak != "" {
			if _, err := regexp.Compile(r.AfterBreak); err != nil {
				return fmt.Errorf("segmentation: rule %d AfterBreak: invalid regex: %w", i, err)
			}
		}
	}
	return nil
}

// defaultSegmentationRules returns SRX-like rules for common sentence boundaries.
func defaultSegmentationRules() []SegmentationRule {
	return []SegmentationRule{
		// No-break: common abbreviations followed by period and space.
		{BeforeBreak: `(?:Mr|Mrs|Ms|Dr|Prof|Sr|Jr|St|etc|vs|approx|dept|est|vol)\.\s`, IsBreak: false},
		// No-break: single capital letter followed by period (initials like "J. K.").
		{BeforeBreak: `[A-Z]\.\s`, IsBreak: false},
		// Break: sentence-ending punctuation (.!?) followed by whitespace.
		{BeforeBreak: `[.!?]`, AfterBreak: `\s+[A-Z\p{Lu}]`, IsBreak: true},
		// Break: sentence-ending punctuation followed by end of string.
		{BeforeBreak: `[.!?]$`, IsBreak: true},
	}
}

// compiledRule holds pre-compiled regexes for a SegmentationRule.
type compiledRule struct {
	before  *regexp.Regexp
	after   *regexp.Regexp
	isBreak bool
}

func compileRules(rules []SegmentationRule) ([]compiledRule, error) {
	compiled := make([]compiledRule, len(rules))
	for i, r := range rules {
		cr := compiledRule{isBreak: r.IsBreak}
		if r.BeforeBreak != "" {
			re, err := regexp.Compile(r.BeforeBreak)
			if err != nil {
				return nil, fmt.Errorf("rule %d: %w", i, err)
			}
			cr.before = re
		}
		if r.AfterBreak != "" {
			re, err := regexp.Compile(r.AfterBreak)
			if err != nil {
				return nil, fmt.Errorf("rule %d: %w", i, err)
			}
			cr.after = re
		}
		compiled[i] = cr
	}
	return compiled, nil
}

// segmentText splits text into sentences using the provided rules.
// It returns the list of segments.
func segmentText(text string, rules []compiledRule) []string {
	if text == "" {
		return nil
	}

	// Find all potential break points.
	runes := []rune(text)
	textLen := len(runes)

	// For each position, determine if it is a break point.
	breakPoints := make(map[int]bool)

	for _, rule := range rules {
		// Find matches for the before pattern.
		if rule.before != nil {
			locs := rule.before.FindAllStringIndex(text, -1)
			for _, loc := range locs {
				breakPos := loc[1] // end of the match = break point

				if breakPos >= len(text) {
					breakPos = len(text)
				}

				// If there is an after-break pattern, check it.
				if rule.after != nil {
					remaining := text[breakPos:]
					if !rule.after.MatchString(remaining) {
						continue
					}
				}

				if rule.isBreak {
					// Only set break if not already suppressed.
					if _, exists := breakPoints[breakPos]; !exists {
						breakPoints[breakPos] = true
					}
				} else {
					// No-break: suppress this position.
					breakPoints[breakPos] = false
				}
			}
		}
	}

	// Collect break positions in order.
	var positions []int
	for pos, isBreak := range breakPoints {
		if isBreak {
			positions = append(positions, pos)
		}
	}

	if len(positions) == 0 {
		return []string{text}
	}

	// Sort positions.
	sortPositions(positions)

	// Split text at break positions.
	var segments []string
	prev := 0
	for _, pos := range positions {
		if pos <= prev || pos > textLen {
			continue
		}
		seg := strings.TrimSpace(text[prev:pos])
		if seg != "" {
			segments = append(segments, seg)
		}
		prev = pos
	}
	// Remaining text.
	if prev < len(text) {
		seg := strings.TrimSpace(text[prev:])
		if seg != "" {
			segments = append(segments, seg)
		}
	}

	if len(segments) == 0 {
		return []string{text}
	}
	return segments
}

// sortPositions sorts an int slice in ascending order (simple insertion sort for small slices).
func sortPositions(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}

// NewSegmentationTool creates a sentence segmentation tool that splits Block
// source text into segments using SRX-like regex rules.
func NewSegmentationTool(cfg *SegmentationConfig) *tool.BaseTool {
	rules := cfg.Rules
	if len(rules) == 0 {
		rules = defaultSegmentationRules()
	}

	compiled, err := compileRules(rules)
	if err != nil {
		// Fallback: no rules means no splitting.
		compiled = nil
	}

	t := &tool.BaseTool{
		ToolName:        "segmentation",
		ToolDescription: "Splits source text into sentence segments using SRX-like rules",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()
		if sourceText == "" {
			block.Properties[PropSegmentCount] = "0"
			return part, nil
		}

		segments := segmentText(sourceText, compiled)

		// Rebuild source segments from the split text.
		newSource := make([]*model.Segment, len(segments))
		for i, seg := range segments {
			newSource[i] = &model.Segment{
				ID:      fmt.Sprintf("s%d", i+1),
				Content: model.NewFragment(seg),
			}
		}
		block.Source = newSource

		block.Properties[PropSegmentCount] = strconv.Itoa(len(segments))

		return part, nil
	}
	return t
}
