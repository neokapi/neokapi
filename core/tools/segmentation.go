package tools

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/neokapi/neokapi/core/tool"

	// Register the built-in segmentation engines so the segment tool can
	// select them by name. SRX (the default) is pure Go; UAX-29 is ICU-backed
	// where cgo is available and silently absent otherwise. The LLM and SaT
	// engines register from their own packages (core/ai/tools, the CLI) when
	// linked.
	_ "github.com/neokapi/neokapi/core/segment/srx"
	_ "github.com/neokapi/neokapi/core/segment/uax29"
)

// Segmentation property keys stored on Block.Properties.
const (
	PropSegmentCount = "segment-count"
)

// SegmentationRule defines a break or no-break pattern for the inline-rules
// segmentation fallback. Break rules cause a split at the match position;
// no-break rules suppress a split. For full SRX 2.0 rule files use the srx
// engine with SourceSrxPath / TargetSrxPath instead.
type SegmentationRule struct {
	BeforeBreak string // Regex matching text before the break point
	AfterBreak  string // Regex matching text after the break point
	IsBreak     bool   // true = split here, false = do NOT split here
}

// SegmentationConfig holds configuration for the segmentation tool.
type SegmentationConfig struct {
	TargetLocale model.LocaleID     `json:"targetLocale,omitempty" schema:"-"`
	Rules        []SegmentationRule `json:"rules,omitempty"        schema:"-"`

	// Engine selects the segmenter backend. The default, srx, is a faithful
	// SRX 2.0 rule engine; uax29 is the ICU Unicode sentence baseline; llm
	// produces semantic chunks via an AI provider; sat runs the SaT ML model
	// through the kapi-sat plugin. An inline Rules list overrides Engine.
	Engine string `json:"engine,omitempty" schema:"title=Segmentation Engine,description=Segmenter backend: srx (rule-based; default)/ uax29 (Unicode baseline)/ llm (semantic chunks)/ sat (ML model)"`
	// Layer names the segmentation overlay layer. Empty is the primary
	// sentence layer (the one bilingual formats project); named layers such as
	// llm-chunk coexist alongside it. Empty defers to the engine's natural
	// layer (sentence for srx/uax29/sat, llm-chunk for llm).
	Layer string `json:"layer,omitempty" schema:"title=Overlay Layer,description=Segmentation overlay layer name; empty uses the engine's natural layer"`

	// Schema-visible properties matching the bridge schema.
	SegmentSource                  bool   `json:"segmentSource,omitempty"                  schema:"title=Segment Source Text,description=Segment the source text,default=true"`
	SegmentTarget                  bool   `json:"segmentTarget,omitempty"                  schema:"title=Segment Target Text,description=Segment existing target text"`
	SourceSrxPath                  string `json:"sourceSrxPath,omitempty"                  schema:"title=Source SRX Rules Path,description=Path to an SRX 2.0 rules file for source text (srx engine)"`
	TargetSrxPath                  string `json:"targetSrxPath,omitempty"                  schema:"title=Target SRX Rules Path,description=Path to an SRX 2.0 rules file for target text (srx engine)"`
	OverwriteSegmentation          bool   `json:"overwriteSegmentation,omitempty"          schema:"title=Overwrite Existing Segmentation,description=Re-segment already-segmented blocks replacing previous segmentation"`
	TreatIsolatedCodesAsWhitespace bool   `json:"treatIsolatedCodesAsWhitespace,omitempty" schema:"title=Treat Isolated Codes as Whitespace,description=Treat isolated inline codes as whitespace during segmentation"`
	TrimLeadingWS                  bool   `json:"trimLeadingWhitespace,omitempty"          schema:"title=Trim Leading Whitespace,description=Exclude leading whitespace from each segment span"`
	TrimTrailingWS                 bool   `json:"trimTrailingWhitespace,omitempty"         schema:"title=Trim Trailing Whitespace,description=Exclude trailing whitespace from each segment span"`
	// RenumberCodes is honored at bilingual projection time, where standalone
	// segments are materialized; in the overlay model the runs are never
	// rewritten, so it is a no-op for overlay production.
	RenumberCodes bool `json:"renumberCodes,omitempty" schema:"title=Renumber Code IDs,description=Renumber inline code IDs when materializing segments to a bilingual format"`

	// LLM / SaT engine parameters.
	Provider    string  `json:"provider,omitempty"    schema:"title=LLM Provider,description=AI provider id for the llm engine"`
	Model       string  `json:"model,omitempty"       schema:"title=Model,description=Model name for the llm or sat engine"`
	Credential  string  `json:"credential,omitempty"  schema:"title=Credential,description=Stored credential name for the llm engine"`
	Instruction string  `json:"instruction,omitempty" schema:"title=Chunking Instruction,description=Optional guidance for the llm engine"`
	SatModel    string  `json:"satModel,omitempty"    schema:"title=SaT Model,description=SaT model for the sat engine (e.g. sat-3l-sm, sat-12l-sm)"`
	Threshold   float64 `json:"threshold,omitempty"   schema:"title=Boundary Threshold,description=Boundary probability threshold for the sat engine (0 = model default)"`

	// Resolved at runtime by the CLI/host, not exposed as flags.
	APIKey     string `json:"-" schema:"-"`
	BaseURL    string `json:"-" schema:"-"`
	PluginPath string `json:"-" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *SegmentationConfig) ToolName() string { return "segmentation" }

// Reset restores default values.
func (c *SegmentationConfig) Reset() {
	*c = SegmentationConfig{SegmentSource: true}
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

// maskOptions maps the tool config to the shared masking options.
func (c *SegmentationConfig) maskOptions() segment.MaskOptions {
	return segment.MaskOptions{
		TreatIsolatedCodesAsWhitespace: c.TreatIsolatedCodesAsWhitespace,
		TrimLeadingWS:                  c.TrimLeadingWS,
		TrimTrailingWS:                 c.TrimTrailingWS,
	}
}

// engineConfig builds the segment.Config for a side, given that side's SRX path.
func (c *SegmentationConfig) engineConfig(srxPath string) segment.Config {
	return segment.Config{
		Mask:        c.maskOptions(),
		SrxPath:     srxPath,
		Provider:    c.Provider,
		Model:       c.Model,
		APIKey:      c.APIKey,
		BaseURL:     c.BaseURL,
		Instruction: c.Instruction,
		SatModel:    c.SatModel,
		Threshold:   c.Threshold,
		PluginPath:  c.PluginPath,
	}
}

// SegmentationSchema returns the auto-generated schema for the segmentation tool.
func SegmentationSchema() *schema.ComponentSchema {
	cfg := &SegmentationConfig{}
	cfg.Reset()
	return schema.FromStruct(cfg, schema.ToolMeta{
		ID:          "segmentation",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Segmentation",
		Description: "Split source text into sentence or chunk segments (stand-off overlay)",
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

// SegmentationTool produces a stand-off segmentation overlay for a Block by
// delegating to a pluggable segment.Segmenter chosen by config. Provider-backed
// engines (llm, sat) honor cancellation via the dispatch context the annotate
// handler reads from its view; the segmentation itself is a read-only
// annotation (it never rewrites runs).
type SegmentationTool struct {
	tool.BaseTool
	cfg *SegmentationConfig

	srcOnce sync.Once
	srcSeg  segment.Segmenter
	srcErr  error
	tgtOnce sync.Once
	tgtSeg  segment.Segmenter
	tgtErr  error
}

// NewSegmentationTool creates a segmentation tool that splits Block content
// into a stand-off segmentation overlay using the configured engine (SRX by
// default). The source runs are never rewritten, so segmentation is reversible
// — dropping the overlay restores the unsegmented content.
func NewSegmentationTool(cfg *SegmentationConfig) *SegmentationTool {
	if cfg == nil {
		cfg = &SegmentationConfig{}
		cfg.Reset()
	}
	// Default: segment source if neither side is explicitly enabled.
	if !cfg.SegmentSource && !cfg.SegmentTarget {
		cfg.SegmentSource = true
	}

	t := &SegmentationTool{cfg: cfg}
	t.ToolName = "segmentation"
	t.ToolDescription = "Splits content into a stand-off segmentation overlay"
	t.Cfg = cfg
	t.Annotate = t.annotate
	return t
}

// sourceSegmenter / targetSegmenter build (once) the segmenter for each side.
// The inline Rules list, when present, overrides the named engine.
func (t *SegmentationTool) sourceSegmenter() (segment.Segmenter, error) {
	t.srcOnce.Do(func() { t.srcSeg, t.srcErr = t.cfg.buildSegmenter(t.cfg.SourceSrxPath) })
	return t.srcSeg, t.srcErr
}

func (t *SegmentationTool) targetSegmenter() (segment.Segmenter, error) {
	t.tgtOnce.Do(func() { t.tgtSeg, t.tgtErr = t.cfg.buildSegmenter(t.cfg.TargetSrxPath) })
	return t.tgtSeg, t.tgtErr
}

func (c *SegmentationConfig) buildSegmenter(srxPath string) (segment.Segmenter, error) {
	if len(c.Rules) > 0 {
		compiled, err := compileRules(c.Rules)
		if err != nil {
			return nil, fmt.Errorf("segmentation rules: %w", err)
		}
		return &rulesSegmenter{compiled: compiled, mask: c.maskOptions()}, nil
	}
	return segment.NewEngine(c.Engine, c.engineConfig(srxPath))
}

// layerFor resolves the overlay layer for a side: the explicit config layer,
// or the engine's natural layer.
func (t *SegmentationTool) layerFor(seg segment.Segmenter) string {
	if t.cfg.Layer != "" {
		return t.cfg.Layer
	}
	return seg.Layer()
}

func (t *SegmentationTool) annotate(v tool.BlockView) error {
	if !v.Translatable() {
		return nil
	}
	cfg := t.cfg

	if cfg.SegmentSource {
		seg, err := t.sourceSegmenter()
		if err != nil {
			return err
		}
		layer := t.layerFor(seg)
		if v.SegmentationLayerFor(nil, layer) == nil || cfg.OverwriteSegmentation {
			spans, err := seg.Segment(v.Context(), v.SourceRuns(), v.SourceLocale())
			if err != nil {
				return fmt.Errorf("segmentation (source): %w", err)
			}
			v.SetSegmentationLayer(nil, layer, spans)
			if layer == segment.LayerSentence {
				v.SetProperty(PropSegmentCount, strconv.Itoa(len(spans)))
			}
		}
	}

	if cfg.SegmentTarget && !cfg.TargetLocale.IsEmpty() && v.HasTarget(cfg.TargetLocale) {
		seg, err := t.targetSegmenter()
		if err != nil {
			return err
		}
		layer := t.layerFor(seg)
		key := model.Variant(cfg.TargetLocale)
		if v.SegmentationLayerFor(&key, layer) == nil || cfg.OverwriteSegmentation {
			spans, err := seg.Segment(v.Context(), v.TargetRuns(cfg.TargetLocale), cfg.TargetLocale)
			if err != nil {
				return fmt.Errorf("segmentation (target): %w", err)
			}
			v.SetSegmentationLayer(&key, layer, spans)
		}
	}

	return nil
}

// ── Inline-rules fallback engine ────────────────────────────────────────────
//
// A lightweight regex break/no-break segmenter for the optional inline Rules
// list. It runs over the masked flattening (so it is code-aware and
// run-anchored like the registered engines) but uses Go's RE2 regexp, so it
// cannot express the lookahead/lookbehind that full SRX rule files need — for
// those, use the srx engine.

type rulesSegmenter struct {
	compiled []compiledRule
	mask     segment.MaskOptions
}

func (r *rulesSegmenter) Layer() string { return segment.LayerSentence }

func (r *rulesSegmenter) Segment(_ context.Context, runs []model.Run, _ model.LocaleID) ([]model.Span, error) {
	fl := segment.Flatten(runs, r.mask)
	return fl.Spans(segmentBoundaries(fl.Text(), r.compiled)), nil
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

// segmentBoundaries returns the sorted rune offsets at which the text breaks
// into segments (exclusive of 0 and the end), applying the rules with
// first-decision-wins per position.
func segmentBoundaries(text string, rules []compiledRule) []int {
	if text == "" {
		return nil
	}
	breakPoints := make(map[int]bool) // byte offset -> isBreak
	for _, rule := range rules {
		if rule.before == nil {
			continue
		}
		for _, loc := range rule.before.FindAllStringIndex(text, -1) {
			breakPos := loc[1]
			if breakPos > len(text) {
				breakPos = len(text)
			}
			if rule.after != nil && !rule.after.MatchString(text[breakPos:]) {
				continue
			}
			if _, exists := breakPoints[breakPos]; !exists {
				breakPoints[breakPos] = rule.isBreak
			}
		}
	}
	var byteOffsets []int
	for pos, isBreak := range breakPoints {
		if isBreak && pos > 0 && pos < len(text) {
			byteOffsets = append(byteOffsets, pos)
		}
	}
	out := make([]int, len(byteOffsets))
	for i, b := range byteOffsets {
		out[i] = utf8.RuneCountInString(text[:b])
	}
	return out
}
