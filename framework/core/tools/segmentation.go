package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
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
	TargetLocale model.LocaleID    `json:"targetLocale,omitempty" schema:"-"`
	Rules        []SegmentationRule `json:"rules,omitempty"        schema:"-"`

	// Schema-visible properties matching the bridge schema.
	SegmentSource                  bool   `json:"segmentSource,omitempty"                  schema:"title=Segment Source Text,description=Segment the source text using SRX rules,default=true"`
	SegmentTarget                  bool   `json:"segmentTarget,omitempty"                  schema:"title=Segment Target Text,description=Segment existing target text using SRX rules"`
	SourceSrxPath                  string `json:"sourceSrxPath,omitempty"                  schema:"title=Source SRX Rules Path,description=Path to SRX segmentation rules file for source text"`
	TargetSrxPath                  string `json:"targetSrxPath,omitempty"                  schema:"title=Target SRX Rules Path,description=Path to SRX segmentation rules file for target text"`
	OverwriteSegmentation          bool   `json:"overwriteSegmentation,omitempty"          schema:"title=Overwrite Existing Segmentation,description=Re-segment already-segmented text units replacing previous segmentation"`
	TreatIsolatedCodesAsWhitespace bool   `json:"treatIsolatedCodesAsWhitespace,omitempty" schema:"title=Treat Isolated Codes as Whitespace,description=Treat isolated inline codes as whitespace during segmentation"`
	RenumberCodes                  bool   `json:"renumberCodes,omitempty"                  schema:"title=Renumber Code IDs,description=Renumber inline code IDs in each segment to start at 1"`
}

// ToolName returns the tool name this config applies to.
func (c *SegmentationConfig) ToolName() string { return "segmentation" }

// Reset restores default values.
func (c *SegmentationConfig) Reset() {
	c.TargetLocale = ""
	c.Rules = nil
	c.SegmentSource = true
	c.SegmentTarget = false
	c.SourceSrxPath = ""
	c.TargetSrxPath = ""
	c.OverwriteSegmentation = false
	c.TreatIsolatedCodesAsWhitespace = false
	c.RenumberCodes = false
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

// SegmentationSchema returns the auto-generated schema for the segmentation tool.
func SegmentationSchema() *schema.ComponentSchema {
	cfg := &SegmentationConfig{}
	cfg.Reset()
	return schema.FromStruct(cfg, schema.ToolMeta{
		ID:          "segmentation",
		Category:    schema.CategoryTransform,
		DisplayName: "Segmentation",
		Description: "Split source text into sentence-level segments",
		Inputs:      []string{schema.PartTypeBlock},
	})
}

// NewSegmentationFromConfig creates a segmentation tool from a config map.
func NewSegmentationFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := &SegmentationConfig{}
	cfg.Reset()
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("segmentation config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewSegmentationTool(cfg), nil
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
				breakPos := min(
					// end of the match = break point
					loc[1], len(text))

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
	// Default: segment source if neither flag is explicitly set.
	if !cfg.SegmentSource && !cfg.SegmentTarget {
		cfg.SegmentSource = true
	}

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

		conf := t.Cfg.(*SegmentationConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		// Segment source text.
		if conf.SegmentSource {
			alreadySegmented := len(block.Source) > 1
			if !alreadySegmented || conf.OverwriteSegmentation {
				sourceText := block.SourceText()
				if sourceText == "" {
					block.Properties[PropSegmentCount] = "0"
				} else {
					segments := segmentText(sourceText, compiled)
					newSource := make([]*model.Segment, len(segments))
					for i, seg := range segments {
						newSource[i] = &model.Segment{
							ID:      fmt.Sprintf("s%d", i+1),
							Content: model.NewFragment(seg),
						}
					}
					block.Source = newSource
					block.Properties[PropSegmentCount] = strconv.Itoa(len(segments))
				}
			}
		}

		// Segment target text if enabled.
		if conf.SegmentTarget && !conf.TargetLocale.IsEmpty() && block.HasTarget(conf.TargetLocale) {
			targetSegs := block.Targets[conf.TargetLocale]
			alreadySegmented := len(targetSegs) > 1
			if !alreadySegmented || conf.OverwriteSegmentation {
				targetText := block.TargetText(conf.TargetLocale)
				if targetText != "" {
					segments := segmentText(targetText, compiled)
					newTarget := make([]*model.Segment, len(segments))
					for i, seg := range segments {
						newTarget[i] = &model.Segment{
							ID:      fmt.Sprintf("s%d", i+1),
							Content: model.NewFragment(seg),
						}
					}
					block.Targets[conf.TargetLocale] = newTarget
				}
			}
		}

		return part, nil
	}
	return t
}
