// Package tools provides additional localization tools for the neokapi pipeline.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// accentMap maps ASCII characters to accented equivalents for pseudo-translation.
var accentMap = map[rune]rune{
	'a': '\u00e0', // a -> à
	'b': '\u0183', // b -> ƃ
	'c': '\u00e7', // c -> ç
	'd': '\u0111', // d -> đ
	'e': '\u00e9', // e -> é
	'f': '\u0192', // f -> ƒ
	'g': '\u011d', // g -> ĝ
	'h': '\u0125', // h -> ĥ
	'i': '\u00ee', // i -> î
	'j': '\u0135', // j -> ĵ
	'k': '\u0137', // k -> ķ
	'l': '\u013c', // l -> ļ
	'm': '\u1e3f', // m -> ḿ
	'n': '\u00f1', // n -> ñ
	'o': '\u00f6', // o -> ö
	'p': '\u00fe', // p -> þ
	'q': '\u01eb', // q -> ǫ
	'r': '\u0155', // r -> ŕ
	's': '\u0161', // s -> š
	't': '\u0163', // t -> ţ
	'u': '\u00fc', // u -> ü
	'v': '\u1e7d', // v -> ṽ
	'w': '\u0175', // w -> ŵ
	'x': '\u1e8b', // x -> ẋ
	'y': '\u00fd', // y -> ý
	'z': '\u017e', // z -> ž
	'A': '\u00c0', // A -> À
	'B': '\u0182', // B -> Ƃ
	'C': '\u00c7', // C -> Ç
	'D': '\u0110', // D -> Đ
	'E': '\u00c9', // E -> É
	'F': '\u0191', // F -> Ƒ
	'G': '\u011c', // G -> Ĝ
	'H': '\u0124', // H -> Ĥ
	'I': '\u00ce', // I -> Î
	'J': '\u0134', // J -> Ĵ
	'K': '\u0136', // K -> Ķ
	'L': '\u013b', // L -> Ļ
	'M': '\u1e3e', // M -> Ḿ
	'N': '\u00d1', // N -> Ñ
	'O': '\u00d6', // O -> Ö
	'P': '\u00de', // P -> Þ
	'Q': '\u01ea', // Q -> Ǫ
	'R': '\u0154', // R -> Ŕ
	'S': '\u0160', // S -> Š
	'T': '\u0162', // T -> Ţ
	'U': '\u00dc', // U -> Ü
	'V': '\u1e7c', // V -> Ṽ
	'W': '\u0174', // W -> Ŵ
	'X': '\u1e8a', // X -> Ẋ
	'Y': '\u00dd', // Y -> Ý
	'Z': '\u017d', // Z -> Ž
}

// PseudoConfig holds configuration for the pseudo-translation tool.
type PseudoConfig struct {
	ExpansionPercent int            `json:"expansionPercent,omitempty" schema:"title=Expansion Percent,description=Extra padding percentage added to simulate translation expansion (0 = no padding),default=0,min=0"`
	Prefix           string         `json:"prefix,omitempty"           schema:"title=Prefix,description=Characters prepended before each translated segment"`
	Suffix           string         `json:"suffix,omitempty"           schema:"title=Suffix,description=Characters appended after each translated segment"`
	TargetLocale     model.LocaleID `json:"targetLocale,omitempty"     schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *PseudoConfig) ToolName() string { return "pseudo-translate" }

// Reset restores default values.
func (c *PseudoConfig) Reset() {
	c.ExpansionPercent = 0
	c.Prefix = "\u2592 "
	c.Suffix = " \u2592"
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *PseudoConfig) Validate() error {
	if c.ExpansionPercent < 0 {
		return errors.New("pseudo: ExpansionPercent must be >= 0")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("pseudo: TargetLocale is required")
	}
	return nil
}

// PseudoTranslateSchema returns the auto-generated schema for the pseudo-translate tool.
func PseudoTranslateSchema() *schema.ComponentSchema {
	return schema.FromStruct(&PseudoConfig{}, schema.ToolMeta{
		ID:          "pseudo-translate",
		Category:    schema.CategoryTranslation,
		DisplayName: "Pseudo Translate",
		Description: "Generate pseudo-translations for localization testing",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewPseudoTranslateFromConfig creates a pseudo-translate tool from a config map.
func NewPseudoTranslateFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg PseudoConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("pseudo-translate config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewPseudoTranslateTool(&cfg), nil
}

// NewPseudoTranslateTool creates a new pseudo-translation tool.
// It replaces ASCII characters with accented equivalents, wraps text
// with brackets, and adds padding for string length testing.
// PseudoTranslateTool implements both tool.Tool (via embedded
// tool.BaseTool) and tool.SessionTool. When the executor opens a
// session, SessionProcess routes through it: for each block we
// check a `targets/<locale>` overlay first (skip if present), emit
// the pseudo-translated block, and write an overlay so downstream
// sessions can see the target without re-running the tool. When
// there's no session (pure streaming callers), BaseTool.Process
// handles the work unchanged.
type PseudoTranslateTool struct {
	*tool.BaseTool
	cfg *PseudoConfig
}

// Compile-time assertion: this type satisfies SessionTool.
var _ tool.SessionTool = (*PseudoTranslateTool)(nil)

// NewPseudoTranslateTool creates a new pseudo-translation tool.
// It replaces ASCII characters with accented equivalents, wraps text
// with brackets, and adds padding for string length testing.
func NewPseudoTranslateTool(cfg *PseudoConfig) *PseudoTranslateTool {
	if cfg.Prefix == "" && cfg.Suffix == "" {
		cfg.Prefix = "\u2592 "
		cfg.Suffix = " \u2592"
	}

	base := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Generates pseudo-translations for testing localization readiness",
		Cfg:             cfg,
		WritesTarget:    true,
	}
	base.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		return applyPseudo(part, cfg)
	}
	return &PseudoTranslateTool{BaseTool: base, cfg: cfg}
}

// applyPseudo runs the deterministic pseudo-translation on a block
// part. Factored out so SessionProcess can call it after checking
// the overlay cache.
func applyPseudo(part *model.Part, conf *PseudoConfig) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}
	if !block.Translatable {
		return part, nil
	}
	runs := block.SourceRuns()
	if len(runs) == 0 {
		return part, nil
	}
	if runsHaveInline(runs) {
		// Pseudo-translate text runs in place, leaving paired
		// codes and placeholders untouched (inline markup is
		// protected).
		targetRuns := pseudoTranslateRuns(runs, conf)
		block.SetTargetRuns(conf.TargetLocale, targetRuns)
	} else {
		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}
		pseudoText := pseudoTranslate(sourceText, conf)
		block.SetTargetText(conf.TargetLocale, pseudoText)
	}
	return part, nil
}

// SessionProcess reads prior targets/<locale> overlays to skip
// already-translated blocks, runs the pseudo translator, and writes
// the target back as an overlay so subsequent sessions can consult
// it.
func (t *PseudoTranslateTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	overlayKind := pseudoOverlayKind(t.cfg.TargetLocale)
	caps := sess.Capabilities()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if err := t.processOne(sess, caps.RandomAccess, overlayKind, part); err != nil {
				return err
			}
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (t *PseudoTranslateTool) processOne(
	sess blockstore.Session,
	randomAccess bool,
	overlayKind string,
	part *model.Part,
) error {
	block, ok := part.Resource.(*model.Block)
	if !ok || block == nil || !block.Translatable {
		// Pass through unchanged.
		_, err := applyPseudo(part, t.cfg)
		return err
	}
	hash := block.ID
	if hash == "" {
		_, err := applyPseudo(part, t.cfg)
		return err
	}

	// Consult existing overlay when the provider supports random
	// access. If one exists, hydrate the block from it and skip the
	// translator.
	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached pseudoCache
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Target != "" {
				block.SetTargetText(t.cfg.TargetLocale, cached.Target)
				return nil
			}
		}
	}

	if _, err := applyPseudo(part, t.cfg); err != nil {
		return err
	}

	// Write the freshly-computed target back as an overlay so future
	// runs can skip the work. Pure text cache — runs-level targets
	// round-trip through the block model itself.
	if target := block.TargetText(t.cfg.TargetLocale); target != "" {
		payload, err := json.Marshal(pseudoCache{Target: target})
		if err != nil {
			return fmt.Errorf("pseudo-translate: encode overlay: %w", err)
		}
		if err := sess.PutOverlay(blockstore.Overlay{
			Kind:      overlayKind,
			BlockHash: hash,
			Payload:   payload,
		}); err != nil {
			// Ignore read-only stores (e.g. FormatReaderStore) — the
			// in-flight block already carries the target; the overlay
			// write is best-effort caching for next time.
			if !errors.Is(err, blockstore.ErrReadOnly) {
				return fmt.Errorf("pseudo-translate: write overlay: %w", err)
			}
		}
	}
	return nil
}

// pseudoOverlayKind returns the "targets/<locale>" kind used for the
// overlay written by pseudo-translate. Shared with AI translate /
// MT translate so any locale target is discoverable under one key.
func pseudoOverlayKind(locale model.LocaleID) string {
	return "targets/" + string(locale)
}

// pseudoCache is the JSON payload stored in a pseudo-translate
// overlay. Small and focused; richer fields (runs, provenance) are
// a follow-up.
type pseudoCache struct {
	Target string `json:"target"`
}

// runsHaveInline reports whether the run sequence contains any
// non-text run (placeholder, paired code, subblock reference, or
// structured plural/select construct). Used by pseudo-translate to
// pick between the text-only fast path and the Run-walker path.
func runsHaveInline(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// pseudoTranslateRuns walks a run sequence and pseudo-translates
// the text of TextRuns in place, leaving every other run type
// unchanged. Also recurses into plural/select form runs so inline
// markup stays protected inside structured constructs.
//
// Wrapping (prefix/suffix) is applied to the WHOLE sequence exactly
// once — a block with placeholders renders as `▒ pre {ph} post ▒`,
// not `▒ pre ▒ {ph} ▒ post ▒`. The old per-run wrapping created
// false visual splices that looked like source-side concatenation
// bugs.
func pseudoTranslateRuns(runs []model.Run, cfg *PseudoConfig) []model.Run {
	if len(runs) == 0 {
		return runs
	}

	// Pass 1: accent-only transform per run. Each TextRun gets its
	// characters replaced; plural/select forms recurse (and pick up
	// their own wrapping there).
	out := make([]model.Run, 0, len(runs)+2)
	totalTextRunes := 0
	for _, r := range runs {
		switch {
		case r.Text != nil:
			accented := accentTransform(r.Text.Text)
			totalTextRunes += len([]rune(accented))
			out = append(out, model.Run{Text: &model.TextRun{Text: accented}})
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, v := range r.Plural.Forms {
				forms[k] = pseudoTranslateRuns(v, cfg)
			}
			out = append(out, model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}})
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, v := range r.Select.Cases {
				cases[k] = pseudoTranslateRuns(v, cfg)
			}
			out = append(out, model.Run{Select: &model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}})
		default:
			out = append(out, r)
		}
	}

	// Pass 2: append expansion padding to the last text run (or add
	// a new tail text run) so the padding sits inside the wrap.
	if cfg.ExpansionPercent > 0 && totalTextRunes > 0 {
		paddingLen := (totalTextRunes * cfg.ExpansionPercent) / 100
		if paddingLen > 0 {
			padding := " " + strings.Repeat("~", paddingLen)
			last := out[len(out)-1]
			if last.Text != nil {
				last.Text.Text += padding
				out[len(out)-1] = last
			} else {
				out = append(out, model.Run{Text: &model.TextRun{Text: padding}})
			}
		}
	}

	// Pass 3: wrap the whole sequence exactly once. Prefix goes in
	// a new leading text run, suffix in a new trailing text run.
	prefix, suffix := effectiveWrap(cfg)
	out = append([]model.Run{{Text: &model.TextRun{Text: prefix}}}, out...)
	out = append(out, model.Run{Text: &model.TextRun{Text: suffix}})
	return out
}

// pseudoTranslate applies pseudo-translation transformations to a
// single string (no placeholders). Used for simple blocks that
// have no inline runs; the runs path uses pseudoTranslateRuns.
func pseudoTranslate(text string, cfg *PseudoConfig) string {
	result := accentTransform(text)

	if cfg.ExpansionPercent > 0 {
		originalLen := len([]rune(result))
		paddingLen := (originalLen * cfg.ExpansionPercent) / 100
		if paddingLen > 0 {
			padding := strings.Repeat("~", paddingLen)
			result = result + " " + padding
		}
	}

	prefix, suffix := effectiveWrap(cfg)
	return prefix + result + suffix
}

// accentTransform replaces ASCII letters with their accented
// equivalents, leaving every other rune untouched. Shared by the
// string and runs paths so both produce identical glyphs.
//
// Content inside `{...}` placeholder markers is passed through
// verbatim — the braces + identifier are keys the runtime uses for
// parameter substitution; accenting them would break the lookup
// (e.g. `{count}` → `{çöüñţ}` means replaceAll("{count}", …) no
// longer matches). ICU-style pluralization patterns like
// `{count, plural, one {# step} other {# steps}}` also come
// through correctly since the outer braces guard the directive.
func accentTransform(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	depth := 0
	for _, r := range text {
		switch r {
		case '{':
			depth++
			b.WriteRune(r)
			continue
		case '}':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
			continue
		}
		if depth > 0 {
			b.WriteRune(r)
			continue
		}
		if replacement, ok := accentMap[r]; ok {
			b.WriteRune(replacement)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// effectiveWrap resolves the prefix/suffix to actually emit,
// falling back to the shade-marker default when the config leaves
// them empty.
func effectiveWrap(cfg *PseudoConfig) (string, string) {
	prefix := cfg.Prefix
	suffix := cfg.Suffix
	if prefix == "" {
		prefix = "\u2592 "
	}
	if suffix == "" {
		suffix = " \u2592"
	}
	return prefix, suffix
}
