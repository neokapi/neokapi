package properties_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readPartsWithConfig reads input through the native reader after applying
// the given config map.
func readPartsWithConfig(t *testing.T, input string, cfg map[string]any) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := properties.NewReader()
	require.NoError(t, reader.Config().ApplyMap(cfg))
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

// #928 finding A: the value text of an entry excluded by the key condition is
// surfaced as a non-translatable content Block (visible to ingestion, skipped
// by MT) when ExtractNonTranslatableContent is on (the default).
func TestExcludedEntry_SurfacesContentBlock_WhenOn(t *testing.T) {
	input := "title.text=Hello\nbuild.number=42\n"
	parts := readPartsWithConfig(t, input, map[string]any{
		"useKeyCondition": true,
		"keyCondition":    ".*text.*",
	})
	blocks := testutil.FilterBlocks(parts)

	// One translatable block for the matching key.
	var translatable, excluded []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			translatable = append(translatable, b)
		} else {
			excluded = append(excluded, b)
		}
	}
	require.Len(t, translatable, 1)
	assert.Equal(t, "title.text", translatable[0].Name)
	assert.Equal(t, "Hello", translatable[0].SourceText())

	// The excluded key surfaces as a non-translatable content block whose
	// source is the value text, as a single verbatim run.
	require.Len(t, excluded, 1)
	cb := excluded[0]
	assert.Equal(t, "build.number", cb.Name)
	assert.Equal(t, "42", cb.SourceText())
	assert.False(t, cb.Translatable)
	assert.True(t, cb.PreserveWhitespace)
	assert.Len(t, cb.Source, 1, "value is a single verbatim run, not inline-parsed")
	assert.Empty(t, cb.SemanticRole(), "plain value carries no semantic role")

	// No Data{skipped-entry} when surfacing is on.
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok {
				assert.NotEqual(t, "skipped-entry", d.Name)
			}
		}
	}
}

// #928 finding A (flag off): with surfacing disabled the part stream is the
// prior shape — the value stays in skeleton and a Data{skipped-entry} carries
// only the key. This is the parity-forced path.
func TestExcludedEntry_StaysSkeletonData_WhenOff(t *testing.T) {
	input := "title.text=Hello\nbuild.number=42\n"
	parts := readPartsWithConfig(t, input, map[string]any{
		"useKeyCondition":               true,
		"keyCondition":                  ".*text.*",
		"extractNonTranslatableContent": false,
	})
	blocks := testutil.FilterBlocks(parts)

	// Only the translatable block is emitted — no non-translatable content
	// block for the excluded entry.
	require.Len(t, blocks, 1)
	assert.True(t, blocks[0].Translatable)
	assert.Equal(t, "title.text", blocks[0].Name)

	// The excluded entry is a Data{skipped-entry} carrying only the key.
	var skipped *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok && d.Name == "skipped-entry" {
				skipped = d
			}
		}
	}
	require.NotNil(t, skipped)
	assert.Equal(t, "build.number", skipped.Properties["key"])
}

// #928 finding A: byte-exact skeleton round-trip holds whether surfacing is on
// (value rides as a content-block ref) or off (value stays in skeleton text).
func TestExcludedEntry_ByteExactRoundtrip(t *testing.T) {
	input := "title.text=Hello\nbuild.number=42\nlegal.text=Trademark\n"
	for _, surface := range []bool{true, false} {
		t.Run(map[bool]string{true: "on", false: "off"}[surface], func(t *testing.T) {
			ctx := t.Context()
			reader := properties.NewReader()
			writer := properties.NewWriter()
			require.NoError(t, reader.Config().ApplyMap(map[string]any{
				"useKeyCondition":               true,
				"keyCondition":                  ".*text.*",
				"extractNonTranslatableContent": surface,
			}))

			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()
			reader.SetSkeletonStore(store)
			writer.SetSkeletonStore(store)

			require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			require.NoError(t, writer.SetOutputWriter(&buf))
			require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
			writer.Close()

			assert.Equal(t, input, buf.String(), "excluded-entry round-trip must be byte-exact")
		})
	}
}

// #928 finding B: a developer comment preceding an entry attaches to that entry
// as a semantic NoteAnnotation (parity-safe), while still being carried verbatim
// on its own Data{comment} part for round-trip.
func TestComment_AttachedAsNoteAnnotation(t *testing.T) {
	input := "# Shown to end users\nkey=value"
	parts := readPartsWithConfig(t, input, map[string]any{})

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	notes := blocks[0].Notes()
	require.Len(t, notes, 1, "comment is reachable as a NoteAnnotation on the adjacent block")
	assert.Equal(t, "Shown to end users", notes[0].Text)
	assert.Equal(t, "developer", notes[0].From)

	// The comment's raw text still rides on its Data part for round-trip.
	var commentData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok && d.Name == "comment" {
				commentData = d
			}
		}
	}
	require.NotNil(t, commentData)
	assert.Equal(t, "# Shown to end users", commentData.Properties["comment"])
}

// #928 finding B: localization-directive comment lines (#_skip, #_text, …) stay
// opaque — they are never surfaced as translator notes on the following entry.
func TestDirectiveLines_StayOpaque(t *testing.T) {
	input := "#_text\nkey=value"
	parts := readPartsWithConfig(t, input, map[string]any{})

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Empty(t, blocks[0].Notes(), "directive lines must not become translator notes")

	// The directive still rides verbatim on its Data part.
	var directiveData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok && d.Name == "comment" {
				directiveData = d
			}
		}
	}
	require.NotNil(t, directiveData)
	assert.Equal(t, "#_text", directiveData.Properties["comment"])
}
