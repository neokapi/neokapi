package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The seam that decides whether a reader's captured bytes may still be
// re-emitted. Its whole job is to draw one distinction — "this block was not
// edited" vs "this block's text equals its source" — so every case here is
// paired with the control that stops "refuse every capture" from being an
// acceptable answer.

func TestVerbatimFor_UneditedCaptureIsUsable(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	b.SetSourceText("We utilize it")
	format.RecordVerbatim(b, "yaml.raw", `  "We utilize it"`, "We utilize it")

	raw, ok := format.VerbatimFor(b, "yaml.raw", b.SourceText())
	require.True(t, ok, "an untouched block must keep its byte-exact capture")
	assert.Equal(t, `  "We utilize it"`, raw)
}

func TestVerbatimFor_EditedSourceRejectsTheCapture(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	b.SetSourceText("We utilize it")
	format.RecordVerbatim(b, "yaml.raw", `  "We utilize it"`, "We utilize it")

	// The edit a source-transform tool makes. Note what the old gate compared:
	// the emitted text against b.SourceText() — which is now the NEW text, so it
	// matched and the pre-edit bytes were re-emitted.
	b.SetSourceText("We use it")
	require.Equal(t, "We use it", b.SourceText())

	_, ok := format.VerbatimFor(b, "yaml.raw", b.SourceText())
	assert.False(t, ok,
		"an edited source must not be written back as the captured bytes — "+
			"comparing against the block's own source can never see the edit")
}

func TestVerbatimFor_CaptureWithNoWitnessIsRefused(t *testing.T) {
	b := &model.Block{ID: "tu1", Properties: map[string]string{"yaml.raw": `  "stale"`}}
	b.SetSourceText("stale")

	_, ok := format.VerbatimFor(b, "yaml.raw", b.SourceText())
	assert.False(t, ok,
		"a capture whose text was never recorded cannot be proven current, so it "+
			"must be re-encoded rather than trusted")
}

func TestVerbatimFor_MissingCaptureIsNotAnError(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	b.SetSourceText("no capture here")
	_, ok := format.VerbatimFor(b, "yaml.raw", b.SourceText())
	assert.False(t, ok, "a format with no capture simply re-encodes")
}

func TestVerbatimCurrent_WitnessOnlyRecording(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	format.RecordVerbatimText(b, "androidxml.value", "We utilize it")

	assert.True(t, format.VerbatimCurrent(b, "androidxml.value", "We utilize it"),
		"the document's own bytes may stand while the text is unchanged")
	assert.False(t, format.VerbatimCurrent(b, "androidxml.value", "We use it"),
		"an edited entry must be spliced over the preserved document")
	assert.Empty(t, b.Properties["androidxml.value"],
		"a witness-only recording must not invent a capture property")
}

func TestVerbatimRunsCurrent(t *testing.T) {
	text := func(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }
	ph := func(id string) model.Run { return model.Run{Ph: &model.PlaceholderRun{ID: id}} }

	captured := []model.Run{text("We "), ph("p1"), text("utilize it")}

	assert.True(t, format.VerbatimRunsCurrent(captured, []model.Run{text("We "), ph("p1"), text("utilize it")}),
		"identical text means the capture still stands")
	assert.False(t, format.VerbatimRunsCurrent(captured, []model.Run{text("We "), ph("p1"), text("use it")}),
		"an edited text run must reject the capture")

	// Inline-code markup is deliberately NOT compared: a capture is preferred
	// because it holds markup detail the run model cannot express, so comparing
	// markup would reject every capture. Only text decides.
	assert.True(t, format.VerbatimRunsCurrent(captured, []model.Run{text("We "), ph("p2"), text("utilize it")}),
		"a differing placeholder id must not by itself invalidate the capture")

	assert.False(t, format.VerbatimRunsCurrent(captured, []model.Run{text("We utilize it")}),
		"a different number of text payloads is a changed shape")
}

func TestVerbatimTextProp_DerivesFromTheCaptureName(t *testing.T) {
	// Per-capture witness names, so one block can hold more than one capture
	// (po records a msgid and a msgstr) without them colliding.
	assert.NotEqual(t, format.VerbatimTextProp("raw-msgid"), format.VerbatimTextProp("raw-msgstr"))
	assert.Equal(t, "@raw-msgid.text", format.VerbatimTextProp("raw-msgid"))
}

// A verbatim capture is a byte copy of the block's own content. If it were
// hashed into the block's context, editing the block would move the content and
// context hashes together and core/reconcile would grade the edit as a new
// block, dropping the translation and approval the old one carried.
//
// This is the same defect as naming a heading after its own title, arriving
// through a property rather than a name — and it bit the formats with the best
// natural keys, because those are the ones that use skeleton mode.
func TestRecordVerbatim_DoesNotEnterTheContextHash(t *testing.T) {
	plain := model.NewBlock("tu1", "Hello")
	plain.Name = "greeting"

	captured := model.NewBlock("tu1", "Hello")
	captured.Name = "greeting"
	format.RecordVerbatim(captured, "raw-msgid", "msgid \"Hello\"", "Hello")

	assert.Equal(t,
		model.ComputeIdentity(plain).ContextHash,
		model.ComputeIdentity(captured).ContextHash,
		"a capture of the content must not become part of the content's identity")

	// It is still there — carried, just not identifying.
	got, ok := format.VerbatimFor(captured, "raw-msgid", "Hello")
	require.True(t, ok, "the capture is still usable for byte-exact output")
	assert.Equal(t, "msgid \"Hello\"", got)
}

// The slot-locale seam (#1482). A captured TARGET slot belongs to a locale, and
// the writer needs that locale to tell "no active locale was given" from "this
// slot has nothing to write". Both cases below draw the same distinction the
// witness above draws, one level up.

func TestVerbatimSlotLocale_RecordedLocaleIsReturned(t *testing.T) {
	b := &model.Block{ID: "tu1"}
	format.RecordVerbatimSlotLocale(b, "raw-msgstr", model.LocaleID("fr"))

	loc, ok := format.VerbatimSlotLocale(b, "raw-msgstr")
	require.True(t, ok, "a recorded slot locale must be readable back")
	assert.Equal(t, model.LocaleID("fr"), loc)
	assert.Equal(t, "raw-msgstr.locale", format.VerbatimSlotLocaleProp("raw-msgstr"))
}

func TestVerbatimSlotLocale_UnknownIsReportedAsUnknown(t *testing.T) {
	// The control. A reader that could not name the slot's locale must leave the
	// writer unable to claim one: the alternative — treating the absence as
	// "untranslated" — is what blanked a translated PO catalog.
	b := &model.Block{ID: "tu1"}
	_, ok := format.VerbatimSlotLocale(b, "raw-msgstr")
	assert.False(t, ok, "no recorded locale must report unknown, not a zero locale")

	// An empty locale is not a locale: recording one records nothing.
	format.RecordVerbatimSlotLocale(b, "raw-msgstr", model.LocaleID(""))
	_, ok = format.VerbatimSlotLocale(b, "raw-msgstr")
	assert.False(t, ok, "an empty locale must not be recorded as if it were known")
}

func TestVerbatimText_ReturnsTheWitnessAsAValue(t *testing.T) {
	// EPUB's package rewrite locates what to replace BY the text as read, so it
	// needs the witness as a value, not as a comparison.
	b := &model.Block{ID: "tu1"}
	b.SetSourceText("This is the first paragraph.")
	format.RecordVerbatimText(b, "epub.xhtml", "This is the first paragraph.")

	// After the edit the block's source is the NEW text; the witness is not.
	b.SetSourceText("This is the first section.")
	asRead, ok := format.VerbatimText(b, "epub.xhtml")
	require.True(t, ok)
	assert.Equal(t, "This is the first paragraph.", asRead,
		"the witness must stay the text as read — keyed on the current source the lookup is tautological")

	_, ok = format.VerbatimText(b, "nothing-recorded")
	assert.False(t, ok)
}
