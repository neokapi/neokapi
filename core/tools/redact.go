package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Detection backend names for RedactConfig.Detectors.
const (
	DetectRules    = "rules"    // literal terms + regex from a rules file/inline
	DetectEntities = "entities" // EntityAnnotations already on the block (from ai-entity-extract)
)

// defaultEntityCategories are the entity categories redacted by default when
// "entities" detection is enabled. Dates, times, currencies and measurements
// are intentionally excluded — they need locale-specific formatting, not
// hiding.
var defaultEntityCategories = []string{
	redaction.CategoryPerson,
	redaction.CategoryOrg,
	redaction.CategoryProduct,
	redaction.CategoryLocation,
}

// RedactConfig configures the redact tool.
type RedactConfig struct {
	Detectors   []string `json:"detectors,omitempty" schema:"title=Detectors,description=Detection backends to run: rules and/or entities"`
	RulesPath   string   `json:"rulesPath,omitempty" schema:"title=Rules File,description=Path to a redaction rules YAML file"`
	Placeholder string   `json:"placeholder,omitempty" schema:"title=Placeholder Template,description=Visible stand-in template; supports {category} and {n}"`
	EntityTypes []string `json:"entityTypes,omitempty" schema:"title=Entity Categories,description=Entity categories to redact when 'entities' detection is enabled"`

	// Rules supplies rules inline as an alternative (or addition) to RulesPath.
	Rules []redaction.Rule `json:"rules,omitempty" schema:"-"`

	// VaultPath, when set, switches to external mode: originals are written
	// to a sidecar [redaction.FileVault] at this path instead of riding on
	// the block as an in-process annotation. Used by extract → merge.
	VaultPath string `json:"vaultPath,omitempty" schema:"-"`

	// SourceLocale is recorded with each stored value; informational.
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *RedactConfig) ToolName() string { return "redact" }

// Reset restores default values.
func (c *RedactConfig) Reset() {
	c.Detectors = []string{DetectRules}
	c.RulesPath = ""
	c.Placeholder = redaction.DefaultPlaceholder
	c.EntityTypes = nil
	c.Rules = nil
	c.VaultPath = ""
	c.SourceLocale = ""
}

// Validate checks configuration validity.
func (c *RedactConfig) Validate() error {
	for _, d := range c.Detectors {
		switch d {
		case DetectRules, DetectEntities:
		default:
			return fmt.Errorf("redact: unknown detector %q (want %q or %q)", d, DetectRules, DetectEntities)
		}
	}
	return nil
}

// RedactSchema returns the auto-generated schema for the redact tool.
func RedactSchema() *schema.ComponentSchema {
	return schema.FromStruct(&RedactConfig{}, schema.ToolMeta{
		ID:          "redact",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Redact",
		Description: "Replace sensitive spans with protected placeholders before processing",
		Tags:        []string{"security", "redaction"},
		Cardinality: schema.Monolingual,
	})
}

// RedactTool replaces sensitive source spans with protected redaction
// placeholders and stashes the originals locally — on the block (in-process)
// or in a sidecar vault (external). It never emits the original value into
// the rewritten content.
type RedactTool struct {
	*tool.BaseTool
	cfg         *RedactConfig
	rules       redaction.Detector // compiled rule detector; nil when unused
	useEntities bool
	entityCats  map[string]bool
	opts        redaction.RedactOptions
	vault       *redaction.FileVault // non-nil in external mode
}

// NewRedactFromConfig builds a redact tool from a config map.
func NewRedactFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg RedactConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("redact config: %w", err)
	}
	if len(cfg.Detectors) == 0 {
		cfg.Detectors = []string{DetectRules}
	}
	return NewRedactTool(&cfg)
}

// NewRedactTool builds a redact tool, compiling rules and opening the sidecar
// vault if configured.
func NewRedactTool(cfg *RedactConfig) (*RedactTool, error) {
	if cfg.Placeholder == "" {
		cfg.Placeholder = redaction.DefaultPlaceholder
	}

	t := &RedactTool{
		cfg:        cfg,
		entityCats: map[string]bool{},
		opts:       redaction.RedactOptions{Placeholder: cfg.Placeholder},
	}

	for _, d := range cfg.Detectors {
		switch d {
		case DetectRules:
			rules := cfg.Rules
			if cfg.RulesPath != "" {
				rf, err := redaction.LoadRulesFile(cfg.RulesPath)
				if err != nil {
					return nil, err
				}
				rules = append(rules, rf.Rules...)
				if cfg.Placeholder == redaction.DefaultPlaceholder && rf.Placeholder != "" {
					t.opts.Placeholder = rf.Placeholder
				}
			}
			det, err := redaction.NewRuleDetector(rules)
			if err != nil {
				return nil, err
			}
			t.rules = det
		case DetectEntities:
			t.useEntities = true
		}
	}

	cats := cfg.EntityTypes
	if len(cats) == 0 {
		cats = defaultEntityCategories
	}
	for _, c := range cats {
		t.entityCats[c] = true
	}

	if cfg.VaultPath != "" {
		v, err := redaction.OpenFileVault(cfg.VaultPath)
		if err != nil {
			return nil, err
		}
		t.vault = v
	}

	base := &tool.BaseTool{
		ToolName:        "redact",
		ToolDescription: "Replaces sensitive spans with protected placeholders before processing",
		Cfg:             cfg,
	}
	// Transform: redact rewrites the source (replacing sensitive spans) and
	// records recovery — on the block as a SecretAnnotation, or in a sidecar
	// vault. It is a recoverable source-transform: it must run before any
	// stand-off overlay is attached, and a later unredact restores the
	// originals from the recovery record.
	base.Transform = t.transform
	t.BaseTool = base
	return t, nil
}

// Process runs the streaming transform, then flushes the sidecar vault (if
// any) once the input is drained.
func (t *RedactTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	if err := t.BaseTool.Process(ctx, in, out); err != nil {
		return err
	}
	return t.Flush()
}

// Flush persists the sidecar vault, if the tool is in external mode. Callers
// that drive the tool via Apply directly (rather than Process) must call Flush
// when done.
func (t *RedactTool) Flush() error {
	if t.vault != nil {
		return t.vault.Flush()
	}
	return nil
}

func (t *RedactTool) transform(v tool.SourceView) error {
	if !v.Translatable() {
		return nil
	}
	runs := v.SourceRuns()
	if len(runs) == 0 {
		return nil
	}
	text := v.SourceText()

	var matches []redaction.Match
	if t.rules != nil {
		ms, err := t.rules.Detect(v.Context(), text, v.SourceLocale())
		if err != nil {
			return fmt.Errorf("redact: %w", err)
		}
		matches = append(matches, ms...)
	}
	if t.useEntities {
		matches = append(matches, t.entityMatches(v, text)...)
	}
	matches = redaction.NormalizeMatches(matches)
	if len(matches) == 0 {
		return nil
	}

	newRuns, records := redaction.Redact(runs, matches, t.opts)
	if len(records) == 0 {
		return nil
	}
	// Redaction rewrites the source, invalidating the run-anchored ranges of any
	// entity facet we consumed; drop it before mutating source so the spans
	// don't dangle (and so the immutability backstop's overlay check passes).
	if t.useEntities {
		v.RemoveFacet(model.FacetEntity)
	}
	v.SetSourceRuns(newRuns)
	t.store(v, records)
	return nil
}

// store persists the recovery records: to the sidecar vault in external mode,
// or to an in-process SecretAnnotation on the block otherwise. The annotation
// is the on-block recovery record a later unredact reads to restore originals.
func (t *RedactTool) store(v tool.SourceView, records []redaction.Redacted) {
	toValue := func(r redaction.Redacted) redaction.RedactedValue {
		return redaction.RedactedValue{
			Token:    r.Token,
			Category: r.Category,
			Disp:     r.Disp,
			Original: r.Original,
			Locale:   v.SourceLocale(),
			BlockID:  v.ID(),
		}
	}
	if t.vault != nil {
		for _, r := range records {
			_ = t.vault.Put(toValue(r))
		}
		return
	}
	ann := &redaction.SecretAnnotation{Values: make(map[string]redaction.RedactedValue, len(records))}
	for _, r := range records {
		ann.Values[r.Token] = toValue(r)
	}
	v.Annotate(redaction.SecretAnnotationKey, ann)
}

// entityMatches turns the block's EntityAnnotations into redaction matches,
// keeping only the configured categories. Offsets reported by the extractor
// are reconciled against the source text so byte spans are exact.
func (t *RedactTool) entityMatches(v tool.SourceView, text string) []redaction.Match {
	var out []redaction.Match
	for _, span := range v.FacetSpans(model.FacetEntity) {
		ea, ok := span.Value.(*model.EntityAnnotation)
		if !ok {
			continue
		}
		cat := entityCategory(ea.Type)
		if !t.entityCats[cat] {
			continue
		}
		hintStart, hintEnd := span.Range.ByteSpan(v.SourceRuns())
		start, end, ok := locateSpan(text, ea.Text, hintStart, hintEnd)
		if !ok {
			continue
		}
		out = append(out, redaction.Match{Start: start, End: end, Category: cat, Original: text[start:end]})
	}
	return out
}

// entityCategory maps a model.EntityType to a bare redaction category.
func entityCategory(t model.EntityType) string {
	bare := strings.TrimPrefix(string(t), model.EntityPrefix)
	if bare == "organization" {
		return redaction.CategoryOrg
	}
	return bare
}

// locateSpan returns exact byte offsets for an entity. It trusts the reported
// offsets when they slice to the expected text, then tries interpreting them
// as rune offsets, then falls back to locating the text by content.
func locateSpan(text, want string, start, end int) (int, int, bool) {
	if want == "" {
		return 0, 0, false
	}
	if start >= 0 && end <= len(text) && start < end && text[start:end] == want {
		return start, end, true
	}
	// Reported offsets may be rune-based; convert to byte offsets.
	runes := []rune(text)
	if start >= 0 && end <= len(runes) && start < end {
		bs := len(string(runes[:start]))
		be := len(string(runes[:end]))
		if be <= len(text) && text[bs:be] == want {
			return bs, be, true
		}
	}
	if i := strings.Index(text, want); i >= 0 {
		return i, i + len(want), true
	}
	return 0, 0, false
}
