package applestrings_test

import (
	"path/filepath"
	"testing"

	applestrings "github.com/neokapi/neokapi/core/formats/applestrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layerNotes returns the layer-scoped NoteAnnotations carried on the document's
// PartLayerStart, or nil when none are present.
func layerNotes(t *testing.T, parts []*model.Part) []*model.NoteAnnotation {
	t.Helper()
	for _, p := range parts {
		if p.Type != model.PartLayerStart {
			continue
		}
		layer, ok := p.Resource.(*model.Layer)
		require.True(t, ok, "PartLayerStart resource must be a *model.Layer")
		v, ok := layer.Anno(model.AnnoNote)
		if !ok {
			return nil
		}
		notes, ok := v.(*model.Notes)
		require.True(t, ok, "layer AnnoNote must be a *model.Notes")
		return notes.Items
	}
	return nil
}

func noteTexts(notes []*model.NoteAnnotation) []string {
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		out = append(out, n.Text)
	}
	return out
}

// TestStringsOrphanCommentsBecomeLayerNotes verifies that .strings comments not
// attached to a following entry — a superseded earlier comment (last-wins) and a
// trailing/orphan comment at EOF — are surfaced as layer-scoped NoteAnnotations
// rather than silently dropped. The comment immediately preceding an entry still
// becomes that block's note (existing behaviour), no extra blocks are emitted,
// and the bytes round-trip exactly (the comments live in the skeleton).
func TestStringsOrphanCommentsBecomeLayerNotes(t *testing.T) {
	input := "/* superseded comment */\n" +
		"/* kept comment */\n" +
		"\"key1\" = \"value1\";\n" +
		"\n" +
		"// trailing orphan note\n"

	parts := readPartsBytes(t, "Orphans.strings", []byte(input))

	// The superseded and trailing comments are surfaced on the layer.
	notes := layerNotes(t, parts)
	assert.ElementsMatch(t,
		[]string{"superseded comment", "trailing orphan note"},
		noteTexts(notes),
		"orphan/superseded comments must surface as layer notes")
	for _, n := range notes {
		assert.Equal(t, "developer", n.From)
		assert.Equal(t, "general", n.Annotates)
	}

	// The kept (last-wins) comment is still the block's own note; only one block
	// is emitted (orphan comments do not create translatable content).
	byName := blockByName(parts)
	require.Len(t, byName, 1)
	key1 := byName["key1"]
	require.NotNil(t, key1)
	require.Len(t, key1.Notes(), 1)
	assert.Equal(t, "kept comment", key1.Notes()[0].Text)

	// Byte-exact round-trip: comments ride in the skeleton, unchanged.
	out := writeParts(t, parts, "")
	assert.Equal(t, input, string(out), "orphan comments must round-trip byte-for-byte")
}

// TestStringsOrphanCommentsGatedByExtractComments verifies the orphan/superseded
// comment layer notes honour the existing ExtractComments toggle (the same one
// that governs per-block comment notes): with it off, no comment surfaces as a
// note, the part stream is unchanged, and the bytes still round-trip.
func TestStringsOrphanCommentsGatedByExtractComments(t *testing.T) {
	input := "/* superseded comment */\n" +
		"/* kept comment */\n" +
		"\"key1\" = \"value1\";\n" +
		"// trailing orphan note\n"

	r := applestrings.NewReader()
	cfg := r.Config().(*applestrings.Config)
	require.NoError(t, cfg.ApplyMap(map[string]any{"extractComments": false}))

	doc := &model.RawDocument{URI: "Orphans.strings", Encoding: "UTF-8", Reader: nopReader([]byte(input))}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))

	assert.Nil(t, layerNotes(t, parts), "no layer notes when ExtractComments is off")
	byName := blockByName(parts)
	require.Len(t, byName, 1)
	assert.Empty(t, byName["key1"].Notes(), "no block notes when ExtractComments is off")

	out := writeParts(t, parts, "")
	assert.Equal(t, input, string(out), "round-trip must stay byte-exact with the flag off")
}

// TestStringsdictXMLCommentBecomesLayerNote verifies that an XML comment
// (<!-- ... -->) inside a .stringsdict plist — which is never part of a
// translatable <string> leaf — is surfaced as a layer-scoped NoteAnnotation, no
// extra blocks are produced, and the document round-trips byte-for-byte.
func TestStringsdictXMLCommentBecomesLayerNote(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<!-- translator hint: keep the count token -->
	<key>%d files selected</key>
	<dict>
		<key>NSStringLocalizedFormatKey</key>
		<string>%#@files@</string>
		<key>files</key>
		<dict>
			<key>NSStringFormatSpecTypeKey</key>
			<string>NSStringPluralRuleType</string>
			<key>NSStringFormatValueTypeKey</key>
			<string>d</string>
			<key>one</key>
			<string>%d file selected</string>
			<key>other</key>
			<string>%d files selected</string>
		</dict>
	</dict>
</dict>
</plist>
`

	parts := readPartsBytes(t, "Hints.stringsdict", []byte(input))

	notes := layerNotes(t, parts)
	require.Len(t, notes, 1)
	assert.Equal(t, "translator hint: keep the count token", notes[0].Text)
	assert.Equal(t, "developer", notes[0].From)

	// The comment is metadata, not a block: still exactly the format key + the
	// two plural leaves.
	byName := blockByName(parts)
	require.Len(t, byName, 3)
	require.NotNil(t, byName["%d files selected"])
	require.NotNil(t, byName["%d files selected/files/one"])

	out := writeParts(t, parts, "")
	assert.Equal(t, input, string(out), "XML comment must round-trip byte-for-byte")
}

// TestNoSpuriousLayerNotes verifies the committed fixtures (whose comments are
// all attached to entries) carry no layer-scoped notes, so the orphan/XML-comment
// surfacing does not add noise to well-formed documents.
func TestNoSpuriousLayerNotes(t *testing.T) {
	for _, name := range []string{"Localizable.strings", "Localizable.stringsdict"} {
		t.Run(name, func(t *testing.T) {
			parts, _ := readParts(t, filepath.Join("testdata", name))
			assert.Nil(t, layerNotes(t, parts), "no orphan comments → no layer notes")
		})
	}
}
