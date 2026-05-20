package regex_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/regex"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// macStringsRules returns regex rules for Mac .strings format.
// Pattern: "key" = "value";  with optional /* comment */ before.
func macStringsRules() []regex.Rule {
	return []regex.Rule{
		{
			// Match: "key" = "value";
			Pattern:     `"([^"]*?)"\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
}

// macStringsWithNotesRules returns regex rules that also capture comments.
func macStringsWithNotesRules() []regex.Rule {
	return []regex.Rule{
		{
			// Match: /* comment */ \n "key" = "value";
			Pattern:     `/\*\s*(.*?)\s*\*/\s*\n\s*"([^"]*?)"\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`,
			SourceGroup: 3,
			IDGroup:     2,
			NoteGroup:   1,
		},
	}
}

// iniRules returns regex rules for INI key=value format.
func iniRules() []regex.Rule {
	return []regex.Rule{
		{
			Pattern:     `(?m)^([^=\[\]#;\s]+)\s*=\s*(.+)$`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
}

// --- Basic Reader Tests ---

// okapi: RegexFilterTest#testStartDocument
func TestReaderMetadata(t *testing.T) {
	reader := regex.NewReader()
	assert.Equal(t, "regex", reader.Name())
	assert.Equal(t, "Regex Extraction", reader.DisplayName())
}

// okapi: RegexFilterTest#testStartDocument
func TestReaderSignature(t *testing.T) {
	reader := regex.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-regex")
	assert.Contains(t, sig.Extensions, ".strings")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// okapi: RegexFilterTest#testStartDocument
func TestLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	input := `"key1" = "Hello";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "regex", layer.Format)
	assert.NotEmpty(t, layer.ID)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestReadNoRules(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	input := "Some content"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "no rules should produce no blocks")

	// Should still have layer start/end and data
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// --- Mac .strings Tests ---

// okapi: RegexFilterTest#testSimpleRule
func TestMacStringsSimple(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	input := `"key1" = "Hello World";`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "key1", blocks[0].Name)
}

// okapi: RegexFilterTest#testConfigurations
func TestMacStringsMultiple(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	input := "\"File\" = \"File\";\n\"Edit\" = \"Edit\";\n\"Help\" = \"Help\";\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "File", blocks[0].SourceText())
	assert.Equal(t, "Edit", blocks[1].SourceText())
	assert.Equal(t, "Help", blocks[2].SourceText())
}

// okapi: RegexFilterTest#testConfigurations (macStrings from file)
func TestMacStringsFile(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	f, err := os.Open("testdata/test.strings")
	require.NoError(t, err)

	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/test.strings", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "File", blocks[0].Name)
	assert.Equal(t, "Edit", blocks[1].Name)
	assert.Equal(t, "Help", blocks[2].Name)
}

// okapi: RegexFilterTest#testSemicolonInData
func TestMacStringsSemicolonInValue(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	input := "\"item1\" = \"Text1;Text2\";\n\"item2\" = \"Simple\";\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Text1;Text2", blocks[0].SourceText())
	assert.Equal(t, "Simple", blocks[1].SourceText())
}

// --- ID and Name Extraction Tests ---

// okapi: RegexFilterTest#testIDAndText
func TestIDAndText(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `\[(\w+)\]\t(.+)`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}

	input := "[ID1]\tFirst text\n[ID2]\tSecond text\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "First text", blocks[0].SourceText())
	assert.Equal(t, "ID1", blocks[0].Name)
	assert.Equal(t, "Second text", blocks[1].SourceText())
	assert.Equal(t, "ID2", blocks[1].Name)
}

// okapi: RegexFilterTest#testNameExtraction
func TestNameExtraction(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `(\S+)\s*=\s*(.+)`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}

	input := "g1.key1=Text of g1.key1\ng1.key2=Text of g1.key2\ng2.key1=Text of g2.key1\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Text of g1.key1", blocks[0].SourceText())
	assert.Equal(t, "g1.key1", blocks[0].Name)
	assert.Equal(t, "Text of g1.key2", blocks[1].SourceText())
	assert.Equal(t, "g1.key2", blocks[1].Name)
	assert.Equal(t, "Text of g2.key1", blocks[2].SourceText())
	assert.Equal(t, "g2.key1", blocks[2].Name)
}

// --- Note Extraction Tests ---

// okapi: RegexFilterTest#testNoteExtraction
func TestNoteExtraction(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsWithNotesRules()

	input := "/* Menu item */\n\"File\" = \"File\";\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "File", blocks[0].SourceText())

	note, hasNote := blocks[0].Annotations["note"]
	require.True(t, hasNote, "block should have a note annotation")
	noteAnnotation := note.(*model.NoteAnnotation)
	assert.Equal(t, "Menu item", noteAnnotation.Text)
}

// --- Escape Handling Tests ---

// okapi: RegexFilterTest#testBackslashEscapeHandling
func TestBackslashEscape(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `"((?:[^"\\]|\\.)*)"`,
			SourceGroup: 1,
		},
	}
	cfg.EscapeType = regex.EscapeBackslash

	input := `"Hello \"World\""`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello \"World\"", blocks[0].SourceText())
}

// okapi: RegexFilterTest#testBackslashEscapeHandling
func TestBackslashEscapeNewlineTab(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `"((?:[^"\\]|\\.)*)"`,
			SourceGroup: 1,
		},
	}
	cfg.EscapeType = regex.EscapeBackslash

	input := "\"Line1\\nLine2\"\n\"Tab\\there\""
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Line1\nLine2", blocks[0].SourceText())
	assert.Equal(t, "Tab\there", blocks[1].SourceText())
}

// okapi: RegexFilterTest#testEscapeDoubleChar
func TestDoubleCharEscape(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `"((?:[^"]|"")*)"`,
			SourceGroup: 1,
		},
	}
	cfg.EscapeType = regex.EscapeDoubleChar
	cfg.EscapeChar = "\""

	input := "\"Hello \"\"World\"\"\"\n\"Simple text\"\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello \"World\"", blocks[0].SourceText())
	assert.Equal(t, "Simple text", blocks[1].SourceText())
}

// okapi: RegexFilterTest#testEscapeDoubleCharNoEscape
func TestNoEscape(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `"([^"]*)"`,
			SourceGroup: 1,
		},
	}
	cfg.EscapeType = regex.EscapeNone

	input := `"Hello" "World"`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "World", blocks[1].SourceText())
}

// --- INI Format Tests ---

// okapi: RegexFilterTest#testConfigurations (INI)
func TestINIFormat(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = iniRules()

	input := "[Section1]\nkey1=Hello World\nkey2=Goodbye\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "key1", blocks[0].Name)
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "key2", blocks[1].Name)
}

// okapi: RegexFilterTest#testConfigurations (INI file)
func TestINIFile(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = iniRules()

	f, err := os.Open("testdata/simple.ini")
	require.NoError(t, err)

	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.ini", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "key1", blocks[0].Name)
	assert.Equal(t, "key2", blocks[1].Name)
	assert.Equal(t, "key3", blocks[2].Name)
}

// --- Data Parts Tests ---

// okapi: RegexFilterTest#testEmptyLines
func TestNonMatchingContentAsData(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = iniRules()

	input := "# Comment line\nkey1=Value1\n\n# Another comment\nkey2=Value2\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Should have layer start/end, blocks, and data parts
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Value1", blocks[0].SourceText())
	assert.Equal(t, "Value2", blocks[1].SourceText())

	// Verify Data parts exist (for comments and blank lines)
	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.Greater(t, dataCount, 0, "non-matching content should produce Data parts")
}

// --- Multiple Rules Tests ---

func TestMultipleRules(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			// Match key=value pairs
			Pattern:     `(?m)^(\w+)=(.+)$`,
			SourceGroup: 2,
			IDGroup:     1,
		},
		{
			// Match LABEL "text" pairs
			Pattern:     `LABEL\s+"([^"]+)"`,
			SourceGroup: 1,
		},
	}

	input := "title=Hello\nLABEL \"World\"\ndesc=Goodbye\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "title", blocks[0].Name)
	assert.Equal(t, "World", blocks[1].SourceText())
	assert.Equal(t, "Goodbye", blocks[2].SourceText())
	assert.Equal(t, "desc", blocks[2].Name)
}

// --- Symbian RLS Tests ---

// okapi: RegexFilterTest#testConfigurations (SymbianRLS)
func TestSymbianRLS(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `rls_string\s+(\w+)\s+"((?:[^"\\]|\\.)*)"`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
	cfg.EscapeType = regex.EscapeBackslash

	input := "rls_string test1 \"Hello World\"\nrls_string test2 \"\\\"Quoted\\\"\"\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "test1", blocks[0].Name)
	assert.Equal(t, "\"Quoted\"", blocks[1].SourceText())
	assert.Equal(t, "test2", blocks[1].Name)
}

// --- StringInfo Tests ---

// okapi: RegexFilterTest#testConfigurations (StringInfo)
func TestStringInfo(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			// StringInfo: ID,value,translatable
			Pattern:     `(?m)^(\w+),([^,]+),1$`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}

	input := "STR1,Hello,1\nSTR2,World,1\nSTR3,NoTranslate,0\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2, "only translatable entries should produce blocks")
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "STR1", blocks[0].Name)
	assert.Equal(t, "World", blocks[1].SourceText())
	assert.Equal(t, "STR2", blocks[1].Name)
}

// --- Roundtrip Tests ---

// okapi: RegexFilterTest#testSimpleRule (roundtrip)
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	input := "\"key1\" = \"Hello\";\n\"key2\" = \"World\";\n"

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := regex.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	// Share config with writer
	writerCfg := &regex.Config{}
	writerCfg.Reset()
	writerCfg.Rules = macStringsRules()

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Equal(t, input, output, "roundtrip should preserve content exactly")
}

// okapi: RegexFilterTest#testConfigurations (roundtrip with translation)
func TestRoundTripWithTranslation(t *testing.T) {
	ctx := t.Context()

	input := "\"key1\" = \"Hello\";\n\"key2\" = \"World\";\n"

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Hello":
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			case "World":
				block.SetTargetText(model.LocaleFrench, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	writer := regex.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.Contains(t, output, "Monde")
	assert.NotContains(t, output, "Hello")
	assert.NotContains(t, output, "World")
	assert.Equal(t, "\"key1\" = \"Bonjour\";\n\"key2\" = \"Monde\";\n", output)
}

// okapi: RegexFilterTest#testConfigurations (INI roundtrip)
func TestRoundTripINI(t *testing.T) {
	ctx := t.Context()

	input := "key1=Hello\nkey2=World\n"

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = iniRules()

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := regex.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Equal(t, input, output, "INI roundtrip should preserve content")
}

// okapi: RegexFilterTest#testBackslashEscapeHandling (roundtrip)
func TestRoundTripBackslashEscape(t *testing.T) {
	ctx := t.Context()

	input := "\"Hello \\\"World\\\"\"\n"

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			Pattern:     `"((?:[^"\\]|\\.)*)"`,
			SourceGroup: 1,
		},
	}
	cfg.EscapeType = regex.EscapeBackslash

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := regex.NewWriter()
	writerCfg := &regex.Config{}
	writerCfg.Reset()
	writerCfg.EscapeType = regex.EscapeBackslash
	_ = writer.SetConfig(writerCfg)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Equal(t, input, output, "backslash escape roundtrip should preserve content")
}

// --- Config Tests ---

func TestConfigValidation(t *testing.T) {
	cfg := &regex.Config{}

	// Empty config is valid (no rules)
	require.NoError(t, cfg.Validate())

	// Rule with empty pattern
	cfg.Rules = []regex.Rule{{Pattern: "", SourceGroup: 1}}
	require.Error(t, cfg.Validate())

	// Rule with sourceGroup < 1
	cfg.Rules = []regex.Rule{{Pattern: `\w+`, SourceGroup: 0}}
	require.Error(t, cfg.Validate())

	// Invalid escape type
	cfg.Rules = nil
	cfg.EscapeType = "invalid"
	require.Error(t, cfg.Validate())

	// Valid config
	cfg.Rules = []regex.Rule{{Pattern: `"([^"]*)"`, SourceGroup: 1}}
	cfg.EscapeType = regex.EscapeBackslash
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMap(t *testing.T) {
	cfg := &regex.Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"escapeType": "backslash",
		"escapeChar": "'",
		"rules": []any{
			map[string]any{
				"pattern":     `"([^"]*)"`,
				"sourceGroup": 1,
				"idGroup":     0,
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "backslash", cfg.EscapeType)
	assert.Equal(t, "'", cfg.EscapeChar)
	require.Len(t, cfg.Rules, 1)
	assert.Equal(t, `"([^"]*)"`, cfg.Rules[0].Pattern)
	assert.Equal(t, 1, cfg.Rules[0].SourceGroup)
}

func TestConfigApplyMapUnknownKey(t *testing.T) {
	cfg := &regex.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
}

func TestConfigReset(t *testing.T) {
	cfg := &regex.Config{
		Rules:      []regex.Rule{{Pattern: "test"}},
		EscapeType: "backslash",
		EscapeChar: "'",
	}
	cfg.Reset()
	assert.Nil(t, cfg.Rules)
	assert.Equal(t, regex.EscapeNone, cfg.EscapeType)
	assert.Equal(t, "\"", cfg.EscapeChar)
}

// --- Prefix/Suffix Assembly Tests (issue #605) ---

// roundtripWithTranslations reads the input, applies the given target
// translations keyed by source text, and writes the output. When
// useSkeleton is true the skeleton (byte-exact) write path is exercised;
// otherwise the streaming (non-skeleton) path is used.
func roundtripWithTranslations(t *testing.T, input string, rules []regex.Rule, escType, escChar string, locale model.LocaleID, translations map[string]string, useSkeleton bool) string {
	t.Helper()
	ctx := t.Context()

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = rules
	if escType != "" {
		cfg.EscapeType = escType
	}
	if escChar != "" {
		cfg.EscapeChar = escChar
	}

	writer := regex.NewWriter()
	writerCfg := &regex.Config{}
	writerCfg.Reset()
	writerCfg.Rules = rules
	if escType != "" {
		writerCfg.EscapeType = escType
	}
	if escChar != "" {
		writerCfg.EscapeChar = escChar
	}
	require.NoError(t, writer.SetConfig(writerCfg))

	var store *format.SkeletonStore
	if useSkeleton {
		var err error
		store, err = format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
	}

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block := p.Resource.(*model.Block)
		if tgt, ok := translations[block.SourceText()]; ok {
			block.SetTargetText(locale, tgt)
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(locale)

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// prefixSuffixRules captures group 2 of ^prefix(.*?)suffix$, leaving a
// non-empty prefix ("prefix") and suffix ("suffix") around the capture.
func prefixSuffixRules() []regex.Rule {
	return []regex.Rule{
		{
			Pattern:     `(?m)^(prefix)(.*?)(suffix)$`,
			SourceGroup: 2,
		},
	}
}

// TestPrefixSuffixStored asserts the reader records the raw text before and
// after the translatable capture as block properties.
func TestPrefixSuffixStored(t *testing.T) {
	ctx := t.Context()
	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = prefixSuffixRules()

	input := "prefixHELLOsuffix"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "HELLO", blocks[0].SourceText())
	assert.Equal(t, "prefix", blocks[0].Properties["regex.prefix"])
	assert.Equal(t, "suffix", blocks[0].Properties["regex.suffix"])
	// Old reconstruction key is gone; assembly is purely prefix+value+suffix.
	assert.NotContains(t, blocks[0].Properties, "regex.fullMatch")
}

// TestPrefixSuffixRoundTripStreaming round-trips with a changed translation on
// the non-skeleton path, asserting prefix+suffix are preserved exactly and the
// translation is spliced between them.
func TestPrefixSuffixRoundTripStreaming(t *testing.T) {
	input := "prefixHELLOsuffix"
	output := roundtripWithTranslations(t, input, prefixSuffixRules(), "", "",
		model.LocaleFrench, map[string]string{"HELLO": "BONJOUR"}, false)
	assert.Equal(t, "prefixBONJOURsuffix", output)
}

// TestPrefixSuffixRoundTripSkeleton round-trips the same case via the
// byte-exact skeleton write path.
func TestPrefixSuffixRoundTripSkeleton(t *testing.T) {
	input := "prefixHELLOsuffix"
	output := roundtripWithTranslations(t, input, prefixSuffixRules(), "", "",
		model.LocaleFrench, map[string]string{"HELLO": "BONJOUR"}, true)
	assert.Equal(t, "prefixBONJOURsuffix", output)
}

// TestPrefixSuffixWithSurroundingData covers a non-empty prefix/suffix where the
// match is embedded among non-matching content (Data parts), confirming
// surrounding bytes and delimiters survive the assembly untouched.
func TestPrefixSuffixWithSurroundingData(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `(?m)^msg\[(\w+)\] = <<(.*?)>>;$`,
			SourceGroup: 2,
			IDGroup:     1,
		},
	}
	input := "# header\nmsg[greet] = <<Hello>>;\n# footer\nmsg[bye] = <<Goodbye>>;\n"

	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, "", "", model.LocaleFrench,
			map[string]string{"Hello": "Bonjour", "Goodbye": "Au revoir"}, skel)
		expected := "# header\nmsg[greet] = <<Bonjour>>;\n# footer\nmsg[bye] = <<Au revoir>>;\n"
		assert.Equal(t, expected, out, "skeleton=%v", skel)
	}
}

// TestPrefixSuffixNoTranslationRoundTrip confirms that without any translation
// the assembly reproduces the source byte-for-byte (both paths).
func TestPrefixSuffixNoTranslationRoundTrip(t *testing.T) {
	input := "prefixHELLOsuffix"
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, prefixSuffixRules(), "", "",
			model.LocaleFrench, nil, skel)
		assert.Equal(t, input, out, "skeleton=%v: source must round-trip exactly", skel)
	}
}

// TestEmptyPrefixSuffix covers a capture that spans the entire match, leaving
// both prefix and suffix empty. The translation replaces the whole match.
func TestEmptyPrefixSuffix(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `(?m)^(.+)$`,
			SourceGroup: 1,
		},
	}
	input := "Hello\n"
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, "", "", model.LocaleFrench,
			map[string]string{"Hello": "Bonjour"}, skel)
		assert.Equal(t, "Bonjour\n", out, "skeleton=%v", skel)
	}
}

// TestPrefixOnly covers a non-empty prefix with an empty suffix (capture at the
// end of the match).
func TestPrefixOnly(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `(?m)^key: (.+)$`,
			SourceGroup: 1,
		},
	}
	input := "key: Hello\n"
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, "", "", model.LocaleFrench,
			map[string]string{"Hello": "Bonjour"}, skel)
		assert.Equal(t, "key: Bonjour\n", out, "skeleton=%v", skel)
	}
}

// TestMultipleCaptureGroupsAssembly confirms prefix/suffix are computed from the
// correct (source) capture group when several groups participate in the match,
// so ID/note groups land in the prefix/suffix and survive verbatim.
func TestMultipleCaptureGroupsAssembly(t *testing.T) {
	rules := macStringsWithNotesRules() // groups: 1=note, 2=id, 3=source
	input := "/* Menu item */\n\"File\" = \"File\";\n"
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, "", "", model.LocaleFrench,
			map[string]string{"File": "Fichier"}, skel)
		// Note comment and the id ("File" key) are part of prefix/suffix and
		// must be untouched; only the source value (group 3) is translated.
		assert.Equal(t, "/* Menu item */\n\"File\" = \"Fichier\";\n", out, "skeleton=%v", skel)
	}
}

// --- Escape Interaction Tests (issue #605) ---

// TestDoubleCharEscapeRoundTripWithTranslation confirms doublechar escaping is
// applied only to the value, not to the surrounding prefix/suffix, and that
// re-escaping a translated value containing the escape char does not double up
// the surrounding delimiters.
func TestDoubleCharEscapeRoundTripWithTranslation(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `"((?:[^"]|"")*)"`,
			SourceGroup: 1,
		},
	}
	// Source value contains an escaped (doubled) quote: "" decodes to ".
	input := "\"Say \"\"Hi\"\"\""
	// Translate to a value that also contains a quote; the writer must
	// re-escape only the value (" -> "") without disturbing the outer
	// delimiter quotes that live in prefix/suffix.
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, regex.EscapeDoubleChar, "\"",
			model.LocaleFrench, map[string]string{`Say "Hi"`: `Dis "Salut"`}, skel)
		assert.Equal(t, "\"Dis \"\"Salut\"\"\"", out, "skeleton=%v", skel)
	}
}

// TestDoubleCharEscapeNoTranslationNoDoubleEscape is the regression guard: a
// source-only round-trip with doublechar escaping must reproduce the input
// byte-for-byte (no double-escaping of the value, no escaping of delimiters).
func TestDoubleCharEscapeNoTranslationNoDoubleEscape(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `"((?:[^"]|"")*)"`,
			SourceGroup: 1,
		},
	}
	input := "\"Hello \"\"World\"\"\""
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, regex.EscapeDoubleChar, "\"",
			model.LocaleFrench, nil, skel)
		assert.Equal(t, input, out, "skeleton=%v: doublechar source must round-trip exactly", skel)
	}
}

// TestBackslashEscapeRoundTripWithTranslation confirms backslash escaping of the
// translated value works through pure assembly, with prefix/suffix delimiters
// preserved.
func TestBackslashEscapeRoundTripWithTranslation(t *testing.T) {
	rules := []regex.Rule{
		{
			Pattern:     `"((?:[^"\\]|\\.)*)"`,
			SourceGroup: 1,
		},
	}
	input := "\"Hello \\\"World\\\"\"\n"
	for _, skel := range []bool{false, true} {
		out := roundtripWithTranslations(t, input, rules, regex.EscapeBackslash, "",
			model.LocaleFrench, map[string]string{`Hello "World"`: `Salut "Monde"`}, skel)
		assert.Equal(t, "\"Salut \\\"Monde\\\"\"\n", out, "skeleton=%v", skel)
	}
}

// --- Context Cancellation Test ---

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	reader := regex.NewReader()
	cfg := reader.Config().(*regex.Config)
	cfg.Rules = macStringsRules()

	input := "\"key1\" = \"Hello\";\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	// Should not hang; channel will be closed
	for range ch {
		// drain
	}
}

// roundTripRules reads input with the given rules then writes it back out
// (no translation) and returns the reconstructed document. With no targets,
// each block's source is re-emitted via prefix+source+suffix assembly, so a
// faithful reader/writer must reproduce the input byte-for-byte.
func roundTripRules(t *testing.T, input string, rules []regex.Rule) string {
	t.Helper()
	ctx := t.Context()

	reader := regex.NewReader()
	reader.Config().(*regex.Config).Rules = rules
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := regex.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleEnglish)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// okapi: RegexFilterTest#testDoubleExtraction
// Okapi's testDoubleExtraction runs a RoundTripComparison over a corpus of
// fixtures driven by distinct .fprm rule sets (SRT, StringInfo, INI, and the
// TestRules01..06 custom configs), asserting each file roundtrips identically.
// The native analog is the same observable contract: for each representative
// rule configuration, read → write with no translation must reproduce the
// source byte-for-byte. We exercise several rule shapes the corpus covers —
// content-with-note (SRT-like), comma-delimited StringInfo, key=value INI, and
// quoted Mac .strings — rather than re-importing Okapi's .fprm files.
func TestDoubleExtractionRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		rules []regex.Rule
	}{
		{
			name:  "macStrings quoted key=value",
			input: "\"key1\" = \"Hello\";\n\"key2\" = \"World\";\n",
			rules: macStringsRules(),
		},
		{
			name:  "ini key=value",
			input: "key1=Hello\nkey2=World\n",
			rules: iniRules(),
		},
		{
			name:  "stringinfo comma-delimited translatable flag",
			input: "STR1,Hello,1\nSTR2,World,1\nSTR3,NoTranslate,0\n",
			rules: []regex.Rule{{Pattern: `(?m)^(\w+),([^,]+),1$`, SourceGroup: 2, IDGroup: 1}},
		},
		{
			name: "srt content rule with note group",
			input: "1\n00:00:12,000 --> 00:00:15,123\nThis is the first subtitle\n\n" +
				"2\n00:00:20,000 --> 00:00:22,000\nAnother subtitle.\n\n",
			rules: []regex.Rule{{
				Pattern:     `(?s)(\d\d:\d\d:\d\d.*?)\n(.*?)(\n\n+|\z)`,
				SourceGroup: 2,
				NoteGroup:   1,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := roundTripRules(t, tt.input, tt.rules)
			assert.Equal(t, tt.input, got, "no-translation roundtrip must be byte-exact")
		})
	}
}

// The remaining RegexFilterTest methods exercise Okapi-only Rule features that
// the native regex format does not model (#611). The native regex.Rule
// supports only Pattern + SourceGroup/IDGroup/NoteGroup; it has no
// collapseNewline option, no MetaRule/GenericAnnotations, and no subfilter
// recursion. These are deliberate-design gaps in the native reader, not bugs,
// so they are skip-classified rather than fake-passed.
//
// okapi-skip: RegexFilterTest#testCollapseNewline — native regex.Rule has no collapseNewline option to fold internal newlines in the captured source to a space
// okapi-skip: RegexFilterTest#testMeta — native regex.Rule has no MetaRule/GenericAnnotations support; only NoteGroup notes are extracted
// okapi-skip: RegexFilterTest#testSubFiltering — native regex format does not support subfilter recursion (re-parsing a captured group with another filter)
// okapi-skip: RegexFilterTest#testNoteWithSubfilter — native regex format has no subFilter; cannot re-parse the source group through okf_regex@TestRules02
