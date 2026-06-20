package wiki_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover #928: surfacing renderable non-translatable contextual
// content (DokuWiki `<code>`/`<file>`/`<html>`/`<php>` tagged block bodies and
// indented code blocks) as model.Block{Translatable:false, Role:code} that is
// visible to ingestion/LLM consumers but skipped by machine translation. The
// behaviour is on by default and opt-out via SetExtractNonTranslatableContent.

// readNoExtract reads via the no-skeleton (scanner) path with surfacing opted
// out — the contextual content stays in Data parts exactly as before #928.
func readNoExtract(t *testing.T, content string) []*model.Part {
	t.Helper()
	reader := wiki.NewReader()
	reader.Config().(*wiki.Config).SetExtractNonTranslatableContent(false)
	return readString(t, reader, content)
}

// roundtripSkeleton drives the byte-exact skeleton path with surfacing toggled
// by extract, returning both the emitted parts and the reconstructed output.
func roundtripSkeleton(t *testing.T, input string, extract bool) ([]*model.Part, string) {
	t.Helper()
	ctx := t.Context()

	reader := wiki.NewReader()
	reader.Config().(*wiki.Config).SetExtractNonTranslatableContent(extract)
	writer := wiki.NewWriter()

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

	return parts, buf.String()
}

func countDataParts(parts []*model.Part) int {
	n := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			n++
		}
	}
	return n
}

// ── Default-on surfacing (no-skeleton scanner path) ──────────────────────

func TestNonTranslatable_TaggedCodeBlock_DefaultOn(t *testing.T) {
	parts := readDefault(t, "<code php>\necho \"hi\";\n</code>\n")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	b := blocks[0]
	assert.False(t, b.Translatable, "tagged code body must be non-translatable")
	assert.Equal(t, model.RoleCode, b.SemanticRole())
	assert.True(t, b.PreserveWhitespace)
	assert.Equal(t, "echo \"hi\";", b.SourceText())
	assert.Equal(t, "php", b.Properties["language"])
}

// The first attribute token is the highlight language; an optional second
// token is the file name — both ride the surfaced block's Properties.
func TestNonTranslatable_TaggedFileBlock_LangAndName(t *testing.T) {
	parts := readDefault(t, "<file php list.php>\n$x = 1;\nfoo();\n</file>\n")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	b := blocks[0]
	assert.False(t, b.Translatable)
	assert.Equal(t, model.RoleCode, b.SemanticRole())
	assert.Equal(t, "$x = 1;\nfoo();", b.SourceText())
	assert.Equal(t, "php", b.Properties["language"])
	assert.Equal(t, "list.php", b.Properties["name"])
}

// A single contiguous run of indented (CODE_START) lines surfaces as ONE
// RoleCode block, surrounded by its translatable neighbours unchanged.
func TestNonTranslatable_IndentedCode_DefaultOn(t *testing.T) {
	parts := readDefault(t, "Intro.\n\n  echo 1\n  echo 2\n\nDone.\n")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)
	assert.Equal(t, "Intro.", blocks[0].SourceText())
	assert.True(t, blocks[0].Translatable)

	code := blocks[1]
	assert.False(t, code.Translatable)
	assert.Equal(t, model.RoleCode, code.SemanticRole())
	assert.True(t, code.PreserveWhitespace)
	assert.Equal(t, "  echo 1\n  echo 2", code.SourceText())

	assert.Equal(t, "Done.", blocks[2].SourceText())
	assert.True(t, blocks[2].Translatable)
}

// ── Opt-out: behaviour byte-identical to before #928 ─────────────────────

func TestNonTranslatable_TaggedCodeBlock_FlagOff(t *testing.T) {
	parts := readNoExtract(t, "<code php>\necho 1;\n</code>\n")
	assert.Empty(t, testutil.FilterBlocks(parts),
		"with surfacing off, tagged code stays non-block Data")
	assert.Equal(t, 3, countDataParts(parts),
		"opener, body and closer each remain a Data part")
}

func TestNonTranslatable_IndentedCode_FlagOff(t *testing.T) {
	parts := readNoExtract(t, "  echo 1\n  echo 2\n")
	assert.Empty(t, testutil.FilterBlocks(parts))
	assert.Equal(t, 2, countDataParts(parts), "each indented line stays a Data part")
}

// ── Skeleton path: surfaces the block AND round-trips byte-exact ──────────

func TestNonTranslatable_Skeleton_TaggedBlock(t *testing.T) {
	input := "Intro\n\n<code php>\necho \"hi\";\nmore();\n</code>\n\nOutro\n"
	parts, output := roundtripSkeleton(t, input, true)
	assert.Equal(t, input, output, "tagged code block must round-trip byte-exact")

	blocks := testutil.FilterBlocks(parts)
	var code *model.Block
	for _, b := range blocks {
		if b.SemanticRole() == model.RoleCode {
			code = b
		}
	}
	require.NotNil(t, code, "tagged code body should surface as a RoleCode block")
	assert.False(t, code.Translatable)
	assert.True(t, code.PreserveWhitespace)
	// On the skeleton path the verbatim body (with line endings) rides the ref.
	assert.Equal(t, "echo \"hi\";\nmore();\n", code.SourceText())
	assert.Equal(t, "php", code.Properties["language"])
}

func TestNonTranslatable_Skeleton_IndentedCode(t *testing.T) {
	input := "Intro\n\n  echo 1\n  echo 2\n\nOutro\n"
	parts, output := roundtripSkeleton(t, input, true)
	assert.Equal(t, input, output, "indented code must round-trip byte-exact")

	blocks := testutil.FilterBlocks(parts)
	var code *model.Block
	for _, b := range blocks {
		if b.SemanticRole() == model.RoleCode {
			code = b
		}
	}
	require.NotNil(t, code, "indented code should surface as a single RoleCode block")
	assert.False(t, code.Translatable)
	assert.Equal(t, "  echo 1\n  echo 2\n", code.SourceText())
}

// A `<code>foo</code>` opener that closes on its own line is a single-line
// construct: it stays a Data marker (no body block) on both flag settings,
// and round-trips byte-exact.
func TestNonTranslatable_Skeleton_SingleLineTag(t *testing.T) {
	input := "<code>inline</code>\n"
	parts, output := roundtripSkeleton(t, input, true)
	assert.Equal(t, input, output)
	assert.Empty(t, testutil.FilterBlocks(parts),
		"a same-line <code>…</code> carries no multi-line body block")
}

// Opt-out on the skeleton path: still byte-exact, still no RoleCode block,
// markers + body preserved as Data parts (unchanged from before #928).
func TestNonTranslatable_Skeleton_FlagOff(t *testing.T) {
	input := "<code php>\necho 1;\n</code>\n"
	parts, output := roundtripSkeleton(t, input, false)
	assert.Equal(t, input, output, "flag-off skeleton round-trip stays byte-exact")
	assert.Empty(t, testutil.FilterBlocks(parts))
	assert.Equal(t, 3, countDataParts(parts))
}

// The toggle is independent of how the Config is constructed: a zero-value
// Config still defaults to surfacing on.
func TestNonTranslatable_DefaultOn_ZeroValueConfig(t *testing.T) {
	var c wiki.Config
	assert.True(t, c.ExtractNonTranslatableContent(),
		"zero-value Config must default to surfacing on")
	require.NoError(t, c.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, c.ExtractNonTranslatableContent())
	require.NoError(t, c.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, c.ExtractNonTranslatableContent())
}
