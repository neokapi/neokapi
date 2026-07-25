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
