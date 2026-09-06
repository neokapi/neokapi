// Package tools provides additional localization tools for the neokapi pipeline.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/imageops"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
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
	Prefix           string         `json:"prefix,omitempty"           schema:"title=Prefix,description=Characters prepended before each translated block"`
	Suffix           string         `json:"suffix,omitempty"           schema:"title=Suffix,description=Characters appended after each translated block"`
	TargetLocale     model.LocaleID `json:"targetLocale,omitempty"     schema:"-"`
	// TermRules is the project's terminology, the same key and shape every
	// governed step takes. Only the do-not-translate rules are read here: a
	// product name has to come through the probe intact, or every string that
	// mentions one reads as a bug. A rule with a replacement is a translation
	// decision and has nothing to say about a locale that is not a language.
	TermRules []profile.TermRule `json:"term_rules,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *PseudoConfig) ToolName() string { return "pseudo-translate" }

// Reset restores default values.
func (c *PseudoConfig) Reset() {
	c.ExpansionPercent = 0
	c.Prefix = "\u2592 "
	c.Suffix = " \u2592"
	c.TargetLocale = ""
	c.TermRules = nil
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
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewPseudoTranslateFromConfig creates a pseudo-translate tool from a config map.
func NewPseudoTranslateFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg PseudoConfig
	// Start from the declared defaults so an explicitly empty prefix or suffix
	// in the config map overrides them. Applying onto a zero struct cannot tell
	// "the caller asked for no markers" from "the caller said nothing".
	cfg.Reset()
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
// The caller owns the defaults: cfg is taken as given, including empty markers.
// Reset() declares what unset means, and the callers that cannot tell set from
// unset apply it themselves.
func NewPseudoTranslateTool(cfg *PseudoConfig) *PseudoTranslateTool {
	base := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Generates pseudo-translations for testing localization readiness",
		Cfg:             cfg,
	}
	// Media: pseudo-localize images (a clearly-visible watermark + color wash) so
	// a swapped/localized image is unmistakable in a UI or build — the visual
	// analog of the text transform. Non-image media passes through.
	base.HandleMediaFn = func(part *model.Part) (*model.Part, error) {
		pseudoLocalizeMedia(part, cfg)
		return part, nil
	}
	// Translate: pseudo-translate writes a target; source is read-only.
	base.Produce = func(v tool.VariantView) error {
		if !v.Translatable() {
			return nil
		}
		runs := v.SourceRuns()
		if len(runs) == 0 {
			return nil
		}
		if shouldWalkRuns(runs, cfg) {
			targetRuns := pseudoTranslateRuns(runs, cfg)
			v.SetTargetRuns(cfg.TargetLocale, targetRuns)
		} else {
			sourceText := v.SourceText()
			if sourceText == "" {
				return nil
			}
			v.SetTargetText(cfg.TargetLocale, pseudoTranslate(sourceText, cfg))
		}
		// Pseudo output is a placeholder, not a real translation: stamp it
		// `draft` so coverage/ship gates never count it as `translated`. It takes
		// no governing context and carries no Profile/ContextFingerprint — there is
		// nothing for it to be stale against, so the governance half of Origin is
		// deliberately left empty (unlike AI/MT/recycle).
		v.StampTargetProvenance(cfg.TargetLocale, model.TargetStatusDraft, model.Origin{Tool: cfg.ToolName()})
		return nil
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
	if shouldWalkRuns(runs, conf) {
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
	block.StampTargetProvenance(conf.TargetLocale, model.TargetStatusDraft, model.Origin{Tool: conf.ToolName()})
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
			if err := t.processOne(ctx, sess, caps.RandomAccess, overlayKind, part); err != nil {
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
	ctx context.Context,
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
	if block.ID == "" {
		_, err := applyPseudo(part, t.cfg)
		return err
	}
	// Key overlays globally-unique per source file (falls back to the raw id for
	// ad-hoc single-document runs) so multi-file projects don't collide.
	hash := blockstore.OverlayKey(ctx, block.ID, block.SourceText())

	// Consult existing overlay when the provider supports random
	// access. If one exists, hydrate the block from it and skip the
	// translator.
	configFP := pseudoConfigFingerprint(t.cfg)
	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached pseudoCache
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Target != "" && cached.Config == configFP {
				block.SetTargetText(t.cfg.TargetLocale, cached.Target)
				block.StampTargetProvenance(t.cfg.TargetLocale, model.TargetStatusDraft, model.Origin{Tool: t.cfg.ToolName()})
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
		payload, err := json.Marshal(pseudoCache{Target: target, Config: configFP})
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
	return blockstore.TargetOverlayKind(locale)
}

// pseudoCache is the JSON payload stored in a pseudo-translate
// overlay. Small and focused; richer fields (runs, provenance) are
// a follow-up.
type pseudoCache struct {
	Target string `json:"target"`
	// Config is the tool-config fingerprint at write time; a cached target is
	// reused only when it matches the current config (prefix/suffix/expansion/
	// locale), so changing the pseudo style re-runs rather than serving stale.
	Config string `json:"config,omitempty"`
}

// pseudoConfigFingerprint hashes the pseudo settings that change a block's
// output — every field of the config does — so the session overlay cache reuses
// a cached target only when the style is unchanged.
func pseudoConfigFingerprint(cfg *PseudoConfig) string {
	return tool.OverlayConfigFingerprint(
		"pseudo",
		strconv.Itoa(cfg.ExpansionPercent),
		cfg.Prefix,
		cfg.Suffix,
		string(cfg.TargetLocale),
	)
}

// pseudoLocalizeMedia pseudo-localizes an image Media part in place: it replaces
// the image with a clearly-visible watermarked variant (tint + border + band)
// and pseudo-translates the alt-text. It is best-effort — non-image media, or a
// transform/decode failure, leaves the part unchanged. Bytes come from the
// Media's own content — inline or deferred — or, for a URI reference, the
// source file (read here because pseudo-localization is an explicit pixel
// transform, distinct from the OCR path that keeps image bytes out of the host).
func pseudoLocalizeMedia(part *model.Part, cfg *PseudoConfig) {
	m, ok := part.Resource.(*model.Media)
	if !ok || m == nil || !isImageMedia(m) {
		return
	}
	var data []byte
	switch {
	case m.HasContent():
		b, err := m.Bytes()
		if err != nil {
			return
		}
		data = b
	case m.URI != "":
		b, err := os.ReadFile(m.URI)
		if err != nil {
			return
		}
		data = b
	}
	if len(data) == 0 {
		return
	}
	out, err := imageops.PseudoLocalize(data, imageops.PseudoOptions{})
	if err != nil {
		return
	}
	// The watermarked image replaces whatever the slice pointed at, so any
	// deferred accessor into the source document is now stale.
	m.Data = out
	m.Open = nil
	m.MimeType = "image/png"
	m.Size = int64(len(out))
	if m.Properties == nil {
		m.Properties = map[string]string{}
	}
	m.Properties["pseudo"] = "true"
	if m.AltText != "" {
		m.AltText = pseudoTranslate(m.AltText, cfg)
	}
}

// isImageMedia reports whether a Media part is a raster image (by MIME type, or
// by filename extension as a fallback).
func isImageMedia(m *model.Media) bool {
	if strings.HasPrefix(m.MimeType, "image/") {
		return true
	}
	name := strings.ToLower(m.Filename)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// runsHaveInline reports whether the run sequence contains any
// non-text run (placeholder, paired code, subblock reference, or
// structured plural/select construct). Used by pseudo-translate to
// pick between the text-only fast path and the Run-walker path.
// shouldWalkRuns reports whether a block has to go through the run walk instead
// of the shortcut that flattens it to one string.
//
// The walk is the capable route: it is the only one that can leave part of a
// block alone. Flattening is an optimization for a block with nothing to
// preserve, and a project that declares do-not-translate terms has something to
// preserve in any sentence that might name one.
//
// It lives in one function because the decision is made in two places — the
// tool's Produce and its streaming block handler — and they disagreed: a
// heading naming the product came out mangled while the same name in a
// paragraph beside a code span survived.
func shouldWalkRuns(runs []model.Run, cfg *PseudoConfig) bool {
	return runsHaveInline(runs) || len(dntTerms(cfg)) > 0
}

func runsHaveInline(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
		// A sequence of nothing but text runs still has to go the run route
		// when one of them is protected. The text route flattens the block to
		// SourceText() and mangles the lot, which loses the marking without a
		// trace — a table cell reading "`docker compose up` + `make dev-server`"
		// is all text runs, and that is how those commands reached the docs site
		// mangled while the same span in a sentence survived.
		if r.Text.NoTranslate {
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
		case r.Text != nil && r.Text.NoTranslate:
			// A code span, a <kbd>, a <samp>. Mangling it is how `kapi check
			// --ship` became `ķàþî çĥéçķ --šĥîþ` on the docs site: the marker
			// survived and the command did not. Carried through untouched,
			// flag and all, so the writer puts back what it read.
			out = append(out, model.Run{Text: &model.TextRun{Text: r.Text.Text, NoTranslate: true}})
		case r.Text != nil:
			// A do-not-translate term inside otherwise ordinary prose splits the
			// run: the product name comes through as itself, the sentence
			// around it is accented. Protected text is left out of the
			// expansion count for the same reason the flagged runs above are —
			// it is not going to grow in a real translation.
			for _, piece := range protectTerms(r.Text.Text, dntTerms(cfg)) {
				if piece.Text.NoTranslate {
					out = append(out, piece)
					continue
				}
				accented := accentTransform(piece.Text.Text)
				totalTextRunes += len([]rune(accented))
				out = append(out, model.TextR(accented))
			}
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, v := range r.Plural.Forms {
				forms[k] = pseudoTranslateRuns(v, cfg)
			}
			out = append(out, model.PluralR(model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}))
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, v := range r.Select.Cases {
				cases[k] = pseudoTranslateRuns(v, cfg)
			}
			out = append(out, model.SelectR(model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}))
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
				out = append(out, model.TextR(padding))
			}
		}
	}

	// Pass 3: wrap the whole sequence exactly once. Prefix goes in
	// a new leading text run, suffix in a new trailing text run.
	// An empty marker adds NO run. A wrap run carrying "" still changes the
	// run sequence, and a consumer that compares a target's shape against its
	// source rejects the target for it — which is how a paragraph came back
	// untranslated on a site configured for markerless pseudo: the KBF
	// compiler dropped every block whose runs no longer matched.
	prefix, suffix := effectiveWrap(cfg)
	if prefix != "" {
		out = append([]model.Run{model.TextR(prefix)}, out...)
	}
	if suffix != "" {
		out = append(out, model.TextR(suffix))
	}
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

// effectiveWrap returns the markers to emit, exactly as configured.
//
// It used to substitute the shade markers for an empty field, which made "wrap
// nothing" impossible to ask for: a demo site wants the mangled text as its
// signal and the brackets are scaffolding. Reset() declares the default, and
// every entry point applies it before this runs, so an empty field here means
// the caller asked for empty.
func effectiveWrap(cfg *PseudoConfig) (string, string) {
	return cfg.Prefix, cfg.Suffix
}

// dntTerms collects the do-not-translate strings from the project's term rules,
// longest first so a term that contains another is matched whole: "neokapi"
// before "kapi", or the probe would protect four letters of it and mangle the
// rest.
func dntTerms(cfg *PseudoConfig) []string {
	if cfg == nil || len(cfg.TermRules) == 0 {
		return nil
	}
	var out []string
	for _, rule := range cfg.TermRules {
		if rule.DoNotTranslate && rule.Term != "" {
			out = append(out, rule.Term)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// protectTerms splits text around every do-not-translate term it contains,
// marking each match so the accent pass leaves it alone.
//
// Matching is case-insensitive and the source's own casing is kept. The docs
// navbar writes "Kapi" and the prose writes "kapi"; both name the product, and
// a probe that mangles one of them reports a bug that is not there. Whether the
// capital is the right one is a vocabulary question, and term-check answers it.
//
// A match must be a whole word, so a longer word that merely starts with a
// product name is still ordinary prose.
func protectTerms(text string, terms []string) []model.Run {
	if text == "" || len(terms) == 0 {
		return []model.Run{{Text: &model.TextRun{Text: text}}}
	}
	lower := strings.ToLower(text)

	var runs []model.Run
	add := func(s string, dnt bool) {
		if s == "" {
			return
		}
		runs = append(runs, model.Run{Text: &model.TextRun{Text: s, NoTranslate: dnt}})
	}

	cursor := 0
	for i := 0; i < len(text); {
		matched := 0
		for _, term := range terms {
			if !strings.HasPrefix(lower[i:], strings.ToLower(term)) {
				continue
			}
			if !boundedAt(text, i, len(term)) {
				continue
			}
			matched = len(term)
			break
		}
		if matched == 0 {
			i++
			continue
		}
		add(text[cursor:i], false)
		add(text[i:i+matched], true)
		i += matched
		cursor = i
	}
	add(text[cursor:], false)

	if len(runs) == 0 {
		return []model.Run{{Text: &model.TextRun{Text: text}}}
	}
	return runs
}

// boundedAt reports whether the slice of length n at i is a whole word: neither
// neighbour may be a letter or a digit.
func boundedAt(text string, i, n int) bool {
	if i > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:i])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	if i+n < len(text) {
		r, _ := utf8.DecodeRuneInString(text[i+n:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
