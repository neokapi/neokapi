package openxml

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests lock in the structural language-retargeting that replaced the
// write-side rewriteWMLLangVal regex (#607). The reader splices the
// `<w:themeFontLang>` w:val out of a settings part as a typed SkeletonLang
// entry; the writer retargets it during skeleton reconstruction when the
// value's primary language matches the source locale and a target locale is
// set. Strict parts and non-matching values pass through byte-exact.

// docxWithSettings clones a transitional DOCX (simple.docx has no settings
// part) and injects a word/settings.xml entry with the given body. The
// reader's settings-lang splice looks the part up by path, so no
// relationship/content-type wiring is required for this test.
func docxWithSettings(t *testing.T, settingsBody []byte) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	zr, err := zip.NewReader(bytes.NewReader(src), int64(len(src)))
	require.NoError(t, err)

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		require.NoError(t, zw.Copy(f))
	}
	w, err := zw.Create("word/settings.xml")
	require.NoError(t, err)
	_, err = w.Write(settingsBody)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// roundTripDocxLang reads input with a skeleton store and writes it back with
// the given source/target locales, returning the output ZIP bytes.
func roundTripDocxLang(t *testing.T, input []byte, src, tgt model.LocaleID) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: src,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(input),
	}
	require.NoError(t, reader.Open(t.Context(), doc))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(input)
	writer.SetSkeletonStore(skelStore)
	writer.SetSourceLocale(src)
	if !tgt.IsEmpty() {
		writer.SetLocale(tgt)
	}
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	writer.Close()

	require.Positive(t, buf.Len(), "output should not be empty")
	return buf.Bytes()
}

func settingsXMLOf(t *testing.T, docx []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	require.NoError(t, err)
	zf := zipFileByName(zr, "word/settings.xml")
	require.NotNil(t, zf, "output should contain word/settings.xml")
	data, err := readZipFile(zf)
	require.NoError(t, err)
	return string(data)
}

// TestThemeFontLangRetargetedToTarget: a transitional themeFontLang whose
// w:val primary language matches the source is retargeted to the target
// locale; the unrelated w:eastAsia attribute is preserved verbatim.
func TestThemeFontLangRetargetedToTarget(t *testing.T) {
	settings := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:themeFontLang w:val="en-US" w:eastAsia="ja-JP"/></w:settings>`)
	in := docxWithSettings(t, settings)

	out := settingsXMLOf(t, roundTripDocxLang(t, in, model.LocaleID("en"), model.LocaleID("fr")))

	assert.Contains(t, out, `w:val="fr"`, "w:val should be retargeted to the target locale")
	assert.NotContains(t, out, `w:val="en-US"`, "source w:val should no longer be present")
	assert.Contains(t, out, `w:eastAsia="ja-JP"`, "w:eastAsia must be preserved verbatim")
}

// TestThemeFontLangPreservedWithoutTarget: with no target locale the value is
// emitted verbatim — the settings part round-trips byte-exact.
func TestThemeFontLangPreservedWithoutTarget(t *testing.T) {
	settings := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:themeFontLang w:val="en-US" w:eastAsia="ja-JP"/></w:settings>`)
	in := docxWithSettings(t, settings)

	out := settingsXMLOf(t, roundTripDocxLang(t, in, model.LocaleID("en"), model.LocaleID("")))

	assert.Equal(t, string(settings), out, "no-target round-trip must be byte-exact")
}

// TestThemeFontLangUnrelatedLocalePreserved: a value whose primary language
// differs from the source locale is preserved even when retargeting.
func TestThemeFontLangUnrelatedLocalePreserved(t *testing.T) {
	settings := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:themeFontLang w:val="ru-RU"/></w:settings>`)
	in := docxWithSettings(t, settings)

	out := settingsXMLOf(t, roundTripDocxLang(t, in, model.LocaleID("en"), model.LocaleID("fr")))

	assert.Contains(t, out, `w:val="ru-RU"`, "unrelated-language value must be preserved")
}

// TestThemeFontLangStrictPreserved: a strict-OOXML settings part is never
// retargeted (upstream's Property.LANGUAGE rewrite is QName-keyed to the
// transitional URI). The reader emits it as verbatim text, not a SkeletonLang
// entry, so even a source-matching en value round-trips unchanged.
func TestThemeFontLangStrictPreserved(t *testing.T) {
	settings := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:settings xmlns:w="http://purl.oclc.org/ooxml/wordprocessingml/main">` +
		`<w:themeFontLang w:val="en-US"/></w:settings>`)
	in := docxWithSettings(t, settings)

	out := settingsXMLOf(t, roundTripDocxLang(t, in, model.LocaleID("en"), model.LocaleID("fr")))

	assert.Contains(t, out, `w:val="en-US"`, "strict themeFontLang must round-trip unchanged")
	assert.NotContains(t, out, `w:val="fr"`, "strict part must not be retargeted")
}
