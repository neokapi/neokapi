package tools

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/neokapi/neokapi/core/tool"

	// Register the built-in segmentation engines so the segment tool can
	// select them by name. SRX (the default) is pure Go; UAX-29 is ICU-backed
	// where cgo is available and silently absent otherwise. The LLM engine
	// registers from core/ai/tools when linked; ML/native engines (e.g. SaT)
	// are plugin-provided and registered by the host at discovery time.
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

// SegmentationConfig is the segmentation tool's common configuration: which
// engine to run and how the resulting overlay is scoped and trimmed. The
// selected engine's own parameters (SRX rules file, LLM provider/model, SaT
// model/threshold) are not fields here — each engine owns its parameter schema
// and the form composes them (see [SegmentationSchema]). At runtime the
// engine-specific values arrive in EngineParams and are decoded into the
// engine's own config.
type SegmentationConfig struct {
	TargetLocale model.LocaleID     `json:"targetLocale,omitempty" schema:"-"`
	Rules        []SegmentationRule `json:"rules,omitempty"        schema:"-"`

	// Engine selects the segmenter backend by registry name (srx is the default).
	// The available engines and their own parameters come from the segment engine
	// registry; an inline Rules list overrides Engine.
	Engine string `json:"engine,omitempty" schema:"title=Segmentation Engine,description=Which segmenter to use; the default needs no configuration,group=segmentation,order=0"`
	// Layer names the segmentation overlay layer. Empty is the primary
	// sentence layer (the one bilingual formats project); named layers such as
	// llm-chunk coexist alongside it. Empty defers to the engine's natural
	// layer (sentence for srx/uax29/sat, llm-chunk for llm).
	Layer string `json:"layer,omitempty" schema:"title=Overlay Layer,description=Segmentation overlay layer name; empty uses the engine's natural layer,group=segmentation,order=10"`

	SegmentSource bool `json:"segmentSource,omitempty" schema:"title=Segment Source Text,description=Segment the source text,default=true,group=segmentation,order=20"`
	SegmentTarget bool `json:"segmentTarget,omitempty" schema:"title=Segment Target Text,description=Segment existing target text,group=segmentation,order=30"`

	OverwriteSegmentation          bool `json:"overwriteSegmentation,omitempty"          schema:"title=Overwrite Existing Segmentation,description=Re-segment already-segmented blocks replacing previous segmentation,group=boundaries,order=10"`
	TreatIsolatedCodesAsWhitespace bool `json:"treatIsolatedCodesAsWhitespace,omitempty" schema:"title=Treat Isolated Codes as Whitespace,description=Treat isolated inline codes as whitespace during segmentation,group=boundaries,order=20"`
	TrimLeadingWS                  bool `json:"trimLeadingWhitespace,omitempty"          schema:"title=Trim Leading Whitespace,description=Exclude leading whitespace from each segment span,default=true,group=boundaries,order=30"`
	TrimTrailingWS                 bool `json:"trimTrailingWhitespace,omitempty"         schema:"title=Trim Trailing Whitespace,description=Exclude trailing whitespace from each segment span,default=true,group=boundaries,order=40"`
	// RenumberCodes is honored at bilingual projection time, where standalone
	// segments are materialized; in the overlay model the runs are never
	// rewritten, so it is a no-op for overlay production.
	RenumberCodes bool `json:"renumberCodes,omitempty" schema:"title=Renumber Code IDs,description=Renumber inline code IDs when materializing segments to a bilingual format,group=boundaries,order=50"`

	// EngineParams carries the selected engine's own parameters, captured from
	// the unified config map and decoded into the engine's [segment.EngineConfig]
	// at build time. Not a form field — the composed schema contributes the
	// engine fields directly; direct constructors may set this map.
	EngineParams map[string]any `json:"-" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *SegmentationConfig) ToolName() string { return "segmentation" }

// Reset restores default values.
func (c *SegmentationConfig) Reset() {
	// Trim leading/trailing whitespace by default so every engine yields clean
	// sentence segments (inter-sentence whitespace left uncovered) — consistent
	// with Okapi and stable for TM keys. The SRX engine also honors trim from its
	// ruleset header; this default extends the same behavior to UAX-29/SaT/LLM.
	*c = SegmentationConfig{SegmentSource: true, TrimLeadingWS: true, TrimTrailingWS: true}
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

// segmentationToolMeta is the tool metadata shared by the registry and the
// exported schema accessor.
func segmentationToolMeta() schema.ToolMeta {
	return toolMeta("segmentation", "Segmentation", schema.CategoryTextProcessing,
		withTags("text-processing"), withAliases("segment"), withWritesOutput(),
		withCardinality(schema.Monolingual),
		withProduces(srcF(model.OverlaySegmentation), tgtF(model.OverlaySegmentation)))
}

// segmentationCommonSchema is the group's shared config: the engine selector
// plus the scope and boundary-handling options common to every engine.
func segmentationCommonSchema() *schema.ComponentSchema {
	cfg := &SegmentationConfig{}
	cfg.Reset()
	base := schema.FromStruct(cfg, segmentationToolMeta())
	base.Description = "Split source text into sentence or chunk segments (stand-off overlay)"
	for i := range base.Groups {
		switch base.Groups[i].ID {
		case "segmentation":
			base.Groups[i].Label = "Segmentation"
		case "boundaries":
			base.Groups[i].Label = "Boundary handling"
			collapse := true
			base.Groups[i].Collapsible = &collapse
			base.Groups[i].Collapsed = true
		}
	}
	return base
}

// segmentationMembers maps the registered segment engines to tool-group members.
// The engine list (selector + per-engine config) is whatever is linked into the
// binary plus any plugin-provided engines — the tool carries no engine-specific
// knowledge.
func segmentationMembers() []registry.ToolGroupMember {
	descs := segment.Descriptors()
	ms := make([]registry.ToolGroupMember, 0, len(descs))
	for _, d := range descs {
		ms = append(ms, registry.ToolGroupMember{
			Name: d.Name, Label: d.Label, Description: d.Description, Schema: d.Schema,
		})
	}
	return ms
}

// SegmentationSchema returns the composed (flat) projection of the segmentation
// group — common options + an engine selector whose chosen engine reveals only
// its own parameters. This is the view CLI flags, docs, and MCP consume; the UI
// uses the group + per-member schemas (master-detail) instead.
func SegmentationSchema() *schema.ComponentSchema {
	members := segmentationMembers()
	variants := make([]schema.Variant, len(members))
	for i, m := range members {
		variants[i] = schema.Variant{Name: m.Name, Label: m.Label, Description: m.Description, Params: m.Schema, When: m.When}
	}
	return schema.ComposeVariants(segmentationCommonSchema(), "engine", segment.DefaultEngine, variants)
}

// RegisterSegmentation registers (or re-registers) the segmentation tool group.
// The host calls it again after plugin-provided segmenters register, so the
// member list reflects them (the group is otherwise built once, before plugin
// discovery).
func RegisterSegmentation(reg *registry.ToolRegistry) {
	reg.RegisterGroup(registry.ToolGroupDef{
		Name:          "segmentation",
		Discriminator: "engine",
		Default:       segment.DefaultEngine,
		Common:        segmentationCommonSchema(),
		Members:       segmentationMembers(),
		ConfigFactory: NewSegmentationFromConfig,
	})
}

// NewSegmentationFromConfig creates a segmentation tool from a config map. The
// engine-specific keys ride along in the same map and are decoded into the
// chosen engine's own config when the segmenter is built.
func NewSegmentationFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := &SegmentationConfig{}
	cfg.Reset()
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("segmentation config: %w", err)
	}
	cfg.EngineParams = config
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

	once sync.Once
	seg  segment.Segmenter
	err  error
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

// segmenter builds (once) the engine for this tool. One engine serves both
// source and target: SRX rules are locale-keyed, so the per-call locale selects
// the right language rules for each side. The inline Rules list, when present,
// overrides the named engine.
func (t *SegmentationTool) segmenter() (segment.Segmenter, error) {
	t.once.Do(func() { t.seg, t.err = t.cfg.buildSegmenter() })
	return t.seg, t.err
}

func (c *SegmentationConfig) buildSegmenter() (segment.Segmenter, error) {
	if len(c.Rules) > 0 {
		compiled, err := compileRules(c.Rules)
		if err != nil {
			return nil, fmt.Errorf("segmentation rules: %w", err)
		}
		return &rulesSegmenter{compiled: compiled, mask: c.maskOptions()}, nil
	}

	desc, ok := segment.Lookup(c.Engine)
	if !ok {
		return nil, fmt.Errorf("%w: %q (have: %v)", segment.ErrEngineUnavailable, c.Engine, segment.Engines())
	}
	params := c.EngineParams
	if params == nil {
		params = map[string]any{}
	}
	return desc.New(segment.BaseConfig{Mask: c.maskOptions()}, params)
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
		seg, err := t.segmenter()
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
		seg, err := t.segmenter()
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
			breakPos := min(loc[1], len(text))
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
