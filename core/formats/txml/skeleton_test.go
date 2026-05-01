package txml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleTranslatable(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<skeleton>&lt;html&gt;&lt;p&gt;</skeleton>
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello World</source><target>Bonjour le monde</target></segment></translatable>
<skeleton>&lt;/p&gt;&lt;/html&gt;</skeleton>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SourceOnly(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Source only text</source></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "source-only TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_TwoSegments(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment><segment segmentId="s2"><source>Goodbye</source><target>Au revoir</target></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "two-segment TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleTranslatables(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>First block</source><target>Premier bloc</target></segment></translatable>
<translatable blockId="b2" datatype="html"><segment segmentId="s1"><source>Second block</source><target>Deuxieme bloc</target></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-translatable TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment></translatable>
</txml>
`
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify the French translation.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.Targets[model.LocaleID("fr")] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Salut"}}}}}
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Salut</target></segment></translatable>
</txml>
`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment></translatable>
</txml>
`
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set a translation with XML special characters.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.Targets[model.LocaleID("fr")] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "A & B < C"}}}}}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), "<target>A &amp; B &lt; C</target>")
}

func TestSkeletonStore_ByteExact_XmlEntities(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>A &amp; B &lt; C</source></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TXML with XML entities should be byte-exact")
}
