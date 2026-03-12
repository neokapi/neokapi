package txml_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/formats/txml"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

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

func TestSkeletonStore_ByteExact_SimpleTXML(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello World</source>
<target>Bonjour le monde</target>
</segment>
</body>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SourceOnly(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Source only text</source>
</segment>
</body>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "source-only TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleSegments(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello World</source>
<target>Bonjour le monde</target>
</segment>
<segment segtype="sentence">
<source>Goodbye</source>
<target>Au revoir</target>
</segment>
</body>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-segment TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello</source>
<target>Bonjour</target>
</segment>
</body>
</txml>
`
	ctx := context.Background()

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

	// Modify the French translation
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.Targets[model.LocaleID("fr-FR")] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Salut")}}
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello</source>
<target>Salut</target>
</segment>
</body>
</txml>
`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello</source>
<target>Bonjour</target>
</segment>
</body>
</txml>
`
	ctx := context.Background()

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

	// Set a translation with XML special characters
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.Targets[model.LocaleID("fr-FR")] = []*model.Segment{{ID: "s1", Content: model.NewFragment("A & B < C")}}
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
	input := `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>A &amp; B &lt; C</source>
</segment>
</body>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TXML with XML entities should be byte-exact")
}
