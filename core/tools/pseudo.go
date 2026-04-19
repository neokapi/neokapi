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
	Prefix           string         `json:"prefix,omitempty"           schema:"title=Prefix,description=Characters prepended before each translated segment,default=["`
	Suffix           string         `json:"suffix,omitempty"           schema:"title=Suffix,description=Characters appended after each translated segment,default=]"`
	TargetLocale     model.LocaleID `json:"targetLocale,omitempty"     schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *PseudoConfig) ToolName() string { return "pseudo-translate" }

// Reset restores default values.
func (c *PseudoConfig) Reset() {
	c.ExpansionPercent = 0
	c.Prefix = "["
	c.Suffix = "]"
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
// check a `targets/<locale>` sidecar first (skip if present), emit
// the pseudo-translated block, and write a sidecar so downstream
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
		cfg.Prefix = "["
		cfg.Suffix = "]"
	}

	base := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Generates pseudo-translations for testing localization readiness",
		Cfg:             cfg,
	}
	base.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		return applyPseudo(part, cfg)
	}
	return &PseudoTranslateTool{BaseTool: base, cfg: cfg}
}

// applyPseudo runs the deterministic pseudo-translation on a block
// part. Factored out so SessionProcess can call it after checking
// the sidecar cache.
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

// SessionProcess reads prior targets/<locale> sidecars to skip
// already-translated blocks, runs the pseudo translator, and writes
// the target back as a sidecar so subsequent sessions can consult
// it.
func (t *PseudoTranslateTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	sidecarKind := pseudoSidecarKind(t.cfg.TargetLocale)
	caps := sess.Capabilities()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if err := t.processOne(sess, caps.RandomAccess, sidecarKind, part); err != nil {
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
	sidecarKind string,
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

	// Consult existing sidecar when the provider supports random
	// access. If one exists, hydrate the block from it and skip the
	// translator.
	if randomAccess {
		if sc, err := sess.GetSidecar(sidecarKind, hash); err == nil && len(sc.Payload) > 0 {
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

	// Write the freshly-computed target back as a sidecar so future
	// runs can skip the work. Pure text cache — runs-level targets
	// round-trip through the block model itself.
	if target := block.TargetText(t.cfg.TargetLocale); target != "" {
		payload, err := json.Marshal(pseudoCache{Target: target})
		if err != nil {
			return fmt.Errorf("pseudo-translate: encode sidecar: %w", err)
		}
		if err := sess.PutSidecar(blockstore.Sidecar{
			Kind:      sidecarKind,
			BlockHash: hash,
			Payload:   payload,
		}); err != nil {
			// Ignore read-only stores (e.g. FormatReaderStore) — the
			// in-flight block already carries the target; the sidecar
			// write is best-effort caching for next time.
			if !errors.Is(err, blockstore.ErrReadOnly) {
				return fmt.Errorf("pseudo-translate: write sidecar: %w", err)
			}
		}
	}
	return nil
}

// pseudoSidecarKind returns the "targets/<locale>" kind used for the
// sidecar written by pseudo-translate. Shared with AI translate /
// MT translate so any locale target is discoverable under one key.
func pseudoSidecarKind(locale model.LocaleID) string {
	return "targets/" + string(locale)
}

// pseudoCache is the JSON payload stored in a pseudo-translate
// sidecar. Small and focused; richer fields (runs, provenance) are
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
func pseudoTranslateRuns(runs []model.Run, cfg *PseudoConfig) []model.Run {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch {
		case r.Text != nil:
			rtext := pseudoTranslate(r.Text.Text, cfg)
			out[i] = model.Run{Text: &model.TextRun{Text: rtext}}
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, v := range r.Plural.Forms {
				forms[k] = pseudoTranslateRuns(v, cfg)
			}
			out[i] = model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}}
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, v := range r.Select.Cases {
				cases[k] = pseudoTranslateRuns(v, cfg)
			}
			out[i] = model.Run{Select: &model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}}
		default:
			out[i] = r
		}
	}
	return out
}

// pseudoTranslate applies pseudo-translation transformations to text.
func pseudoTranslate(text string, cfg *PseudoConfig) string {
	// Step 1: Replace ASCII characters with accented equivalents.
	var accented strings.Builder
	for _, r := range text {
		if replacement, ok := accentMap[r]; ok {
			accented.WriteRune(replacement)
		} else {
			accented.WriteRune(r)
		}
	}

	result := accented.String()

	// Step 2: Add expansion padding.
	if cfg.ExpansionPercent > 0 {
		originalLen := len([]rune(result))
		paddingLen := (originalLen * cfg.ExpansionPercent) / 100
		if paddingLen > 0 {
			padding := strings.Repeat("~", paddingLen)
			result = result + " " + padding
		}
	}

	// Step 3: Wrap with prefix/suffix.
	prefix := cfg.Prefix
	suffix := cfg.Suffix
	if prefix == "" {
		prefix = "["
	}
	if suffix == "" {
		suffix = "]"
	}
	result = prefix + result + suffix

	return result
}
