package androidxml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readWithConfig reads an inline Android resources document through the reader
// configured by apply (nil = defaults) and returns the parts.
func readWithConfig(t *testing.T, doc string, apply func(*androidxml.Config)) []*model.Part {
	t.Helper()
	r := androidxml.NewReader()
	cfg := &androidxml.Config{}
	cfg.Reset()
	if apply != nil {
		apply(cfg)
	}
	require.NoError(t, r.SetConfig(cfg))
	require.NoError(t, r.Open(t.Context(), newDoc("strings.xml", []byte(doc))))
	defer r.Close()
	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	return parts
}

const nonTranslatableDoc = `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="greeting">Hello</string>
    <string name="debug" translatable="false">DEBUG_BUILD_MARKER</string>
    <string-array name="flags" translatable="false">
        <item>internal_flag_a</item>
        <item>internal_flag_b</item>
    </string-array>
    <plurals name="raw_counts" translatable="false">
        <item quantity="one">%1$d_unit</item>
        <item quantity="other">%1$d_units</item>
    </plurals>
</resources>
`

// TestNonTranslatableContentSurfaced (issue #928 A) verifies that, with the
// default ExtractNonTranslatableContent on, <string>/<string-array>/<plurals>
// entries marked translatable="false" are surfaced as NON-translatable content
// blocks — visible to ingestion, skipped by MT — while a byte-faithful
// round-trip is preserved.
func TestNonTranslatableContentSurfaced(t *testing.T) {
	t.Parallel()

	parts := readWithConfig(t, nonTranslatableDoc, nil)
	by := blockByName(parts)

	// The real string is translatable; the marked entries are surfaced as
	// non-translatable content.
	require.Contains(t, by, "greeting")
	assert.True(t, by["greeting"].Translatable)

	cases := []struct {
		name string
		text string
		kind string
	}{
		{"debug", "DEBUG_BUILD_MARKER", "string"},
		{"flags[0]", "internal_flag_a", "string-array"},
		{"flags[1]", "internal_flag_b", "string-array"},
		{"raw_counts[one]", "%1$d_unit", "plurals"},
		{"raw_counts[other]", "%1$d_units", "plurals"},
	}
	for _, c := range cases {
		b := by[c.name]
		require.NotNilf(t, b, "translatable=\"false\" entry %q is surfaced", c.name)
		assert.Falsef(t, b.Translatable, "%q is non-translatable content", c.name)
		assert.Truef(t, b.PreserveWhitespace, "%q preserves whitespace", c.name)
		assert.Emptyf(t, b.SemanticRole(), "%q is a plain value, no semantic role", c.name)
		assert.Equalf(t, c.kind, b.Properties["androidxml.kind"], "%q kind", c.name)
		// The value re-renders to its exact source bytes (printf codes survive).
		assert.Equalf(t, c.text, model.RenderRunsWithData(b.SourceRuns()),
			"%q value is surfaced verbatim", c.name)
	}

	// Round-trip with no edits reproduces the original bytes exactly.
	out := writeParts(t, parts, "")
	assert.Equal(t, nonTranslatableDoc, string(out),
		"surfacing non-translatable content must not change the bytes")
}

// TestNonTranslatableContentDisabled (issue #928 — flag off path) verifies that
// turning ExtractNonTranslatableContent off keeps translatable="false" entries
// in opaque skeleton: no blocks are emitted for them, only the genuinely
// translatable entry surfaces, and the round-trip is still byte-exact. This is
// the part-stream-byte-identical baseline parity relies on.
func TestNonTranslatableContentDisabled(t *testing.T) {
	t.Parallel()

	parts := readWithConfig(t, nonTranslatableDoc, func(c *androidxml.Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	names := map[string]bool{}
	for _, b := range blocks(parts) {
		names[b.Name] = true
	}
	assert.Equal(t, map[string]bool{"greeting": true}, names,
		"only the translatable entry is extracted when surfacing is off")

	out := writeParts(t, parts, "")
	assert.Equal(t, nonTranslatableDoc, string(out),
		"flag-off round-trip must be byte-exact")
}

// TestExtractNonTranslatableContentApplyMap verifies the ApplyMap key and the
// default value of the toggle.
func TestExtractNonTranslatableContentApplyMap(t *testing.T) {
	t.Parallel()

	cfg := &androidxml.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "surfacing defaults on")

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "nope"}),
		"non-bool value is rejected")
}

// TestNonTranslatableSkeletonByteExact pins the merge (skeleton store) path for
// surfaced translatable="false" entries: reading with a skeleton store wired,
// then writing it back with no translation, must reproduce the source bytes
// exactly even though each marked entry now rides a content ref.
func TestNonTranslatableSkeletonByteExact(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	reader := androidxml.NewReader()
	writer := androidxml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(nonTranslatableDoc, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// The marked entries surfaced as non-translatable content blocks.
	var nonTranslatable int
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && !b.Translatable {
				nonTranslatable++
			}
		}
	}
	assert.Equal(t, 5, nonTranslatable, "debug + 2 array items + 2 plural items")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	assert.Equal(t, nonTranslatableDoc, buf.String(),
		"skeleton round-trip over surfaced non-translatable content is byte-exact")
}

// TestNonTranslatableNeverSpliced verifies the writer never replaces the value
// of a non-translatable content block, even if a tool erroneously attaches a
// target to it — the source value always passes through verbatim.
func TestNonTranslatableNeverSpliced(t *testing.T) {
	t.Parallel()

	parts := readWithConfig(t, nonTranslatableDoc, nil)
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok && !b.Translatable {
			b.SetTargetText("de", "ÜBERSETZT") // should be ignored by the writer
		}
	}

	out := string(writeParts(t, parts, "de"))
	assert.Contains(t, out, `<string name="debug" translatable="false">DEBUG_BUILD_MARKER</string>`)
	assert.Contains(t, out, "<item>internal_flag_a</item>")
	assert.NotContains(t, out, "ÜBERSETZT",
		"a target on a non-translatable block must never be spliced")
}

// TestNonTranslatableCommentNote (issue #928 B) verifies an XML comment
// immediately preceding a now-surfaced translatable="false" entry attaches as a
// developer note on that surfaced block.
func TestNonTranslatableCommentNote(t *testing.T) {
	t.Parallel()

	doc := `<resources>
    <!-- Internal debug marker, do not localize. -->
    <string name="debug" translatable="false">MARKER</string>
</resources>
`
	parts := readWithConfig(t, doc, nil)
	by := blockByName(parts)

	b := by["debug"]
	require.NotNil(t, b)
	assert.False(t, b.Translatable)
	notes := b.Notes()
	require.Len(t, notes, 1, "the preceding comment attaches as a note")
	assert.Equal(t, "Internal debug marker, do not localize.", notes[0].Text)
	assert.Equal(t, "developer", notes[0].From)
}

// TestCommentCarriedAcrossReference (issue #928 B) verifies that a comment
// preceding a bare resource reference — which stays skeleton, not a surfaced
// entry — is not dropped but carried forward to the next surfaced (translatable)
// entry instead.
func TestCommentCarriedAcrossReference(t *testing.T) {
	t.Parallel()

	doc := `<resources>
    <!-- Context for the surrounding strings. -->
    <string name="alias">@string/other</string>
    <string name="real">Hello</string>
</resources>
`
	parts := readWithConfig(t, doc, nil)
	by := blockByName(parts)

	// The reference is not extracted (stays skeleton).
	assert.NotContains(t, by, "alias")

	real := by["real"]
	require.NotNil(t, real)
	notes := real.Notes()
	require.Len(t, notes, 1,
		"the comment carries past the reference onto the next surfaced entry")
	assert.Equal(t, "Context for the surrounding strings.", notes[0].Text)
}
