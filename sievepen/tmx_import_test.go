package sievepen_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func importString(t *testing.T, tm sievepen.TMStore, body string, opts sievepen.ImportTMXOptions) (string, int) {
	t.Helper()
	sid, n, err := sievepen.ImportTMXSession(tm, strings.NewReader(body), opts)
	require.NoError(t, err)
	return sid, n
}

func TestImportTMX_PlainBilingual(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" segtype="sentence" adminlang="en-US" srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="t1">
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour</seg></tuv>
    </tu>
  </body>
</tmx>`
	sid, n := importString(t, tm, body, sievepen.ImportTMXOptions{OriginKey: "f.tmx"})
	assert.NotEmpty(t, sid)
	assert.Equal(t, 1, n)

	entries := tm.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "Hello", entries[0].VariantText("en"))
	assert.Equal(t, "Bonjour", entries[0].VariantText("fr"))
	require.Len(t, entries[0].Origins, 1)
	assert.Equal(t, sid, entries[0].Origins[0].SessionID)
}

func TestImportTMX_Multilingual(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="tm3" creationtoolversion="9.38" segtype="sentence" adminlang="en-GB" srclang="en-GB" datatype="plaintext"/>
  <body>
    <tu>
      <tuv xml:lang="en-GB"><seg>Hello</seg></tuv>
      <tuv xml:lang="fr-FR"><seg>Bonjour</seg></tuv>
      <tuv xml:lang="de-DE"><seg>Hallo</seg></tuv>
      <tuv xml:lang="es-ES"><seg>Hola</seg></tuv>
      <tuv xml:lang="it-IT"><seg>Ciao</seg></tuv>
    </tu>
  </body>
</tmx>`
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{OriginKey: "eurlex.tmx"})
	assert.Equal(t, 1, n)
	entries := tm.Entries()
	require.Len(t, entries, 1)
	assert.Len(t, entries[0].Variants, 5)
}

func TestImportTMX_ElementPH(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body>
<tu><tuv xml:lang="en"><seg>Click <ph x="1">{0}</ph> to continue</seg></tuv><tuv xml:lang="fr"><seg>Cliquer <ph x="1">{0}</ph> pour continuer</seg></tuv></tu>
</body></tmx>`
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{})
	assert.Equal(t, 1, n)
	e := tm.Entries()[0]
	frag := model.RunsToFragment(e.Variant("en"))
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
	assert.Equal(t, "Click  to continue", frag.Text())
	// Placeholder span.
	require.Len(t, frag.Spans, 1)
	assert.Equal(t, "tmx:ph", frag.Spans[0].SubType)
	assert.Equal(t, "{0}", frag.Spans[0].Data)
}

func TestImportTMX_ElementBPT_EPT(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body>
<tu><tuv xml:lang="en"><seg>Click <bpt i="1">&lt;b&gt;</bpt>here<ept i="1">&lt;/b&gt;</ept></seg></tuv><tuv xml:lang="fr"><seg>Cliquer <bpt i="1">&lt;b&gt;</bpt>ici<ept i="1">&lt;/b&gt;</ept></seg></tuv></tu>
</body></tmx>`
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{})
	assert.Equal(t, 1, n)
	e := tm.Entries()[0]
	frag := model.RunsToFragment(e.Variant("en"))
	require.NotNil(t, frag)
	require.Len(t, frag.Spans, 2)
	assert.Equal(t, "tmx:bpt", frag.Spans[0].SubType)
	assert.Equal(t, "tmx:ept", frag.Spans[1].SubType)
	assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
	assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
	assert.Equal(t, "<b>", frag.Spans[0].Data)
	assert.Equal(t, "</b>", frag.Spans[1].Data)
}

func TestImportTMX_ElementIT(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body>
<tu><tuv xml:lang="en"><seg><it pos="begin">&lt;span&gt;</it>text<it pos="end">&lt;/span&gt;</it></seg></tuv><tuv xml:lang="fr"><seg>texte</seg></tuv></tu>
</body></tmx>`
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{})
	assert.Equal(t, 1, n)
	e := tm.Entries()[0]
	frag := model.RunsToFragment(e.Variant("en"))
	require.Len(t, frag.Spans, 2)
	assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
	assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
}

func TestImportTMX_ElementHI(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body>
<tu><tuv xml:lang="en"><seg>This is <hi type="b">important</hi> text</seg></tuv><tuv xml:lang="fr"><seg>Ceci est <hi type="b">important</hi></seg></tuv></tu>
</body></tmx>`
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{})
	assert.Equal(t, 1, n)
	e := tm.Entries()[0]
	frag := model.RunsToFragment(e.Variant("en"))
	require.Len(t, frag.Spans, 2)
	assert.Equal(t, "tmx:hi", frag.Spans[0].SubType)
	assert.Equal(t, "tmx:hi", frag.Spans[1].SubType)
	assert.Contains(t, frag.Text(), "important")
}

func TestImportTMX_ElementUT(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body>
<tu><tuv xml:lang="en"><seg>foo<ut>{weird}</ut>bar</seg></tuv><tuv xml:lang="fr"><seg>foo bar</seg></tuv></tu>
</body></tmx>`
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{})
	assert.Equal(t, 1, n)
	e := tm.Entries()[0]
	frag := model.RunsToFragment(e.Variant("en"))
	require.Len(t, frag.Spans, 1)
	assert.Equal(t, "tmx:ut", frag.Spans[0].SubType)
}

func TestImportTMX_HeaderMetadata(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="bitextor" creationtoolversion="8.0" segtype="sentence"
          adminlang="en" srclang="en" datatype="plaintext"
          o-tmf="tmx" o-encoding="utf-8">
    <prop type="original-format">tmx 1.4</prop>
    <prop type="corpus">bitextor-2024</prop>
  </header>
  <body>
    <tu><tuv xml:lang="en"><seg>hi</seg></tuv><tuv xml:lang="nb"><seg>hei</seg></tuv></tu>
  </body>
</tmx>`
	sid, _ := importString(t, tm, body, sievepen.ImportTMXOptions{OriginKey: "bitextor.tmx"})
	s, ok := tm.GetImportSession(sid)
	require.True(t, ok)
	assert.Equal(t, "bitextor", s.ToolName)
	assert.Equal(t, "8.0", s.ToolVersion)
	assert.Equal(t, "sentence", s.SegType)
	assert.Equal(t, "tmx", s.OriginalFormat)
	assert.Equal(t, "utf-8", s.OriginalEncoding)
	assert.Equal(t, "bitextor-2024", s.Properties["corpus"])
}

func TestImportTMX_SourceDocumentProp(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="bitextor" creationtoolversion="8" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu>
      <tuv xml:lang="en"><prop type="source-document">https://example.com/en/doc</prop><seg>hi</seg></tuv>
      <tuv xml:lang="nb"><seg>hei</seg></tuv>
    </tu>
  </body>
</tmx>`
	_, _ = importString(t, tm, body, sievepen.ImportTMXOptions{OriginKey: "bitextor.tmx"})
	entries := tm.Entries()
	require.Len(t, entries, 1)
	require.Len(t, entries[0].Origins, 1)
	assert.Equal(t, "https://example.com/en/doc", entries[0].Origins[0].Reference)
}

func TestImportTMX_DuplicateHashWarn(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
<body><tu><tuv xml:lang="en"><seg>a</seg></tuv><tuv xml:lang="fr"><seg>b</seg></tuv></tu></body></tmx>`

	_, _ = importString(t, tm, body, sievepen.ImportTMXOptions{OriginKey: "f.tmx"})

	var warned string
	warnFn := func(msg string) { warned = msg }
	_, n := importString(t, tm, body, sievepen.ImportTMXOptions{OriginKey: "f.tmx", WarnFunc: warnFn})
	assert.Equal(t, 1, n) // proceeds anyway
	assert.Contains(t, warned, "previously imported")
	assert.Len(t, tm.ListImportSessions(), 2)
}

func TestImportTMX_LocaleFilter(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
<body>
<tu><tuv xml:lang="en"><seg>a</seg></tuv><tuv xml:lang="fr"><seg>b</seg></tuv><tuv xml:lang="de"><seg>c</seg></tuv></tu>
</body></tmx>`
	_, _ = importString(t, tm, body, sievepen.ImportTMXOptions{Locales: []model.LocaleID{"en", "fr"}})
	e := tm.Entries()[0]
	assert.Len(t, e.Variants, 2)
	assert.True(t, e.HasLocale("en"))
	assert.True(t, e.HasLocale("fr"))
	assert.False(t, e.HasLocale("de"))
}

func TestImportTMX_CustomMappingFile(t *testing.T) {
	// Uses default mapping only — custom file loading exercised by mapping test.
	m, err := sievepen.DefaultTMXMapping()
	require.NoError(t, err)
	assert.Equal(t, "code:placeholder", m.Resolve("ph", ""))
	assert.Equal(t, "media:image", m.Resolve("ph", "image"))
	assert.Equal(t, "fmt:bold", m.Resolve("bpt", "b"))
	assert.Equal(t, "code:markup", m.Resolve("unknown", ""))
}

// Verify that the legacy bilingual import helper still produces variant-based entries.
func TestImportTMXWithOptions_LegacyBilingual(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	body := bytes.NewBufferString(`<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
<body><tu><tuv xml:lang="en"><seg>hello</seg></tuv><tuv xml:lang="fr"><seg>bonjour</seg></tuv></tu></body></tmx>`)
	n, err := sievepen.ImportTMXWithOptions(tm, body, "en", "fr", sievepen.ImportTMXOptions{OriginKey: "legacy.tmx"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	e := tm.Entries()[0]
	assert.Equal(t, "hello", e.VariantText("en"))
	assert.Equal(t, "bonjour", e.VariantText("fr"))
}
