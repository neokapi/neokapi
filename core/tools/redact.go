package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Detection backend names for RedactConfig.Detectors.
const (
	DetectRules    = "rules"    // literal terms + regex from a rules file/inline
	DetectEntities = "entities" // EntityAnnotations already on the block (from entity-extract)
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
	EntityTypes []string `json:"entityTypes,omitempty" schema:"title=Entity Categories,description=Entity categories to redact: person, org, product, location, date, time, currency, measurement, role, other. Setting any enables 'entities' detection (which needs an upstream NER step such as entity-extract)."`

	// Rules supplies rules inline as an alternative (or addition) to RulesPath.
	Rules []redaction.Rule `json:"rules,omitempty" schema:"-"`

	// VaultPath, when set, switches to external mode: originals are written
	// to a sidecar [redaction.FileVault] at this path instead of riding on
	// the block as an in-process annotation. Used by extract → merge.
	VaultPath string `json:"vaultPath,omitempty" schema:"-"`

	// SourceLocale is recorded with each stored value; informational.
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`

	// WholeBlock redacts every block, translatable or not, and each target
	// locale's runs beside the source. A run whose next tool reads the whole
	// block (term extraction, voice inference, a check that reads the
	// translation) needs it; unredact restores both sides.
	WholeBlock bool `json:"wholeBlock,omitempty" schema:"-"`
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
	c.WholeBlock = false
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
	for _, et := range c.EntityTypes {
		if _, ok := redaction.NormalizeEntityCategory(et); !ok {
			return fmt.Errorf("redact: unknown entity category %q (want one of: %s)", et, strings.Join(redaction.EntityCategories, ", "))
		}
	}
	return nil
}

// entityDetectionEnabled reports whether the config selects the entity detector
// — explicitly via Detectors, or implicitly by naming entity categories to
// redact (e.g. "redact dates"). When true, redact needs an upstream entity
// overlay (see ResolveRedactContract).
func (c *RedactConfig) entityDetectionEnabled() bool {
	if slices.Contains(c.Detectors, DetectEntities) {
		return true
	}
	return len(c.EntityTypes) > 0
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
		// The redacted content is the point, so `kapi exec redact` needs
		// somewhere to put it. Without WritesOutput the command has no -o at all
		// ("unknown shorthand flag: 'o'"), so it redacted in memory, exited 0, and
		// wrote nothing — #1471 §3, caught here by the invariant that generalizes
		// it (#1476).
		WritesOutput: true,
		// Recoverable transformer (AD-006): vaults the originals it removes and
		// restores them via unredact, so the placement pass holds it to the
		// remote-egress rule.
		Recoverable: true,
		// Transformer: rewrites the source. Optionally upgrades on an entity
		// overlay; emits the rewritten source plus the in-process secret
		// annotation that unredact restores from.
		Consumes: []schema.IOPort{{Type: string(model.OverlayEntity), Side: model.SideSource, Optional: true}},
		Produces: []schema.IOPort{
			schema.Port(schema.PortSource, model.SideSource),
			schema.Port(redaction.SecretAnnotationKey, model.SideSource),
		},
	})
}

// ResolveRedactContract refines redact's IO contract from its config: when
// entity detection is enabled (the "entities" detector, or any entityTypes set),
// the upstream entity overlay becomes a *required* input. So a flow that redacts
// entities without an entity producer upstream (an NER step such as
// entity-extract, or a store/interchange source carrying entities) fails
// data-flow validation instead of silently redacting nothing — which would leak
// the very content it was meant to hide to the downstream translator. With only
// rule-based detection, redact reads no upstream port and the contract is
// unchanged. Registered via ToolRegistry.SetContractResolver; see AD-020/AD-006.
func ResolveRedactContract(config map[string]any, base registry.ToolInfo) registry.ToolInfo {
	var cfg RedactConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return base
	}
	if !cfg.entityDetectionEnabled() {
		return base
	}
	consumes := make([]schema.IOPort, len(base.Consumes))
	copy(consumes, base.Consumes)
	for i := range consumes {
		if consumes[i].Type == string(model.OverlayEntity) && consumes[i].Side == model.SideSource {
			consumes[i].Optional = false
		}
	}
	base.Consumes = consumes
	return base
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

	// Naming entity categories to redact (e.g. "redact dates") implies entity
	// detection, so the user doesn't also have to list the "entities" detector.
	if len(cfg.EntityTypes) > 0 {
		t.useEntities = true
	}
	cats := cfg.EntityTypes
	if len(cats) == 0 {
		cats = defaultEntityCategories
	}
	for _, c := range cats {
		// Normalize so "Dates"/"organization"/"entity:person" all map to a
		// canonical category; unknown names are rejected by Validate upstream.
		if canon, ok := redaction.NormalizeEntityCategory(c); ok {
			t.entityCats[canon] = true
		}
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
	// Transform producer (AD-006): redact detects sensitive spans and returns
	// an edit plan — the redacted runs, the span mapping, and the originals to
	// vault. The framework applier rewrites the source, rebases surviving
	// overlays, and hands the secrets to VaultSecrets atomically; a later
	// unredact restores the originals from the recovery record.
	base.Transform = t.transform
	base.VaultSecrets = t.vaultSecrets
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

// transform is the read-only edit producer: it detects sensitive spans and
// returns the redacted runs, the span mapping (so the applier rebases the
// surviving source overlays — a term tag outside the redacted spans follows
// the rewrite, while spans overlapping a redaction, including the consumed
// entity spans, are dropped), and the originals to vault.
func (t *RedactTool) transform(v tool.BlockView) (tool.EditPlan, error) {
	if !v.Translatable() && !t.cfg.WholeBlock {
		return tool.EditPlan{}, nil
	}
	var plan tool.EditPlan
	newRuns, records, edits, err := t.redactRuns(v, v.SourceRuns(), v.SourceLocale(), true, t.opts)
	if err != nil {
		return tool.EditPlan{}, err
	}
	if len(records) > 0 {
		plan.NewRuns, plan.Edits = newRuns, edits
		plan.Secrets = appendSecrets(plan.Secrets, records, "")
	}
	if t.cfg.WholeBlock {
		for _, locale := range v.TargetLocales() {
			// A token of its own per locale, so a target's originals never
			// share a key with the source's on the block.
			opts := t.opts
			opts.TokenPrefix = tokenPrefix(opts) + string(locale) + "-"
			tr, recs, _, err := t.redactRuns(v, v.TargetRuns(locale), locale, false, opts)
			if err != nil {
				return tool.EditPlan{}, err
			}
			if len(recs) > 0 {
				plan.SetTarget(locale, tr)
				plan.Secrets = appendSecrets(plan.Secrets, recs, locale)
			}
		}
	}
	return plan, nil
}

func tokenPrefix(opts redaction.RedactOptions) string {
	if opts.TokenPrefix == "" {
		return redaction.DefaultTokenPrefix
	}
	return opts.TokenPrefix
}

// redactRuns detects the sensitive spans in one run sequence and returns the
// redacted runs with what they withheld, and the edits that map the old text
// to the new. Detection reads the sequence's own text runs, the coordinates
// redaction.Redact carves in, and each plural form and select case is
// redacted the same way under a token prefix of its own. A sequence holding a
// plural or a select reports one edit over its whole text, since the text of
// a form has no position in the sequence's own. Entity overlays anchor to the
// source's own text, so they drive the top of the source only.
func (t *RedactTool) redactRuns(v tool.BlockView, runs []model.Run, locale model.LocaleID, source bool, opts redaction.RedactOptions) ([]model.Run, []redaction.Redacted, []model.RunEdit, error) {
	if len(runs) == 0 {
		return nil, nil, nil, nil
	}
	text := redaction.TextOf(runs)
	var matches []redaction.Match
	if t.rules != nil {
		ms, err := t.rules.Detect(v.Context(), text, locale)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("redact: %w", err)
		}
		matches = append(matches, ms...)
	}
	if t.useEntities && source {
		matches = append(matches, t.entityMatches(v, text)...)
	}
	matches = redaction.NormalizeMatches(matches)
	out, records, edits := runs, []redaction.Redacted(nil), []model.RunEdit(nil)
	if len(matches) > 0 {
		out, records, edits = redaction.Redact(runs, matches, opts)
	}
	if !model.HasStructuredRuns(out) {
		if len(records) == 0 {
			return nil, nil, nil, nil
		}
		return out, records, edits, nil
	}
	tree := slices.Clone(out)
	for i, r := range tree {
		prefix := fmt.Sprintf("%s%d.", tokenPrefix(opts), i)
		switch {
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for form, fr := range r.Plural.Forms {
				fopts := opts
				fopts.TokenPrefix = prefix + string(form) + "-"
				nr, recs, _, err := t.redactRuns(v, fr, locale, false, fopts)
				if err != nil {
					return nil, nil, nil, err
				}
				if len(recs) == 0 {
					nr = fr
				}
				forms[form] = nr
				records = append(records, recs...)
			}
			tree[i] = model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}}
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for key, cr := range r.Select.Cases {
				copts := opts
				copts.TokenPrefix = prefix + key + "-"
				nr, recs, _, err := t.redactRuns(v, cr, locale, false, copts)
				if err != nil {
					return nil, nil, nil, err
				}
				if len(recs) == 0 {
					nr = cr
				}
				cases[key] = nr
				records = append(records, recs...)
			}
			tree[i] = model.Run{Select: &model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}}
		}
	}
	if len(records) == 0 {
		return nil, nil, nil, nil
	}
	return tree, records, tool.FullSpanEdit(runs, tree), nil
}

// appendSecrets adds the originals redaction withheld from one edition:
// locale names the target, empty for the source.
func appendSecrets(secrets []tool.Secret, records []redaction.Redacted, locale model.LocaleID) []tool.Secret {
	for _, r := range records {
		secrets = append(secrets, tool.Secret{Token: r.Token, Category: r.Category, Disp: r.Disp, Original: r.Original, Locale: locale})
	}
	return secrets
}

// vaultSecrets is the applier's secret sink: it persists the recovery records
// to the sidecar vault in external mode, or to an in-process SecretAnnotation
// on the block otherwise. The annotation is the on-block recovery record a
// later unredact reads to restore originals, with each edition as redact read
// it, so unredact can put an edition nothing changed back as it was. The
// applier calls it before the rewrite lands, so a vault failure aborts the
// transform fail-closed.
func (t *RedactTool) vaultSecrets(v tool.BlockView, secrets []tool.Secret) error {
	toValue := func(s tool.Secret) redaction.RedactedValue {
		locale := s.Locale
		if locale.IsEmpty() {
			locale = v.SourceLocale()
		}
		return redaction.RedactedValue{
			Token:    s.Token,
			Category: s.Category,
			Disp:     s.Disp,
			Original: s.Original,
			Locale:   locale,
			BlockID:  v.ID(),
		}
	}
	if t.vault != nil {
		for _, s := range secrets {
			if err := t.vault.Put(toValue(s)); err != nil {
				return err
			}
		}
		return nil
	}
	ann := &redaction.SecretAnnotation{Values: make(map[string]redaction.RedactedValue, len(secrets))}
	for _, s := range secrets {
		ann.Values[s.Token] = toValue(s)
		if s.Locale.IsEmpty() {
			ann.Source = slices.Clone(v.SourceRuns())
			continue
		}
		if ann.Targets == nil {
			ann.Targets = map[model.LocaleID]model.Edition{}
		}
		if _, seen := ann.Targets[s.Locale]; !seen {
			if ed := v.Target(s.Locale); ed != nil {
				read := *ed
				read.Runs = slices.Clone(ed.Runs)
				ann.Targets[s.Locale] = read
			}
		}
	}
	v.Annotate(redaction.SecretAnnotationKey, ann)
	return nil
}

// entityMatches turns the block's EntityAnnotations into redaction matches,
// keeping only the configured categories. Offsets reported by the extractor
// are reconciled against the source text so byte spans are exact.
func (t *RedactTool) entityMatches(v tool.BlockView, text string) []redaction.Match {
	var out []redaction.Match
	for _, span := range v.OverlaySpans(model.OverlayEntity) {
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
