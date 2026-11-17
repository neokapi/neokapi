package yaml_test

import (
	"testing"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
)

// readBlocks reads input, with cfg applied when non-nil, and returns its blocks.
func readBlocks(t *testing.T, input string, cfg map[string]any) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	if cfg != nil {
		require.NoError(t, reader.Cfg.ApplyMap(cfg))
	}
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	var blocks []*model.Block
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// TestTranslatedWrite_KeepsAnchorsAndTags writes a translated document whose
// scalars carry anchors and tags, then reads the output back. The anchor on a
// translated scalar stays in place, so the alias that refers to it still
// resolves, and the alias position keeps writing `*name`.
func TestTranslatedWrite_KeepsAnchorsAndTags(t *testing.T) {
	t.Parallel()
	input := "defaults: &defaults\n" +
		"  greeting: &hello Hello there\n" +
		"  quoted: &q \"Quoted value\"\n" +
		"  tagged: !!str &t Tagged first\n" +
		"  anchored: &a !!str Anchored first\n" +
		"  block: &lit |\n" +
		"    Literal text\n" +
		"copy:\n" +
		"  <<: *defaults\n" +
		"  extra: *hello\n" +
		"  again: *q\n" +
		"  more: *lit\n"

	out := translateRoundtripWithSkeleton(t, input)

	assert.Contains(t, out, "greeting: &hello XHello there\n")
	assert.Contains(t, out, "quoted: &q \"XQuoted value\"\n")
	assert.Contains(t, out, "tagged: !!str &t XTagged first\n")
	assert.Contains(t, out, "anchored: &a !!str XAnchored first\n")
	assert.Contains(t, out, "block: &lit |\n")
	assert.Contains(t, out, "extra: *hello\n")
	assert.Contains(t, out, "again: *q\n")
	assert.Contains(t, out, "more: *lit\n")

	// The output is valid YAML and the aliases resolve to the translated
	// anchor values.
	var doc map[string]any
	require.NoError(t, yamlv3.Unmarshal([]byte(out), &doc))
	cp := doc["copy"].(map[string]any)
	assert.Equal(t, "XHello there", cp["extra"])
	assert.Equal(t, "XQuoted value", cp["again"])

	// kapi's own reader accepts the output.
	blocks := readBlocks(t, out, nil)
	require.NotEmpty(t, blocks)
}

// TestRead_YAML11BooleansAreNotText checks that plain yes/no/on/off, which
// YAML 1.1 parsers such as Ruby's Psych read as booleans, follow
// extract_non_strings like any other non-string scalar, while a quoted or
// `!!str`-tagged spelling stays a string.
func TestRead_YAML11BooleansAreNotText(t *testing.T) {
	t.Parallel()
	input := "enabled: yes\ndisabled: off\nOn_key: ON\nanswer: 'yes'\nforced: !!str no\nletter: y\ntitle: Welcome\n"

	texts := func(bs []*model.Block) []string {
		var out []string
		for _, b := range bs {
			out = append(out, b.SourceText())
		}
		return out
	}
	assert.Equal(t, []string{"yes", "no", "y", "Welcome"}, texts(readBlocks(t, input, nil)))
	assert.Equal(t, []string{"yes", "off", "ON", "yes", "no", "y", "Welcome"},
		texts(readBlocks(t, input, map[string]any{"extract_non_strings": true})))
}

// TestRead_BareURLIsNotTranslatable checks that a value made of one absolute
// URL is extracted but marked non-translatable, that translate_urls turns
// that off, and that a translated write keeps the URL as written.
func TestRead_BareURLIsNotTranslatable(t *testing.T) {
	t.Parallel()
	input := "url: https://example.com/path\nquoted: \"mailto://a@b.c\"\nlink: See https://example.com for more\n"

	blocks := readBlocks(t, input, nil)
	require.Len(t, blocks, 3)
	assert.False(t, blocks[0].Translatable)
	assert.False(t, blocks[1].Translatable)
	assert.True(t, blocks[2].Translatable)

	blocks = readBlocks(t, input, map[string]any{"translate_urls": true})
	require.Len(t, blocks, 3)
	assert.True(t, blocks[0].Translatable)

	out := translateRoundtripWithSkeleton(t, input)
	assert.Contains(t, out, "url: https://example.com/path\n")
	assert.Contains(t, out, "link: XSee https://example.com for more\n")
}
