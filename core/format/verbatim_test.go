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
	assert.Equal(t, "raw-msgid.text", format.VerbatimTextProp("raw-msgid"))
}
