package yaml_test

import (
	"bytes"
	"strings"
	"testing"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snippetRoundtripWithoutSkeleton does a read → write roundtrip without
// configuring a SkeletonStore. This exercises the writer's "rebuild from
// blocks" path (Mode 2) — the path the parity round-trip harness uses
// for formats that don't opt into skeleton-backed output.
func snippetRoundtripWithoutSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := yamlfmt.NewReader()
	writer := yamlfmt.NewWriter()

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// TestWriter_PreservesMappingKeyOrder verifies the rebuild-from-blocks
// path keeps the original document's mapping key order. yaml.v3's
// reader is order-preserving (it returns an ordered Node tree), so the
// writer must not lose that ordering when reconstructing the document.
func TestWriter_PreservesMappingKeyOrder(t *testing.T) {
	t.Parallel()
	input := "greeting: Hello world\nfarewell: Goodbye now\n"
	output := snippetRoundtripWithoutSkeleton(t, input)

	greetingIdx := strings.Index(output, "greeting:")
	farewellIdx := strings.Index(output, "farewell:")
	require.NotEqual(t, -1, greetingIdx, "output should contain greeting key: %q", output)
	require.NotEqual(t, -1, farewellIdx, "output should contain farewell key: %q", output)
	assert.Less(t, greetingIdx, farewellIdx,
		"greeting must appear before farewell (original document order); got:\n%s", output)
}

// TestWriter_PreservesManyKeyOrder uses keys that are intentionally
// not in alphabetical order, so any sort-by-key behaviour in the
// encoder becomes obvious regardless of map iteration randomness.
func TestWriter_PreservesManyKeyOrder(t *testing.T) {
	t.Parallel()
	input := "zebra: stripes\napple: red\nmango: orange\nbanana: yellow\n"
	output := snippetRoundtripWithoutSkeleton(t, input)

	want := []string{"zebra:", "apple:", "mango:", "banana:"}
	prev := -1
	for _, key := range want {
		idx := strings.Index(output, key)
		require.NotEqual(t, -1, idx, "output missing %s; got:\n%s", key, output)
		assert.Greater(t, idx, prev, "key %s out of order in:\n%s", key, output)
		prev = idx
	}
}

// TestWriter_KeepsEveryBlockWhenKeyPathsRepeat pins the rebuild path against a
// document where two blocks share a key path. The reader emits a block per key
// occurrence and the skeleton path reproduces the document byte-for-byte; the
// rebuild path keyed its block map by the key path, so a repeat collapsed two
// blocks into one and a translatable string went out of the document with
// nothing logged (#1597).
//
// The consequence is not "a duplicate got deduplicated". Both strings were
// handed to the pipeline, so both were translated — paid for, and written into
// content memory — and then one translation was discarded on write-back. The
// two write paths disagreed about how many strings the document contains.
func TestWriter_KeepsEveryBlockWhenKeyPathsRepeat(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		input string
		want  []string
	}{
		"repeated top-level key": {
			input: "title: First\ntitle: Second\n",
			want:  []string{"First", "Second"},
		},
		"repeated nested key": {
			input: "a:\n  k: one\n  k: two\n",
			want:  []string{"one", "two"},
		},
		"three occurrences": {
			input: "k: a\nk: b\nk: c\n",
			want:  []string{"a", "b", "c"},
		},
		"repeat interleaved with distinct keys": {
			input: "k: one\nother: x\nk: two\n",
			want:  []string{"one", "two", "x"},
		},
		"distinct keys are untouched": {
			input: "greeting: Hello\nfarewell: Goodbye\n",
			want:  []string{"Goodbye", "Hello"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, tc.want, yamlBlockTexts(t, tc.input),
				"the reader must extract every occurrence")

			output := snippetRoundtripWithoutSkeleton(t, tc.input)
			assert.ElementsMatch(t, tc.want, yamlBlockTexts(t, output),
				"the rebuild path must not drop a block; output:\n%s", output)
		})
	}
}

// yamlBlockTexts reads a YAML document and returns its blocks' source texts.
func yamlBlockTexts(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	var texts []string
	for _, b := range testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx))) {
		texts = append(texts, b.SourceText())
	}
	return texts
}

// TestWriter_PreservesLeadingNewline guards #1630: the rebuild-from-blocks
// path must keep a block's leading "\n". A bare ScalarNode lets yaml.v3
// auto-select a literal block scalar (`|2-`, `|2`) that drops the leading
// blank line, so the text read back loses its leading newline; the writer
// remaps a leading-newline value to a double-quoted scalar, which escapes
// the newline and round-trips it byte-for-byte. The drift corrupts the
// content-derived block identity (AD-036), so content-memory matches stop
// hitting after a conversion.
func TestWriter_PreservesLeadingNewline(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"double_quoted_escape", "a: \"\\nx\"\n", "\nx"},
		{"literal_block_blank_first_line", "k: |\n\n  0\n", "\n0\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Sanity: the reader parses the leading newline in the first place.
			before := yamlBlockTexts(t, tc.input)
			require.Len(t, before, 1, "input should read to exactly one block: %q", tc.input)
			require.Equal(t, tc.want, before[0], "reader lost leading newline on input %q", tc.input)

			// read → rebuild-write → read must preserve the leading newline.
			rebuilt := snippetRoundtripWithoutSkeleton(t, tc.input)
			after := yamlBlockTexts(t, rebuilt)
			require.Len(t, after, 1, "rebuilt output should read to one block; got:\n%s", rebuilt)
			assert.Equal(t, tc.want, after[0],
				"leading newline dropped through rebuild path\ninput:   %q\nrebuilt: %q", tc.input, rebuilt)
		})
	}
}

// TestWriter_PreservesNestedKeyOrder verifies order inside a nested
// mapping is also preserved.
func TestWriter_PreservesNestedKeyOrder(t *testing.T) {
	t.Parallel()
	input := "messages:\n  greeting: Hello\n  farewell: Goodbye\n"
	output := snippetRoundtripWithoutSkeleton(t, input)

	greetingIdx := strings.Index(output, "greeting:")
	farewellIdx := strings.Index(output, "farewell:")
	require.NotEqual(t, -1, greetingIdx, "output should contain greeting key: %q", output)
	require.NotEqual(t, -1, farewellIdx, "output should contain farewell key: %q", output)
	assert.Less(t, greetingIdx, farewellIdx,
		"greeting must precede farewell in nested mapping; got:\n%s", output)
}
