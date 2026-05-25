package ttx_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := ttx.NewReader()
	writer := ttx.NewWriter()

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

// okapi: RoundTripTtxIT#ttxFiles
func TestSkeletonStore_ByteExact_SimpleTTX(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Hello world</Tuv>
<Tuv Lang="FR-FR">Bonjour le monde</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TTX roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SourceOnly(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Source only text</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "source-only TTX roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleTUs(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Hello world</Tuv>
<Tuv Lang="FR-FR">Bonjour le monde</Tuv>
</Tu>
<Tu MatchPercent="100">
<Tuv Lang="EN-US">Goodbye</Tuv>
<Tuv Lang="FR-FR">Au revoir</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-TU TTX roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Hello</Tuv>
<Tuv Lang="FR-FR">Bonjour</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	ctx := t.Context()

	reader := ttx.NewReader()
	writer := ttx.NewWriter()

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
				b.SetTargetText(model.LocaleID("FR-FR"), "Salut")
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Hello</Tuv>
<Tuv Lang="FR-FR">Salut</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">Hello</Tuv>
<Tuv Lang="FR-FR">Bonjour</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	ctx := t.Context()

	reader := ttx.NewReader()
	writer := ttx.NewWriter()

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
			b.SetTargetText(model.LocaleID("FR-FR"), "A & B < C")
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), "A &amp; B &lt; C")
}

func TestSkeletonStore_ByteExact_XmlEntities(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0">
<Body>
<Raw>
<Tu MatchPercent="0">
<Tuv Lang="EN-US">A &amp; B &lt; C</Tuv>
</Tu>
</Raw>
</Body>
</TRADOStag>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TTX with XML entities should be byte-exact")
}
