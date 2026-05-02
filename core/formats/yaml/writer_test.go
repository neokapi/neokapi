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
