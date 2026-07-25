package format

import (
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// pluralFormOrder fixes the traversal order of a plural run's forms so the
// collected payloads are comparable between two run sequences.
var pluralFormOrder = []model.PluralForm{
	model.PluralZero, model.PluralOne, model.PluralTwo,
	model.PluralFew, model.PluralMany, model.PluralOther,
}

// Verbatim capture is how a same-format round-trip stays byte-exact: a reader
// keeps the original bytes of a value it could not model losslessly (a YAML
// scalar's quoting and block-scalar indentation, a .properties line's escapes
// and continuations, an XLIFF <source> body's inline-code attributes), and its
// writer re-emits those bytes instead of re-encoding the parse tree. Nothing
// the reader could not represent is lost, which is the framework's core value.
//
// A writer may only prefer the captured bytes while they still represent the
// text it is about to write. It cannot establish that by comparing against the
// block's own source:
//
//	if raw, ok := block.Properties["yaml.raw"]; ok && text == block.SourceText() {
//	        write(raw) // WRONG
//	}
//
// After a source edit that comparison is always true — the block's source *is*
// the new text — so the stale bytes win and the edit is silently discarded while
// the command exits 0 (#1473: `kapi ksed -i` was a no-op on yaml, properties, po,
// kbf and xliff). The test has to be against the text the captured bytes decode
// to, which is a fact recorded at read time, not re-derived from the block.
//
// RecordVerbatim / VerbatimFor are that seam for readers whose capture is opaque
// bytes. A reader whose capture is structured (kbf's annotation runs, xliff's
// native inline tree) already carries its own text and compares against it
// directly — VerbatimRunsCurrent is the shared spelling of that comparison.
//
// The distinction being drawn is the family's recurring one: "this block was not
// edited" is not the same fact as "this block's text happens to equal its
// source", exactly as "absent" is not the same fact as "present but unreadable".

// VerbatimTextProp names the companion property under which a reader records the
// text its captured bytes decode to. It is derived from the capture property's
// own name so a block can hold more than one capture without collision.
func VerbatimTextProp(prop string) string { return prop + ".text" }

// RecordVerbatim stores raw as the block's captured verbatim bytes for prop,
// together with text — the text those bytes decode to, which is what makes the
// capture usable later. Readers must call this rather than assigning the capture
// property directly: a capture with no recorded text cannot be proven current,
// so VerbatimFor will refuse it (correct output, no byte-exactness) instead of
// silently emitting stale bytes.
func RecordVerbatim(block *model.Block, prop, raw, text string) {
	if block == nil || prop == "" {
		return
	}
	if block.Properties == nil {
		block.Properties = make(map[string]string, 2)
	}
	block.Properties[prop] = raw
	block.Properties[VerbatimTextProp(prop)] = text
}

// RecordVerbatimText records only the witness — the block's text as read — for a
// writer whose verbatim bytes are the surrounding document rather than a
// per-block capture (androidxml's preserved <resources> body, for instance,
// which is spliced element by element). Such a writer still needs the witness:
// its decision is "splice this entry or leave the original bytes alone", and
// "leave them alone" is only right while the text has not changed.
func RecordVerbatimText(block *model.Block, prop, text string) {
	if block == nil || prop == "" {
		return
	}
	if block.Properties == nil {
		block.Properties = make(map[string]string, 1)
	}
	block.Properties[VerbatimTextProp(prop)] = text
}

// A captured value that stands in a TARGET slot — a bilingual .csv's target
// column, a PO entry's msgstr — belongs to a locale: the one the reader attached
// its content to the block under. A writer that is not pointed at that locale
// still has to reproduce the slot, and what it must reproduce is the block's
// target for the slot's own locale, not the bytes captured before the pipeline
// ran. Without the locale recorded, "this writer has no active locale" is
// indistinguishable from "there is nothing to write for this slot" — which is how
// a tool's edit to an existing translation came to be discarded, and, in PO's
// case, how a translation the file already carried came to be blanked while the
// command exited 0 (#1482, the non-write-locale form of #1471).

// VerbatimSlotLocaleProp names the companion property under which a reader
// records the locale a captured target slot belongs to. Derived from the capture
// property's own name, like VerbatimTextProp.
func VerbatimSlotLocaleProp(prop string) string { return prop + ".locale" }

// RecordVerbatimSlotLocale records that the value captured under prop occupies a
// target slot belonging to locale. A reader calls this whenever it knows which
// locale the slot is for — including when the slot is empty, so a target a tool
// ADDS is written into it rather than ignored.
func RecordVerbatimSlotLocale(block *model.Block, prop string, locale model.LocaleID) {
	if block == nil || prop == "" || locale.IsEmpty() {
		return
	}
	if block.Properties == nil {
		block.Properties = make(map[string]string, 1)
	}
	block.Properties[VerbatimSlotLocaleProp(prop)] = string(locale)
}

// VerbatimSlotLocale returns the locale the target slot captured under prop
// belongs to. It reports false when the reader did not know one — in which case
// the capture is the only statement of the slot's content, and re-emitting it is
// the writer's only faithful move.
func VerbatimSlotLocale(block *model.Block, prop string) (model.LocaleID, bool) {
	if block == nil || len(block.Properties) == 0 || prop == "" {
		return "", false
	}
	loc, ok := block.Properties[VerbatimSlotLocaleProp(prop)]
	if !ok || loc == "" {
		return "", false
	}
	return model.LocaleID(loc), true
}

// VerbatimText returns the text a reader recorded for prop at read time — the
// witness itself, as a value, rather than a comparison against it.
//
// A writer needs the witness as a value when it does not splice by position but
// *locates* what to replace by matching the text it read. EPUB's package rewrite
// does exactly that: it walks the original XHTML and replaces the body of the
// element whose text matches. Keying that lookup on the block's current source
// makes it tautological in the same way the shortcut above was — after a source
// edit the needle is the new text, nothing in the original file matches it, and
// the replacement silently no-ops while the write reports success (#1482). The
// needle has to be the text as read; the replacement is the block's current text.
func VerbatimText(block *model.Block, prop string) (string, bool) {
	if block == nil || len(block.Properties) == 0 || prop == "" {
		return "", false
	}
	text, ok := block.Properties[VerbatimTextProp(prop)]
	return text, ok
}

// VerbatimCurrent reports whether the text recorded for prop at read time still
// equals emitted, i.e. whether the original bytes may stand. A block with no
// recorded witness reports false: unprovable is treated as changed, so the
// failure mode is a re-serialization (correct content, formatting possibly
// normalised) rather than a silently discarded edit.
func VerbatimCurrent(block *model.Block, prop, emitted string) bool {
	if block == nil || len(block.Properties) == 0 {
		return false
	}
	asRead, ok := block.Properties[VerbatimTextProp(prop)]
	return ok && asRead == emitted
}

// VerbatimFor returns the verbatim bytes captured under prop when they still
// decode to emitted — that is, when re-emitting them reproduces exactly the text
// the writer wants. It reports false when nothing was captured, when no text was
// recorded alongside the capture (faithfulness cannot be established, so the
// writer must re-encode), or when the text has since changed — the source edit
// case that used to be indistinguishable from no edit at all.
func VerbatimFor(block *model.Block, prop, emitted string) (string, bool) {
	if block == nil || len(block.Properties) == 0 {
		return "", false
	}
	raw, ok := block.Properties[prop]
	if !ok {
		return "", false
	}
	if !VerbatimCurrent(block, prop, emitted) {
		return "", false
	}
	return raw, true
}

// VerbatimRunsCurrent reports whether a structured capture taken at read time
// still represents the block's current content, comparing the text payloads the
// two run sequences carry. Inline-code runs are deliberately not compared: a
// capture is preferred precisely because it holds markup detail the run
// downconversion cannot express, so comparing markup would reject every capture.
// Text is what a source edit changes, and text is what decides.
func VerbatimRunsCurrent(captured, current []model.Run) bool {
	a, b := runTextPayloads(captured), runTextPayloads(current)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// runTextPayloads collects a run sequence's text payloads in order, descending
// into every plural form and select case (not just "other") so a change in any
// branch is seen. Inline-code runs contribute nothing.
func runTextPayloads(runs []model.Run) []string {
	var out []string
	collectRunTextPayloads(&out, runs)
	return out
}

func collectRunTextPayloads(out *[]string, runs []model.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			*out = append(*out, r.Text.Text)
		case r.Plural != nil:
			for _, cat := range pluralFormOrder {
				if form, ok := r.Plural.Forms[cat]; ok {
					collectRunTextPayloads(out, form)
				}
			}
		case r.Select != nil:
			for _, key := range slices.Sorted(maps.Keys(r.Select.Cases)) {
				collectRunTextPayloads(out, r.Select.Cases[key])
			}
		}
	}
}
