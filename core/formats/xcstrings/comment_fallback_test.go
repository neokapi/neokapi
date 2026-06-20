package xcstrings_test

import (
	"testing"

	xcstrings "github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readInlineXC reads inline .xcstrings bytes through a reader whose config has
// `cfg` applied (nil = defaults), returning all parts.
func readInlineXC(t *testing.T, data string, cfg map[string]any) []*model.Part {
	t.Helper()
	r := xcstrings.NewReader()
	if cfg != nil {
		require.NoError(t, r.Config().ApplyMap(cfg))
	}
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "in.xcstrings", Encoding: "UTF-8", Reader: ioNopCloser([]byte(data)),
	}))
	defer r.Close()
	return testutil.CollectParts(t, r.Read(t.Context()))
}

// commentFallbackCatalog has four entries exercising the zero-leaf paths:
//   - NoLoc:      comment, no "localizations" key at all
//   - EmptyLoc:   comment, an empty "localizations" object
//   - Translated: comment + a real localization (a leaf is emitted)
//   - NoComment:  no comment and no leaf (nothing should surface)
const commentFallbackCatalog = `{
  "sourceLanguage" : "en",
  "strings" : {
    "NoLoc" : {
      "comment" : "Shown on the splash screen"
    },
    "EmptyLoc" : {
      "comment" : "Accessibility label",
      "localizations" : {

      }
    },
    "Translated" : {
      "comment" : "Primary action",
      "localizations" : {
        "fr" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "OK"
          }
        }
      }
    },
    "NoComment" : {

    }
  },
  "version" : "1.0"
}
`

func blocksByNameXC(blocks []*model.Block) map[string]*model.Block {
	m := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		m[b.Name] = b
	}
	return m
}

// TestCommentFallback_SurfacesDeveloperComment verifies that, with surfacing on
// (the default), an entry that produces no translatable leaf still surfaces its
// developer comment via a non-translatable fallback block whose source is the
// entry key.
func TestCommentFallback_SurfacesDeveloperComment(t *testing.T) {
	parts := readInlineXC(t, commentFallbackCatalog, nil)
	blocks := testutil.FilterBlocks(parts)
	byName := blocksByNameXC(blocks)

	// NoLoc + EmptyLoc fallback blocks + the Translated/fr leaf = 3 blocks.
	// NoComment yields nothing (no leaf, no comment).
	require.Len(t, blocks, 3)
	_, hasNoComment := byName["NoComment"]
	assert.False(t, hasNoComment, "entry with no leaf and no comment must not surface a block")

	for _, tc := range []struct{ name, comment string }{
		{"NoLoc", "Shown on the splash screen"},
		{"EmptyLoc", "Accessibility label"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := byName[tc.name]
			require.NotNil(t, b, "fallback block must exist")

			// Non-translatable, skipped by MT.
			assert.False(t, b.Translatable, "fallback block must not be translatable")
			// Source is the entry key.
			assert.Equal(t, tc.name, model.RenderRunsWithData(b.SourceRuns()))
			// Entry key carried, but NOT under the writer's value-splice key.
			assert.Equal(t, tc.name, b.Properties["xcstrings.entryKey"])
			_, hasKey := b.Properties["xcstrings.key"]
			assert.False(t, hasKey, "fallback block must not set xcstrings.key (writer-splice path)")
			// Developer comment reachable as a note.
			notes := b.Notes()
			require.Len(t, notes, 1)
			assert.Equal(t, tc.comment, notes[0].Text)
			assert.Equal(t, "developer", notes[0].From)
		})
	}

	// The real localization still emits a translatable leaf carrying its comment.
	translated := byName["Translated/fr"]
	require.NotNil(t, translated)
	assert.True(t, translated.Translatable)
	assert.Equal(t, "OK", model.RenderRunsWithData(translated.TargetRuns("fr")))
	require.Len(t, translated.Notes(), 1)
	assert.Equal(t, "Primary action", translated.Notes()[0].Text)
}

// TestCommentFallback_OffKeepsPartStreamByteIdentical verifies that with
// surfacing off (what the parity runner forces) no fallback block is emitted —
// only the genuine translatable leaf remains, so the part stream matches the
// prior behavior.
func TestCommentFallback_OffKeepsPartStreamByteIdentical(t *testing.T) {
	parts := readInlineXC(t, commentFallbackCatalog, map[string]any{
		"extractNonTranslatableContent": false,
	})
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1, "only the genuine translatable leaf must remain")
	assert.Equal(t, "Translated/fr", blocks[0].Name)
	assert.True(t, blocks[0].Translatable)
}

// TestCommentFallback_RoundTripByteExact verifies that the added fallback blocks
// do not disturb the byte-faithful round-trip: the writer ignores them (they
// carry no value reference) and reproduces the original document exactly, on
// both the original-bytes path and the skeleton (merge) path.
func TestCommentFallback_RoundTripByteExact(t *testing.T) {
	t.Run("original_bytes", func(t *testing.T) {
		parts := readInlineXC(t, commentFallbackCatalog, nil)
		out := writeParts(t, parts, "")
		assert.Equal(t, commentFallbackCatalog, string(out))
	})

	t.Run("skeleton_merge", func(t *testing.T) {
		out := skeletonRoundtripXC(t, commentFallbackCatalog)
		assert.Equal(t, commentFallbackCatalog, out)
	})
}

const staleCommentCatalog = `{
  "sourceLanguage" : "en",
  "strings" : {
    "OldKey" : {
      "extractionState" : "stale",
      "comment" : "Deprecated; remove next release",
      "localizations" : {
        "fr" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Vieux"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`

// TestCommentFallback_StaleSkipped verifies the stale path: when extractStale is
// off the entry emits no translatable leaf, but its developer comment is still
// surfaced via a fallback block (carrying the stale extractionState) when
// surfacing is on, and nothing is emitted when surfacing is off.
func TestCommentFallback_StaleSkipped(t *testing.T) {
	t.Run("default_extracts_leaf_no_fallback", func(t *testing.T) {
		// extractStale defaults true: the stale entry emits a normal leaf and
		// carries its comment there, so no fallback is produced.
		blocks := testutil.FilterBlocks(readInlineXC(t, staleCommentCatalog, nil))
		require.Len(t, blocks, 1)
		assert.True(t, blocks[0].Translatable)
		assert.Equal(t, "OldKey/fr", blocks[0].Name)
	})

	t.Run("stale_off_surfaces_comment_fallback", func(t *testing.T) {
		blocks := testutil.FilterBlocks(readInlineXC(t, staleCommentCatalog, map[string]any{
			"extractStale": false,
		}))
		require.Len(t, blocks, 1)
		b := blocks[0]
		assert.False(t, b.Translatable)
		assert.Equal(t, "OldKey", b.Name)
		assert.Equal(t, "stale", b.Properties["xcstrings.extractionState"])
		require.Len(t, b.Notes(), 1)
		assert.Equal(t, "Deprecated; remove next release", b.Notes()[0].Text)
	})

	t.Run("stale_off_surfacing_off_emits_nothing", func(t *testing.T) {
		blocks := testutil.FilterBlocks(readInlineXC(t, staleCommentCatalog, map[string]any{
			"extractStale":                  false,
			"extractNonTranslatableContent": false,
		}))
		assert.Empty(t, blocks, "stale-skipped entry with surfacing off must emit no block")
	})
}

// TestCommentFallback_ConfigToggles pins the config plumbing for the flag.
func TestCommentFallback_ConfigToggles(t *testing.T) {
	c := &xcstrings.Config{}
	c.Reset()
	assert.True(t, c.ExtractNonTranslatableContent(), "default must be on")

	require.NoError(t, c.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, c.ExtractNonTranslatableContent())

	c.SetExtractNonTranslatableContent(true)
	assert.True(t, c.ExtractNonTranslatableContent())

	err := c.ApplyMap(map[string]any{"extractNonTranslatableContent": "nope"})
	require.Error(t, err, "non-bool value must be rejected")
}
