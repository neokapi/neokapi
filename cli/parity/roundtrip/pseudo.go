//go:build parity

package roundtrip

import (
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
)

// extLatinOldChars / extLatinNewChars mirror Okapi's
// TextModificationStep with TYPE_EXTREPLACE + SCRIPT_EXT_LATIN
// (oldChars[0] / newChars[0] in the Java source). Each rune at index i
// in the "old" string maps to the rune at index i in the "new" string.
// Runes not in the table pass through unchanged. The character set is
// chosen so every replaced glyph is visually distinct from its source
// while staying within commonly-supported Latin-Extended Unicode blocks.
const (
	extLatinOldChars = "AaBbCcDdEeFfGgHhIiJjKkLlNnOoPpQqRrSsTtUuWwYyZz"
	extLatinNewChars = "ÀàßƀĆćĎďĒēƑƒĜĝĤĥ" +
		"ĨĩĵĴĶķĹĺŃńŌōƤƥǪǫŔŕ" +
		"ŚśŢţŨũŴŵŶŷŹź"
)

// extLatinMap is the precomputed substitution table. Built from the
// strings above so the source stays grep-able against the upstream
// TextModificationStep.java.
var extLatinMap = func() map[rune]rune {
	oldRunes := []rune(extLatinOldChars)
	newRunes := []rune(extLatinNewChars)
	if len(oldRunes) != len(newRunes) {
		panic("ext-latin map length mismatch — pseudo.go is out of sync with Okapi TextModificationStep")
	}
	m := make(map[rune]rune, len(oldRunes))
	for i, r := range oldRunes {
		m[r] = newRunes[i]
	}
	return m
}()

// pseudoText applies the SCRIPT_EXT_LATIN substitution to a plain text
// string rune-by-rune. Runes not in the map pass through.
func pseudoText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if mapped, ok := extLatinMap[r]; ok {
			out = append(out, mapped)
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// applyPseudoToBlock mirrors Okapi's TextModificationStep with
// TYPE_EXTREPLACE + SCRIPT_EXT_LATIN: for each translatable Block,
// every TextRun has its text rune-substituted via extLatinMap.
// Non-text runs (Ph, PcOpen, PcClose, Sub, Plural, Select) pass
// through unchanged so inline codes survive the round-trip.
//
// Target preference matches TextModificationStep semantics: if the
// Block already has a target for the locale (e.g. a PO file's
// existing msgstr, an XLIFF target, or a TMX tuv), the substitution
// runs against THAT target, mirroring Okapi's "modify the existing
// translation if one exists, otherwise use the source." Without this
// preference the bridge would always pseudo the English source while
// the okapi reference engine pseudos the existing French
// translation — same fixture, divergent outputs.
//
// The Block is shared with the native writer (NativeEngine) and
// echoed back over gRPC for the bridge writer (BridgeEngine), so the
// same target shape drives both engines.
func applyPseudoToBlock(b *model.Block, spec PseudoSpec) {
	applyPseudoToBlockOpts(b, spec, false)
}

// baseSeg is one segment of the pseudo base: its overlay-local id, its
// props (carrying the xliff2 <ignorable> marker or the ts numerus-form
// index), its runs, and — for xliff2 — the inline IR keyed by this id.
type baseSeg struct {
	id        string
	props     map[string]string
	runs      []model.Run
	ignorable bool
	ir        *xliff2.Content // xliff2 inline IR for this segment, or nil
}

// applyPseudoToBlockOpts is applyPseudoToBlock with a forceSourceBase
// flag. When true, the pseudo always operates on the source runs even
// if a target already exists — used for formats (xliff2) where okapi
// unconditionally overwrites the existing target.
//
// Multi-segment handling rides on the stand-off segmentation model
// (AD-002): a Block carries a flat []Run per side plus a segmentation
// overlay describing per-segment boundaries by run index. We pick a base
// SIDE (the source runs, or an existing target's runs) and iterate its
// segments. The base is the source when forceSourceBase is set or no
// usable target exists; otherwise it is the existing target chosen by
// pickPseudoBase. Crucially the segment list comes from the base side's
// own overlay, so a target that carries MORE segments than the source
// (e.g. a ts <message numerus> whose msgid is one source segment but
// whose translation has several <numerusform>s) gets every segment
// pseudo-translated — mirroring okapi's per-TextUnit TextModificationStep.
//
// Each base segment's runs are pseudo-translated (TextRun text only;
// inline-code runs pass through), concatenated into one flat target run
// slice, and — when there is more than one segment — re-described by a
// target segmentation overlay whose spans reuse the base segment ids and
// props at the concatenated run-index boundaries.
func applyPseudoToBlockOpts(b *model.Block, spec PseudoSpec, forceSourceBase bool) {
	if !b.Translatable {
		return
	}
	tgt := model.LocaleID(spec.TgtLocale())

	// Decide the base side. baseLocale == "" means "base on the source".
	baseFromSource := forceSourceBase && len(b.Source) > 0
	var baseLocale model.LocaleID
	if !baseFromSource {
		baseLocale = pickPseudoBase(b, tgt)
	}

	var baseSegs []baseSeg
	if baseLocale == "" {
		baseSegs = sourceBaseSegs(b)
	} else {
		baseSegs = targetBaseSegs(b, baseLocale)
	}
	if len(baseSegs) == 0 {
		// Nothing usable to base the pseudo on (no source, no target).
		return
	}

	// Existing target segment lookup so ignorable segments preserve their
	// original target verbatim (e.g. an authored French translation
	// inside an xliff2 <ignorable>).
	existingByID := make(map[string][]model.Run)
	for _, s := range targetBaseSegs(b, tgt) {
		existingByID[s.id] = s.runs
	}

	var tgtIR map[string]*xliff2.Content
	outSegs := make([]baseSeg, 0, len(baseSegs))
	var allRuns []model.Run

	for _, bs := range baseSegs {
		runs := bs.runs
		switch {
		case bs.ignorable:
			// Ignorable segments are not pseudo-translated — okapi's
			// TextModificationStep only operates on <segment>s, never
			// <ignorable>s. Preserve any existing target verbatim,
			// otherwise clone the base (source) runs.
			if ex, ok := existingByID[bs.id]; ok {
				runs = ex
			} else {
				runs = cloneRuns(bs.runs)
			}
		default:
			pseudo := make([]model.Run, 0, len(bs.runs))
			for _, r := range bs.runs {
				if r.Text != nil {
					pseudo = append(pseudo, model.Run{Text: &model.TextRun{Text: pseudoText(r.Text.Text)}})
				} else {
					pseudo = append(pseudo, r)
				}
			}
			runs = pseudo
			// xliff2 carries inline-code fidelity (<ph> attributes,
			// <pc>/<sc>/<ec> structure, <data> refs) in the unit's
			// inline IR (UnitSegmentsAnnotation), keyed by segment span
			// id. The writer's freshness check requires the IR's flat
			// text to equal the segment runs' flat text — so when we
			// pseudo the base as the new target, clone the base IR and
			// pseudo-translate its Text nodes in place. Without this the
			// writer falls back to RenderRunsWithData and emits target
			// text without inline codes, e.g. "Ĥōŵ ĩś ŷōũŕ ďàŷ?" instead
			// of "Ĥōŵ ĩś <ph .../>ŷōũŕ<ph .../> ďàŷ?".
			if bs.ir != nil && bs.id != "" {
				if tgtIR == nil {
					tgtIR = make(map[string]*xliff2.Content)
				}
				tgtIR[bs.id] = &xliff2.Content{Inlines: clonePseudoInlines(bs.ir.Inlines)}
			}
		}
		outSegs = append(outSegs, baseSeg{id: bs.id, props: bs.props, runs: runs})
		allRuns = append(allRuns, runs...)
	}

	b.SetTargetRuns(tgt, allRuns)

	// Reproduce the base's multi-segment structure on the target side so
	// the writer can map per-segment runs (and xliff2 inline IR) back to
	// <segment>/<ignorable>/<numerusform> elements. Single-segment blocks
	// normally stay overlay-free (matches the readers' behavior for
	// one-segment units), but a lone segment carrying props — e.g. a ts
	// <message numerus> with a single <numerusform> tagged numerus-form=0
	// — keeps its overlay so the writer re-emits the right element.
	emitOverlay := len(outSegs) > 1
	if !emitOverlay {
		for _, s := range outSegs {
			if len(s.props) > 0 {
				emitOverlay = true
				break
			}
		}
	}
	if emitOverlay {
		spans := make([]model.Span, 0, len(outSegs))
		cursor := 0
		for _, s := range outSegs {
			start := cursor
			end := cursor + len(s.runs)
			cursor = end
			sp := model.Span{
				ID:    s.id,
				Range: model.RunRange{StartRun: start, EndRun: end},
			}
			if len(s.props) > 0 {
				props := make(map[string]string, len(s.props))
				for k, v := range s.props {
					props[k] = v
				}
				sp.Props = props
			}
			spans = append(spans, sp)
		}
		key := model.Variant(tgt)
		b.SetSegmentation(&key, spans)
	}

	// Attach the pseudo-translated inline IR for the target locale so the
	// xliff2 writer reconstructs inline codes on the target segments.
	if len(tgtIR) > 0 {
		setUnitTargetIR(b, tgt, tgtIR)
	}
}

// sourceBaseSegs returns the source side's segments as the pseudo base.
// With a source segmentation overlay each span becomes a baseSeg (id,
// props, runs, ignorable flag, inline IR); without one the whole source
// is a single anonymous segment.
func sourceBaseSegs(b *model.Block) []baseSeg {
	if len(b.Source) == 0 {
		return nil
	}
	srcIR := unitSourceIR(b)
	overlay := b.SourceSegmentation()
	if overlay == nil || len(overlay.Spans) == 0 {
		return []baseSeg{{runs: b.Source}}
	}
	out := make([]baseSeg, 0, len(overlay.Spans))
	for _, sp := range overlay.Spans {
		bs := baseSeg{
			id:        sp.ID,
			props:     sp.Props,
			runs:      sp.Range.ExtractRuns(b.Source),
			ignorable: sp.Props["xliff2:kind"] == "ignorable",
		}
		if srcIR != nil {
			bs.ir = srcIR[sp.ID]
		}
		out = append(out, bs)
	}
	return out
}

// targetBaseSegs returns an existing target's segments for a locale, used
// both to source the pseudo base (bilingual fixtures) and to look up
// verbatim ignorable targets. Returns nil when the locale has no target.
func targetBaseSegs(b *model.Block, loc model.LocaleID) []baseSeg {
	t := b.Target(loc)
	if t == nil {
		return nil
	}
	var ir map[string]*xliff2.Content
	if a := unitSegmentsAnn(b); a != nil {
		ir = a.Target[loc]
	}
	key := model.Variant(loc)
	overlay := b.SegmentationFor(&key)
	if overlay == nil || len(overlay.Spans) == 0 {
		if len(t.Runs) == 0 {
			return nil
		}
		return []baseSeg{{runs: t.Runs}}
	}
	out := make([]baseSeg, 0, len(overlay.Spans))
	for _, sp := range overlay.Spans {
		bs := baseSeg{
			id:        sp.ID,
			props:     sp.Props,
			runs:      sp.Range.ExtractRuns(t.Runs),
			ignorable: sp.Props["xliff2:kind"] == "ignorable",
		}
		if ir != nil {
			bs.ir = ir[sp.ID]
		}
		out = append(out, bs)
	}
	return out
}

// pickPseudoBase returns the locale whose existing target should be the
// pseudo base, or "" to use the source. The chosen target is the one for
// the requested locale when it carries any text content, otherwise any
// existing non-empty target (bilingual formats — ts, tmx, xliff/po with
// an existing translation under a different locale — keep their authored
// translation as the base, matching okapi's TextModificationStep which
// takes the file's target language). Returns "" when no target is usable
// so the caller falls back to the source.
//
// "Has text content" matters because some filters (e.g. okf_xliff for a
// trans-unit with only alt-trans alternatives) hand back a one-segment
// empty target placeholder. Treating that as the base would yield an
// empty pseudo result; instead we want the source so the round-trip
// matches Okapi's TextModificationStep with applyToBlankEntries=true.
func pickPseudoBase(b *model.Block, tgt model.LocaleID) model.LocaleID {
	if t := b.Target(tgt); t != nil && runsHaveText(t.Runs) {
		return tgt
	}
	for _, loc := range b.TargetLocales() {
		if t := b.Target(loc); t != nil && runsHaveText(t.Runs) {
			return loc
		}
	}
	return ""
}

// unitSegmentsAnn returns the block's xliff2 UnitSegmentsAnnotation, or
// nil when absent. The annotation carries the per-segment inline IR for
// both the source and each target locale, keyed by segment span id.
func unitSegmentsAnn(b *model.Block) *xliff2.UnitSegmentsAnnotation {
	if b == nil || b.Annotations == nil {
		return nil
	}
	ann, _ := b.Annotations[(&xliff2.UnitSegmentsAnnotation{}).AnnotationType()].(*xliff2.UnitSegmentsAnnotation)
	return ann
}

// unitSourceIR returns the unit's per-segment source inline IR map (span
// id → Content), or nil when the block carries no UnitSegmentsAnnotation.
func unitSourceIR(b *model.Block) map[string]*xliff2.Content {
	if ann := unitSegmentsAnn(b); ann != nil {
		return ann.Source
	}
	return nil
}

// setUnitTargetIR stores the per-segment target inline IR for a locale on
// the block's UnitSegmentsAnnotation, creating the annotation if absent.
func setUnitTargetIR(b *model.Block, loc model.LocaleID, irByID map[string]*xliff2.Content) {
	key := (&xliff2.UnitSegmentsAnnotation{}).AnnotationType()
	if b.Annotations == nil {
		b.Annotations = map[string]model.Annotation{}
	}
	ann, ok := b.Annotations[key].(*xliff2.UnitSegmentsAnnotation)
	if !ok || ann == nil {
		ann = &xliff2.UnitSegmentsAnnotation{
			Source: map[string]*xliff2.Content{},
			Target: map[model.LocaleID]map[string]*xliff2.Content{},
		}
		b.Annotations[key] = ann
	}
	if ann.Target == nil {
		ann.Target = map[model.LocaleID]map[string]*xliff2.Content{}
	}
	ann.Target[loc] = irByID
}

// clonePseudoInlines deep-clones an xliff2 Inline tree and pseudo-
// translates every Text node's Content via pseudoText. Inline-code
// nodes (Ph/Sc/Ec/Pc/Mrk/Sm/Em) and their attributes are preserved
// verbatim — pseudo affects only translatable text.
func clonePseudoInlines(inls []xliff2.Inline) []xliff2.Inline {
	out := make([]xliff2.Inline, len(inls))
	for i, in := range inls {
		switch {
		case in.Text != nil:
			out[i] = xliff2.Inline{Text: &xliff2.Text{Content: pseudoText(in.Text.Content)}}
		case in.Pc != nil:
			pc := *in.Pc
			pc.Children = clonePseudoInlines(in.Pc.Children)
			out[i] = xliff2.Inline{Pc: &pc}
		case in.Mrk != nil:
			mrk := *in.Mrk
			mrk.Children = clonePseudoInlines(in.Mrk.Children)
			out[i] = xliff2.Inline{Mrk: &mrk}
		case in.Ph != nil:
			ph := *in.Ph
			out[i] = xliff2.Inline{Ph: &ph}
		case in.Sc != nil:
			sc := *in.Sc
			out[i] = xliff2.Inline{Sc: &sc}
		case in.Ec != nil:
			ec := *in.Ec
			out[i] = xliff2.Inline{Ec: &ec}
		case in.Sm != nil:
			sm := *in.Sm
			out[i] = xliff2.Inline{Sm: &sm}
		case in.Em != nil:
			em := *in.Em
			out[i] = xliff2.Inline{Em: &em}
		}
	}
	return out
}

// runsHaveText reports whether a Run sequence carries any non-whitespace
// text in a TextRun. Whitespace-only text counts as empty so we mirror
// okapi's TextModificationStep, which falls back to source when the
// existing target carries no real translation. Some xliff fixtures (e.g.
// MQ-12-Test01.xlf) have placeholder `<target> </target>` tags that
// contain just a space — pseudo-translating the space leaves an
// effectively empty target, while okapi pseudos the source instead.
func runsHaveText(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			continue
		}
		for _, ch := range r.Text.Text {
			if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
				return true
			}
		}
	}
	return false
}
