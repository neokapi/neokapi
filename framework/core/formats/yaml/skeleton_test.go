package yaml_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snippetRoundtripWithSkeleton does a read→skeleton→write roundtrip via SkeletonStore.
func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

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
			output := snippetRoundtripWithSkeleton(t, tc.input)
			assert.Equal(t, tc.input, output, "skeleton store roundtrip should be byte-exact")
		})
	}
}

func TestSkeletonStore_LiteralBlock(t *testing.T) {
	input := "description: |\n  This is a literal\n  block scalar.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "literal block scalar should roundtrip byte-exact")
}

func TestSkeletonStore_FoldedBlock(t *testing.T) {
	input := "description: >\n  This is a folded\n  block scalar.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "folded block scalar should roundtrip byte-exact")
}

func TestSkeletonStore_LiteralBlockChompKeep(t *testing.T) {
	input := "key: |+\n  Line one\n  Line two\n\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "literal block with keep chomp should roundtrip byte-exact")
}

func TestSkeletonStore_LiteralBlockChompStrip(t *testing.T) {
	input := "key: |-\n  Line one\n  Line two\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "literal block with strip chomp should roundtrip byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "greeting: Hello World\nfarewell: Goodbye\n"
	ctx := context.Background()
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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Bonjour le monde")}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Au revoir")}}
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
	input := "greeting: \"Hello World\"\nfarewell: \"Goodbye\"\n"
	ctx := context.Background()
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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Bonjour le monde")}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Au revoir")}}
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
	input := "parent:\n  child: Hello\n"
	ctx := context.Background()
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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Hallo")}}
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
	input := "title: Hello World\nnested:\n  description: A description\n  count: 42\ntags:\n  - a\n  - b\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "skeleton store should preserve all formatting")
}

func TestSkeletonStore_RailsStyle(t *testing.T) {
	input := "en:\n  title: My Rails Website\n  items:\n    - test1\n    - test2\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "Rails-style YAML should roundtrip byte-exact")
}
