package yaml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snippetRoundtripWithSkeleton does a read→skeleton→write roundtrip via SkeletonStore.
func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := yamlfmt.NewReader()
	writer := yamlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"simple_plain", "key: value\n"},
		{"simple_double_quoted", "key: \"Hello World\"\n"},
		{"simple_single_quoted", "key: 'Hello World'\n"},
		{"nested", "parent:\n  child: value\n  other: text\n"},
		{"deep_nesting", "root:\n  level1:\n    level2: deep value\n"},
		{"multi_keys", "a: value_a\nb: value_b\n"},
		{"array", "items:\n  - First\n  - Second\n  - Third\n"},
		{"flow_mapping", "person: {name: Alice, role: Developer}\n"},
		{"flow_sequence", "colors: [red, green, blue]\n"},
		{"comment", "# top comment\nkey: value\n"},
		{"inline_comment", "key: value # inline comment\n"},
		{"multi_document", "---\ntitle: Document One\n---\ntitle: Document Two\n"},
		{"mixed_types", "name: Test\ncount: 42\nactive: true\n"},
		{"double_quoted_escapes", "key: \"Hello\\tWorld\\nNew line\"\n"},
		{"single_quoted_escape", "key: 'It''s a test'\n"},
		{"unicode", "greeting: \u4f60\u597d\u4e16\u754c\n"},
		{"empty_doc", ""},
		{"comments_only", "# This is a comment\n# Another comment\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output := snippetRoundtripWithSkeleton(t, tc.input)
			assert.Equal(t, tc.input, output, "skeleton store roundtrip should be byte-exact")
		})
	}
}

// translateRoundtripWithSkeleton does a read→skeleton→write roundtrip,
// pseudo-translating every block by prefixing its source text with "X"
// (and applying it to every text run). It mirrors the parity harness's
// inline pseudo so the test exercises the re-encode path that the
// byte-exact (untranslated) path skips. Returns the merged output.
func translateRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := yamlfmt.NewReader()
	writer := yamlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		runs := make([]model.Run, 0, len(b.Source))
		for _, r := range b.Source {
			if r.Text != nil {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: "X" + r.Text.Text}})
			} else {
				runs = append(runs, r)
			}
		}
		b.SetTargetRuns(locale, runs)
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// assertNoMixedEOL fails if output contains any LF not preceded by CR —
// i.e. mixed line endings within a CRLF document.
func assertNoMixedEOL(t *testing.T, out string) {
	t.Helper()
	for i := range out {
		if out[i] == '\n' && (i == 0 || out[i-1] != '\r') {
			t.Fatalf("bare LF at offset %d (mixed line endings within CRLF doc): %q", i, out)
		}
	}
}

// TestSkeletonStore_CRLF_ByteExact verifies that an untranslated
// roundtrip preserves the source's CRLF line endings byte-for-byte. The
// earlier bug captured the trailing CR of a plain-scalar line into the
// scalar value range, so the CR survived only as part of the
// (smuggled) raw bytes — masking the defect on the untranslated path.
// This locks the corrected behavior in.
//
// Grounded in YAML 1.2 §5.4 (Line Break Characters): a CR immediately
// preceding an LF is part of the line break and never appears in a
// scalar's parsed value.
func TestSkeletonStore_CRLF_ByteExact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		// Exact shape of okapi's Test01.yml fixture.
		{"rails_config", "config:\r\n  title: \"My Rails Website\"\r\nconfig2:\r\n  - test1\r\n  - test2\r\n"},
		{"plain_scalars", "a: one\r\nb: two\r\n"},
		{"sequence", "items:\r\n  - First\r\n  - Second\r\n  - Third\r\n"},
		{"double_quoted", "key: \"Hello World\"\r\n"},
		{"single_quoted", "key: 'Hello World'\r\n"},
		{"inline_comment", "key: value # note\r\n"},
		{"literal_block", "desc: |\r\n  line one\r\n  line two\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output := snippetRoundtripWithSkeleton(t, tc.input)
			assert.Equal(t, tc.input, output, "CRLF source must roundtrip byte-exact")
		})
	}
}

// TestSkeletonStore_CRLF_TranslatedPreservesEOL verifies the re-encode
// path: when scalars are translated, the writer must still emit the
// source's CRLF convention consistently — never a mix. This is the
// regression the parity harness flagged on Test01.yml (list items came
// out with bare LF while sibling keys kept CRLF).
//
// Mirrors Okapi YamlFilter: it detects the source newline type via
// BOMNewlineEncodingDetector and replays it through
// YamlSkeletonWriter.getEncoderManager().getLineBreak() on every break.
func TestSkeletonStore_CRLF_TranslatedPreservesEOL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "rails_config",
			input: "config:\r\n  title: \"My Rails Website\"\r\nconfig2:\r\n  - test1\r\n  - test2\r\n",
			want:  "config:\r\n  title: \"XMy Rails Website\"\r\nconfig2:\r\n  - Xtest1\r\n  - Xtest2\r\n",
		},
		{
			name:  "plain_list_items",
			input: "items:\r\n  - one\r\n  - two\r\n",
			want:  "items:\r\n  - Xone\r\n  - Xtwo\r\n",
		},
		{
			// Multi-line literal block: the re-encoded scalar's internal
			// line break must adopt the source CRLF too — exercises the
			// writer-side applyEOL path (the surrounding skeleton already
			// carries CRLF; without applyEOL the body's break would be LF).
			name:  "literal_block",
			input: "desc: |\r\n  line one\r\n  line two\r\n",
			want:  "desc: |\r\n  Xline one\r\n  line two\r\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := translateRoundtripWithSkeleton(t, tc.input)
			assertNoMixedEOL(t, out)
			assert.Equal(t, tc.want, out, "translated CRLF roundtrip must keep CRLF throughout")
		})
	}
}

// TestSkeletonStore_LF_TranslatedNoRegression proves the EOL handling
// never rewrites an LF source: bare-LF documents must stay bare-LF after
// a translated roundtrip (the dominant-EOL detector returns "\n" and the
// writer leaves output untouched).
func TestSkeletonStore_LF_TranslatedNoRegression(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain_list_items", "items:\n  - one\n  - two\n", "items:\n  - Xone\n  - Xtwo\n"},
		{"literal_block", "desc: |\n  line one\n  line two\n", "desc: |\n  Xline one\n  line two\n"},
		{"rails_config", "config:\n  title: \"My Rails Website\"\nconfig2:\n  - test1\n  - test2\n", "config:\n  title: \"XMy Rails Website\"\nconfig2:\n  - Xtest1\n  - Xtest2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := translateRoundtripWithSkeleton(t, tc.input)
			assert.NotContains(t, out, "\r", "LF source must never gain CR on roundtrip")
			assert.Equal(t, tc.want, out)
		})
	}
}

func TestSkeletonStore_LiteralBlock(t *testing.T) {
	t.Parallel()
	input := "description: |\n  This is a literal\n  block scalar.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "literal block scalar should roundtrip byte-exact")
}

func TestSkeletonStore_FoldedBlock(t *testing.T) {
	t.Parallel()
	input := "description: >\n  This is a folded\n  block scalar.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "folded block scalar should roundtrip byte-exact")
}

func TestSkeletonStore_LiteralBlockChompKeep(t *testing.T) {
	t.Parallel()
	input := "key: |+\n  Line one\n  Line two\n\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "literal block with keep chomp should roundtrip byte-exact")
}

func TestSkeletonStore_LiteralBlockChompStrip(t *testing.T) {
	t.Parallel()
	input := "key: |-\n  Line one\n  Line two\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "literal block with strip chomp should roundtrip byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	t.Parallel()
	input := "greeting: Hello World\nfarewell: Goodbye\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := yamlfmt.NewReader()
	writer := yamlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello World":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	assert.Equal(t, "greeting: Bonjour le monde\nfarewell: Au revoir\n", output)
}

func TestSkeletonStore_WithTranslation_DoubleQuoted(t *testing.T) {
	t.Parallel()
	input := "greeting: \"Hello World\"\nfarewell: \"Goodbye\"\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := yamlfmt.NewReader()
	writer := yamlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello World":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	assert.Equal(t, "greeting: \"Bonjour le monde\"\nfarewell: \"Au revoir\"\n", output)
}

func TestSkeletonStore_WithTranslation_Nested(t *testing.T) {
	t.Parallel()
	input := "parent:\n  child: Hello\n"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := yamlfmt.NewReader()
	writer := yamlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.SetTargetText(locale, "Hallo")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "parent:\n  child: Hallo\n", buf.String())
}

func TestSkeletonStore_PreservesFormatting(t *testing.T) {
	t.Parallel()
	input := "title: Hello World\nnested:\n  description: A description\n  count: 42\ntags:\n  - a\n  - b\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "skeleton store should preserve all formatting")
}

func TestSkeletonStore_RailsStyle(t *testing.T) {
	t.Parallel()
	input := "en:\n  title: My Rails Website\n  items:\n    - test1\n    - test2\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "Rails-style YAML should roundtrip byte-exact")
}

// Native equivalent of Okapi's RoundTripYamlIT: it iterates the okf_yaml file
// corpus through an EventComparator, asserting the extracted text units are
// stable across an extract→merge→re-extract roundtrip. This test reproduces the
// same observable contract over a representative set of YAML constructs: read,
// write back through the skeleton store with no translation, re-extract, and
// assert the source text units are identical.
//
// okapi: RoundTripYamlIT#yamlFiles
// okapi: YamlXliffCompareIT#yamlXliffCompareFiles
// okapi-skip: RoundTripYamlIT#yamlSerializedFiles — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
func TestRoundTrip_YamlIT(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"simple", "title: Hello\ndescription: World\n"},
		{"nested", "en:\n  title: My Rails Website\n  greeting: Hello\n"},
		{"deep_nesting", "root:\n  level1:\n    level2: deep value\n"},
		{"sequence", "items:\n  - First\n  - Second\n  - Third\n"},
		{"quoted_escapes", "key1: \"Hello\\tWorld\"\nkey2: \"Line1\\nLine2\"\n"},
		{"literal_block", "description: |\n  This is a literal\n  block scalar.\n"},
		{"folded_block", "description: >\n  This is a folded\n  block scalar.\n"},
		{"inline_comment", "key: value # inline comment\n"},
		{"multi_document", "---\ntitle: Document One\n---\ntitle: Document Two\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			merged := snippetRoundtripWithSkeleton(t, tc.input)
			first := blockTexts(readYAML(t, tc.input))
			second := blockTexts(readYAML(t, merged))
			require.NotEmpty(t, first, "%s should produce translatable blocks", tc.name)
			assert.Equal(t, first, second,
				"%s text units must be stable across an extract→write→re-extract roundtrip", tc.name)
		})
	}
}
