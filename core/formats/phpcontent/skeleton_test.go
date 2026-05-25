package phpcontent_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/phpcontent"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func phpSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := phpcontent.NewReader()
	writer := phpcontent.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleAssignment(t *testing.T) {
	input := `<?php $text = 'Hello world';`
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "simple assignment roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_DoubleQuoted(t *testing.T) {
	input := `<?php $text = "Hello world";`
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "double-quoted roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleAssignments(t *testing.T) {
	input := "<?php\n$a = 'Hello';\n$b = 'World';\n"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple assignments should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "<?php\r\n$a = 'Hello';\r\n$b = 'World';\r\n"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved")
}

func TestSkeletonStore_ByteExact_ArrayIndex(t *testing.T) {
	input := `<?php $arr['greeting'] = 'Hello';`
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "array index roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Comments(t *testing.T) {
	input := "<?php\n// This is a comment\n$text = 'Hello';"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "comments should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_MultilineComment(t *testing.T) {
	input := "<?php\n/* Multi\nline\ncomment */\n$text = 'Hello';"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiline comments should be preserved")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_NoStrings(t *testing.T) {
	input := "<?php\n$x = 42;\n"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "code without strings should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "<?php\n$a = 'Hello';\n$b = 'World';\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := phpcontent.NewReader()
	writer := phpcontent.NewWriter()

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
			case "World":
				b.SetTargetText(locale, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "<?php\n$a = 'Bonjour';\n$b = 'Monde';\n", buf.String())
}

func TestSkeletonStore_ByteExact_SkipDirective(t *testing.T) {
	input := "<?php\n//_skip_\n$text = 'Skip this';\n$other = 'Keep this';"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "skip directives should be preserved")
}

func TestSkeletonStore_ByteExact_Heredoc(t *testing.T) {
	input := "<?php $text = <<<EOT\nHello heredoc\nEOT;\n"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "heredoc roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Nowdoc(t *testing.T) {
	input := "<?php $text = <<<'EOT'\nHello nowdoc\nEOT;\n"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "nowdoc roundtrip should be byte-exact")
}
