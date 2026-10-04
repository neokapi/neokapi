package kbf

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// v1Bundle is a bundle in schema 1: source runs beside targets keyed by
// locale, a provenance record for some of them, a target under the empty
// locale, and an origin for a locale that has no target.
const v1Bundle = `{
  "schemaVersion": "1.0",
  "kind": "kapi-bundle",
  "generator": {"id": "test", "version": "0"},
  "project": {"id": "p", "sourceLocale": "en"},
  "documents": [{
    "id": "d1",
    "documentType": "jsx",
    "path": "app/Greeting.tsx",
    "blocks": [{
      "id": "d1:b1",
      "hash": "h1",
      "translatable": true,
      "type": "jsx:element",
      "source": [{"text": "Sign in"}],
      "targets": {
        "nb": [{"text": "Logg inn"}],
        "de": [{"text": "Anmelden"}],
        "": [{"text": "EMPTYLOC"}]
      },
      "targetOrigins": {
        "nb": {"kind": "ai", "engine": "claude", "context_fingerprint": "cfp-1"},
        "": {"kind": "human"},
        "fr": {"kind": "mt"}
      },
      "placeholders": [],
      "properties": {}
    }]
  }]
}`

func TestUnmarshalReadsASchema1FileAsEditions(t *testing.T) {
	f, err := Unmarshal([]byte(v1Bundle))
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, f.SchemaVersion, "the file holds the current shape once read")

	require.Len(t, f.Documents, 1)
	require.Len(t, f.Documents[0].Blocks, 1)
	b := f.Documents[0].Blocks[0]

	assert.Equal(t, "Sign in", model.RunsText(b.SourceRuns()))
	assert.Equal(t, []string{"de", "nb"}, b.TargetKeys(), "an origin with no target describes nothing")

	nb, ok := b.Edition("nb")
	require.True(t, ok)
	assert.Equal(t, "Logg inn", model.RunsText(nb.Runs))
	assert.Equal(t, Origin{Kind: "ai", Engine: "claude", ContextFingerprint: "cfp-1"}, nb.Origin)

	de, ok := b.Edition("de")
	require.True(t, ok)
	assert.Equal(t, Origin{}, de.Origin)

	require.NotNil(t, b.Unlabelled, "a target under the empty locale is the unlabelled edition")
	assert.Equal(t, "EMPTYLOC", model.RunsText(b.Unlabelled.Runs))
	assert.Equal(t, "human", b.Unlabelled.Origin.Kind)
}

func TestDecodeReadsASchema1FileAsEditions(t *testing.T) {
	f, err := Decode(strings.NewReader(v1Bundle))
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, f.SchemaVersion)
	b := f.Documents[0].Blocks[0]
	assert.Equal(t, "Sign in", model.RunsText(b.SourceRuns()))
	assert.Equal(t, []string{"de", "nb"}, b.TargetKeys())
}

// A schema 1 file written again is written in the current schema, and a
// second read of that finds the same content.
func TestASchema1FileIsWrittenInTheCurrentSchema(t *testing.T) {
	f, err := Unmarshal([]byte(v1Bundle))
	require.NoError(t, err)
	out, err := Marshal(f)
	require.NoError(t, err)

	text := string(out)
	assert.Contains(t, text, `"schemaVersion": "2.0"`)
	assert.Contains(t, text, `"editions": {`)
	assert.Contains(t, text, `"unlabelled": {`)
	for _, gone := range []string{`"source":`, `"targets":`, `"targetOrigins":`} {
		assert.NotContains(t, text, gone)
	}

	again, err := Unmarshal(out)
	require.NoError(t, err)
	assert.Equal(t, f, again)
}

// The block cache stores each block as JSON with no envelope, so a block
// written in schema 1 has to read on its own.
func TestABlockInTheSchema1ShapeReadsWithNoEnvelope(t *testing.T) {
	var b Block
	require.NoError(t, json.Unmarshal([]byte(`{"id":"b","hash":"h","translatable":true,"type":"jsx:element","source":[{"text":"Hi"}],"targets":{"fr":[{"text":"Salut"}]},"placeholders":[],"properties":{}}`), &b))
	assert.Equal(t, "Hi", model.RunsText(b.SourceRuns()))
	fr, ok := b.Edition("fr")
	require.True(t, ok)
	assert.Equal(t, "Salut", model.RunsText(fr.Runs))

	var empty Block
	require.NoError(t, json.Unmarshal([]byte(`{"id":"b","source":null}`), &empty))
	_, held := empty.Edition(SourceEdition)
	assert.True(t, held, "a schema 1 block always holds its source")
}

func TestABlockCarryingBothShapesIsRefused(t *testing.T) {
	var b Block
	err := json.Unmarshal([]byte(`{"id":"b","editions":{"":{"runs":[{"text":"Hi"}]}},"source":[{"text":"Hi"}]}`), &b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both editions and the schema 1 source")
}

func TestUnmarshalTakesAMinorOfAKnownMajorAndRefusesAnUnknownOne(t *testing.T) {
	envelope := func(v string) []byte {
		return []byte(`{"schemaVersion":"` + v + `","kind":"kapi-bundle","generator":{"id":"x","version":"1"},"project":{"id":"p","sourceLocale":"en"},"documents":[]}`)
	}
	for _, v := range []string{"1.0", "1.4", "2.0", "2.7"} {
		_, err := Unmarshal(envelope(v))
		require.NoError(t, err, v)
	}
	_, err := Unmarshal(envelope("3.0"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported major schemaVersion 3")

	f, err := Unmarshal(envelope("2.7"))
	require.NoError(t, err)
	assert.Equal(t, "2.7", f.SchemaVersion, "a minor of the current major is kept as read")
}

// The blocks Marshal writes are in the current schema, so a file stamped with
// another major, or with no valid version, is written as the current one.
func TestMarshalStampsTheVersionOfTheShapeItWrites(t *testing.T) {
	for in, want := range map[string]string{
		"":     SchemaVersion,
		"1.0":  SchemaVersion,
		"1.4":  SchemaVersion,
		"two":  SchemaVersion,
		"2.0":  "2.0",
		"2.7":  "2.7",
		"3.0":  SchemaVersion,
		"2.0x": SchemaVersion,
	} {
		f := &File{SchemaVersion: in, Kind: Kind, Documents: []Document{}}
		out, err := Marshal(f)
		require.NoError(t, err, in)
		assert.Contains(t, string(out), `"schemaVersion": "`+want+`"`, in)
	}
}

// peerBlock holds an edition of every kind a block can carry.
func peerBlock() *Block {
	return &Block{
		ID:           "b",
		Hash:         "h",
		Translatable: true,
		Type:         BlockTypeJSXElement,
		Editions: map[string]Edition{
			SourceEdition: {Runs: []Run{{Text: &TextRun{Text: "Sign in to <b>save</b>"}}}, Status: "established"},
			"nb": {
				Runs:   []Run{{Text: &TextRun{Text: "Logg inn"}}},
				Status: "translated",
				Origin: Origin{Kind: "ai", Engine: "claude"},
				Score:  0.75,
				Derived: &Derivation{
					From: SourceEdition,
					Rev:  "r:0123456789abcdef",
				},
			},
			"en;channel=short": {Runs: []Run{{Text: &TextRun{Text: "Sign in"}}}, Derived: &Derivation{From: SourceEdition, Rev: "r:0123456789abcdef"}},
			"fr;tone=formal":   {Runs: []Run{{Text: &TextRun{Text: "Connectez-vous"}}}},
		},
		Unlabelled: &Edition{Runs: []Run{{Text: &TextRun{Text: "EMPTYLOC"}}}},
		Properties: BlockProperties{File: "a.tsx"},
	}
}

func TestEditionsRoundTripByteForByte(t *testing.T) {
	first, err := MarshalBlock(peerBlock())
	require.NoError(t, err)

	var decoded Block
	require.NoError(t, json.Unmarshal(first, &decoded))
	assert.Equal(t, []string{"en;channel=short", "fr;tone=formal", "nb"}, decoded.TargetKeys())

	second, err := MarshalBlock(&decoded)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))

	text := string(first)
	assert.Contains(t, text, `<b>save</b>`, "no HTML escaping inside an edition")
	// Keys in byte order, the source first; fields in declaration order.
	assert.Less(t, strings.Index(text, `"": {`), strings.Index(text, `"en;channel=short": {`))
	assert.Less(t, strings.Index(text, `"en;channel=short": {`), strings.Index(text, `"fr;tone=formal": {`))
	at := strings.Index(text, `"nb": {`)
	require.GreaterOrEqual(t, at, 0)
	nb := text[at:]
	assert.Less(t, strings.Index(nb, `"runs"`), strings.Index(nb, `"status"`))
	assert.Less(t, strings.Index(nb, `"status"`), strings.Index(nb, `"origin"`))
	assert.Less(t, strings.Index(nb, `"origin"`), strings.Index(nb, `"score"`))
	assert.Less(t, strings.Index(nb, `"score"`), strings.Index(nb, `"derived"`))
}

// A block always carries its source edition and a placeholder list, even when
// the writer filled neither, so a reader in any language finds both.
func TestMarshalWritesTheSourceEditionAndThePlaceholderListAlways(t *testing.T) {
	out, err := MarshalBlock(&Block{ID: "b", Type: BlockTypeJSXElement})
	require.NoError(t, err)
	text := string(out)
	assert.Contains(t, text, `"editions": {
    "": {
      "runs": []
    }
  }`)
	assert.Contains(t, text, `"placeholders": []`)
	assert.NotContains(t, text, `"unlabelled"`)

	// An edition that records nothing beside its runs writes only its runs.
	out, err = MarshalBlock(&Block{ID: "b", Editions: map[string]Edition{"": {Runs: []Run{{Text: &TextRun{Text: "x"}}}}, "de": {}}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"de": {
      "runs": []
    }`)
}

func TestMarshalOfAJSONMarshalledBlockEscapesHTMLAsTheCallerAsked(t *testing.T) {
	// json.Marshal escapes HTML. The block's own marshaler must not undo the
	// caller's choice either way: compact JSON from json.Marshal escapes, the
	// bundle encoder does not.
	b := &Block{ID: "b", Editions: SourceEditions([]Run{{Text: &TextRun{Text: "<b>"}}})}
	escaped, err := json.Marshal(b)
	require.NoError(t, err)
	assert.Contains(t, string(escaped), "\\u003cb\\u003e")

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(b))
	assert.Contains(t, buf.String(), `"<b>"`)
}

// The bundle carries every edition the model holds, the tone and channel
// editions included, with its status, origin, score and derivation, and a
// read files each one back where it was.
func TestEditionsCarryEveryEditionOfTheModel(t *testing.T) {
	mb := model.NewRunsBlock("b", []model.Run{model.TextR("Sign in")})
	mb.SourceLocale = "en"
	mb.SetEditionStatus(model.EditionKey{}, model.Status("established"))
	mb.SetSourceOrigin(&model.Origin{Kind: model.OriginOCR, Confidence: 0.5})
	nb := model.Edition{
		Runs:    []model.Run{model.TextR("Logg inn")},
		Status:  model.Status(model.TargetStatusTranslated),
		Origin:  model.Origin{Kind: model.OriginAI, Engine: "claude"},
		Score:   0.9,
		Derived: &model.Derivation{From: model.EditionKey{}, Rev: "r:00"},
	}
	mb.SetEdition(model.Variant("nb"), nb)
	short := model.EditionKey{Locale: "en", Channel: "short"}
	mb.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("Sign")}, Derived: &model.Derivation{From: model.Variant("en"), Rev: "r:01"}})
	formal := model.EditionKey{Locale: "fr", Tone: "formal"}
	mb.SetEdition(formal, model.Edition{Runs: []model.Run{model.TextR("Connectez-vous")}})
	mb.SetTargetRuns("", []model.Run{model.TextR("EMPTYLOC")})
	mb.SetEdition(model.Variant("de"), model.Edition{Status: model.Status(model.TargetStatusDraft)})

	editions, unlabelled := EditionsOf(mb)
	assert.ElementsMatch(t, []string{"", "nb", "en;channel=short", "fr;tone=formal"}, keysOf(editions),
		"an edition with no runs is left out")
	require.NotNil(t, unlabelled)
	assert.Equal(t, "EMPTYLOC", model.RunsText(unlabelled.Runs))
	assert.Equal(t, "established", editions[SourceEdition].Status)
	assert.Equal(t, model.OriginOCR, editions[SourceEdition].Origin.Kind)
	assert.Equal(t, &Derivation{From: "en", Rev: "r:01"}, editions["en;channel=short"].Derived)

	// Through the wire and back onto a fresh block.
	kb := Block{ID: "b", Editions: editions, Unlabelled: unlabelled}
	data, err := MarshalBlock(&kb)
	require.NoError(t, err)
	var read Block
	require.NoError(t, json.Unmarshal(data, &read))

	back := model.NewRunsBlock("b", read.SourceRuns())
	back.SourceLocale = "en"
	read.FileEditions(back)

	src, _ := back.Edition(model.EditionKey{})
	assert.Equal(t, model.Status("established"), src.Status)
	assert.Equal(t, model.OriginOCR, src.Origin.Kind)
	gotNB, ok := back.Edition(model.Variant("nb"))
	require.True(t, ok)
	assert.Equal(t, nb, gotNB)
	gotShort, ok := back.Edition(short)
	require.True(t, ok)
	assert.Equal(t, "Sign", model.RunsText(gotShort.Runs))
	assert.Equal(t, &model.Derivation{From: model.Variant("en"), Rev: "r:01"}, gotShort.Derived)
	gotFormal, ok := back.Edition(formal)
	require.True(t, ok)
	assert.Equal(t, "Connectez-vous", model.RunsText(gotFormal.Runs))
	assert.Equal(t, "EMPTYLOC", back.TargetText(""))
	assert.Equal(t, "Sign in", back.SourceText())
	assert.ElementsMatch(t,
		[]model.EditionKey{model.Variant("en"), short, formal, model.Variant("nb")}, back.NativeEditions(),
		"every edition a bundle holds is native to it")
}

// Two keys that spell one locale two ways settle on the same edition on every
// read: the one whose spelling sorts last.
func TestFileEditionsSettlesTwoSpellingsOfOneLocaleTheSameWay(t *testing.T) {
	kb := Block{ID: "b", Editions: map[string]Edition{
		SourceEdition: {Runs: []Run{{Text: &TextRun{Text: "Hi"}}}},
		"nb-NO":       {Runs: []Run{{Text: &TextRun{Text: "dash"}}}},
		"nb_NO":       {Runs: []Run{{Text: &TextRun{Text: "underscore"}}}},
	}}
	for range 20 {
		mb := model.NewRunsBlock("b", kb.SourceRuns())
		kb.FileEditions(mb)
		assert.Equal(t, "underscore", mb.TargetText("nb-NO"))
	}
}

func keysOf(m map[string]Edition) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
