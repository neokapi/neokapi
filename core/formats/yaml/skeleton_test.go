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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}}}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Au revoir"}}}}}
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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}}}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Au revoir"}}}}}
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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Hallo"}}}}}
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
