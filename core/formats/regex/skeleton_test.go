package regex_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/regex"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string, rules []regex.Rule) string {
	t.Helper()
	ctx := t.Context()

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = rules

	writer := regex.NewWriter()
	_ = writer.SetConfig(cfg)

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

func TestSkeletonStore_ByteExact_MacStrings(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `"([^"]*?)"\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
	input := `/* Welcome message */
"greeting" = "Hello, World!";
"farewell" = "Goodbye!";
`
	output := snippetRoundtripWithSkeleton(t, input, rules)
	assert.Equal(t, input, output, "Mac .strings roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_INI(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `(?m)^([^=\[\]#;\s]+)\s*=\s*(.+)$`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
	input := "[section]\nkey1=value1\nkey2=value2\n"
	output := snippetRoundtripWithSkeleton(t, input, rules)
	assert.Equal(t, input, output, "INI format roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_NoRules(t *testing.T) {
	input := "Just plain text\nwith no rules\n"
	output := snippetRoundtripWithSkeleton(t, input, nil)
	assert.Equal(t, input, output, "no-rules roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `"([^"]*?)"\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
	input := ""
	output := snippetRoundtripWithSkeleton(t, input, rules)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `"([^"]*?)"\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
	input := `"greeting" = "Hello";
"farewell" = "Goodbye";
`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = rules

	writer := regex.NewWriter()
	_ = writer.SetConfig(cfg)

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
			case "Hello":
				b.SetTargetText(locale, "Bonjour")
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

	expected := `"greeting" = "Bonjour";
"farewell" = "Au revoir";
`
	assert.Equal(t, expected, buf.String())
}
