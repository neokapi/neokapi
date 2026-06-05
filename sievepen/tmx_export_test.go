package sievepen_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportTMX_Multilingual(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Hello", "Bonjour", "Hallo")))
	var buf bytes.Buffer
	require.NoError(t, sievepen.ExportTMX(context.Background(), tm, &buf, nil))
	out := buf.String()
	assert.Contains(t, out, `<tmx version="1.4">`)
	assert.Contains(t, out, `xml:lang="en"`)
	assert.Contains(t, out, `xml:lang="fr"`)
	assert.Contains(t, out, `xml:lang="de"`)
	assert.Contains(t, out, "Hello")
	assert.Contains(t, out, "Bonjour")
	assert.Contains(t, out, "Hallo")
}

func TestExportTMX_LocalesFilter(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Hello", "Bonjour", "Hallo")))
	var buf bytes.Buffer
	require.NoError(t, sievepen.ExportTMX(context.Background(), tm, &buf, []model.LocaleID{"en", "fr"}))
	out := buf.String()
	assert.Contains(t, out, `xml:lang="en"`)
	assert.Contains(t, out, `xml:lang="fr"`)
	assert.NotContains(t, out, `xml:lang="de"`)
}

func TestExportTMX_BilingualShim(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Hello", "Bonjour", "Hallo")))
	var buf bytes.Buffer
	require.NoError(t, sievepen.ExportTMXBilingual(context.Background(), tm, &buf, "en", "de"))
	out := buf.String()
	assert.Contains(t, out, "Hello")
	assert.Contains(t, out, "Hallo")
	assert.NotContains(t, out, "Bonjour")
}

func TestExportTMX_EscapesSpecialChars(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(context.Background(), sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "A & B < C > D"}}},
			"fr": {{Text: &model.TextRun{Text: "A et B"}}},
		},
	}))
	var buf bytes.Buffer
	require.NoError(t, sievepen.ExportTMX(context.Background(), tm, &buf, nil))
	out := buf.String()
	assert.Contains(t, out, "A &amp; B &lt; C &gt; D")
}

// --- Roundtrip ---

func TestTMXRoundtrip_Bilingual(t *testing.T) {
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
<body><tu tuid="x1"><tuv xml:lang="en"><seg>Hello</seg></tuv><tuv xml:lang="fr"><seg>Bonjour</seg></tuv></tu></body></tmx>`

	tm1 := sievepen.NewInMemoryTM()
	_, n, err := sievepen.ImportTMXSession(context.Background(), tm1, strings.NewReader(body), sievepen.ImportTMXOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var buf bytes.Buffer
	require.NoError(t, sievepen.ExportTMX(context.Background(), tm1, &buf, nil))

	tm2 := sievepen.NewInMemoryTM()
	_, _, err = sievepen.ImportTMXSession(context.Background(), tm2, &buf, sievepen.ImportTMXOptions{})
	require.NoError(t, err)

	e := mustEntries(t, tm2)[0]
	assert.Equal(t, "Hello", e.VariantText("en"))
	assert.Equal(t, "Bonjour", e.VariantText("fr"))
}

func TestTMXRoundtrip_Multilingual(t *testing.T) {
	body := `<?xml version="1.0"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
<body><tu><tuv xml:lang="en"><seg>A</seg></tuv><tuv xml:lang="fr"><seg>B</seg></tuv><tuv xml:lang="de"><seg>C</seg></tuv><tuv xml:lang="es"><seg>D</seg></tuv></tu></body></tmx>`

	tm1 := sievepen.NewInMemoryTM()
	_, _, err := sievepen.ImportTMXSession(context.Background(), tm1, strings.NewReader(body), sievepen.ImportTMXOptions{})
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, sievepen.ExportTMX(context.Background(), tm1, &buf, nil))

	tm2 := sievepen.NewInMemoryTM()
	_, _, err = sievepen.ImportTMXSession(context.Background(), tm2, &buf, sievepen.ImportTMXOptions{})
	require.NoError(t, err)

	e := mustEntries(t, tm2)[0]
	assert.Len(t, e.Variants, 4)
}
