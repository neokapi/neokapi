package termbase

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tbxBasicWellFormed = `<?xml version="1.0" encoding="UTF-8"?>
<tbx style="dca" type="TBX-Basic" xml:lang="en">
  <text>
    <body>
      <conceptEntry id="c1">
        <descrip type="definition">A piece of software for editing text.</descrip>
        <descrip type="subjectField">software</descrip>
        <langSec xml:lang="en">
          <termSec>
            <term>text editor</term>
            <termNote type="partOfSpeech">noun</termNote>
            <termNote type="administrativeStatus">preferredTerm-admn-sts</termNote>
          </termSec>
          <termSec>
            <term>editor</term>
            <termNote type="administrativeStatus">admittedTerm-admn-sts</termNote>
          </termSec>
        </langSec>
        <langSec xml:lang="fr">
          <termSec>
            <term>éditeur de texte</term>
            <termNote type="partOfSpeech">noun</termNote>
            <termNote type="grammaticalGender">masculine</termNote>
          </termSec>
        </langSec>
      </conceptEntry>
      <conceptEntry id="c2">
        <langSec xml:lang="en">
          <termSec>
            <term>deprecated thing</term>
            <termNote type="administrativeStatus">deprecatedTerm-admn-sts</termNote>
          </termSec>
        </langSec>
      </conceptEntry>
    </body>
  </text>
</tbx>`

const martifDoc = `<?xml version="1.0" encoding="UTF-8"?>
<martif type="TBX" xml:lang="en">
  <text>
    <body>
      <termEntry id="m1">
        <descripGrp>
          <descrip type="definition">A natural language used by people.</descrip>
        </descripGrp>
        <descrip type="subjectField">linguistics</descrip>
        <langSet xml:lang="en">
          <tig>
            <term>language</term>
            <termNote type="partOfSpeech">noun</termNote>
            <termNote type="administrativeStatus">preferredTerm-admn-sts</termNote>
          </tig>
        </langSet>
        <langSet xml:lang="de">
          <ntig>
            <termGrp>
              <term>Sprache</term>
              <termNote type="partOfSpeech">noun</termNote>
              <termNote type="grammaticalGender">feminine</termNote>
              <termNote type="administrativeStatus">admittedTerm-admn-sts</termNote>
            </termGrp>
          </ntig>
        </langSet>
      </termEntry>
    </body>
  </text>
</martif>`

func conceptByID(t *testing.T, tb TermBase, id string) Concept {
	t.Helper()
	c, ok := tb.GetConcept(id)
	require.True(t, ok, "concept %q should exist", id)
	return c
}

func TestImportTBX(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		opts    TBXImportOptions
		wantNum int
		check   func(t *testing.T, tb TermBase)
	}{
		{
			name:    "TBX-Basic v3",
			doc:     tbxBasicWellFormed,
			wantNum: 2,
			check: func(t *testing.T, tb TermBase) {
				c1 := conceptByID(t, tb, "c1")
				assert.Equal(t, "A piece of software for editing text.", c1.Definition)
				assert.Equal(t, "software", c1.Domain)
				require.Len(t, c1.Terms, 3)

				en := c1.SourceTerm("en")
				require.NotNil(t, en)
				assert.Equal(t, "text editor", en.Text)
				assert.Equal(t, "noun", en.PartOfSpeech)
				assert.Equal(t, model.TermPreferred, en.Status)

				// second en term is admitted
				ens := c1.TargetTerms("en")
				require.Len(t, ens, 2)
				assert.Equal(t, "editor", ens[1].Text)
				assert.Equal(t, model.TermAdmitted, ens[1].Status)

				fr := c1.SourceTerm("fr")
				require.NotNil(t, fr)
				assert.Equal(t, "éditeur de texte", fr.Text)
				assert.Equal(t, "masculine", fr.Gender)

				c2 := conceptByID(t, tb, "c2")
				dep := c2.SourceTerm("en")
				require.NotNil(t, dep)
				assert.Equal(t, model.TermDeprecated, dep.Status)
			},
		},
		{
			name:    "MARTIF / TBX 2008 with tig and ntig",
			doc:     martifDoc,
			wantNum: 1,
			check: func(t *testing.T, tb TermBase) {
				c := conceptByID(t, tb, "m1")
				assert.Equal(t, "A natural language used by people.", c.Definition)
				assert.Equal(t, "linguistics", c.Domain)

				en := c.SourceTerm("en")
				require.NotNil(t, en)
				assert.Equal(t, "language", en.Text)
				assert.Equal(t, "noun", en.PartOfSpeech)
				assert.Equal(t, model.TermPreferred, en.Status)

				de := c.SourceTerm("de")
				require.NotNil(t, de)
				assert.Equal(t, "Sprache", de.Text, "ntig/termGrp term should be extracted")
				assert.Equal(t, "feminine", de.Gender)
				assert.Equal(t, model.TermAdmitted, de.Status)
			},
		},
		{
			name:    "default status applied when no status note",
			doc:     martifDoc,
			opts:    TBXImportOptions{DefaultStatus: model.TermProposed},
			wantNum: 1,
			check: func(t *testing.T, tb TermBase) {
				// The MARTIF doc declares explicit statuses, so the default is
				// only a fallback. Verify the explicit ones still win.
				c := conceptByID(t, tb, "m1")
				assert.Equal(t, model.TermPreferred, c.SourceTerm("en").Status)
			},
		},
		{
			name:    "fallback domain and id prefix",
			doc:     tbxBasicWellFormed,
			opts:    TBXImportOptions{Domain: "fallback-domain", Source: TermSourceBrandVocabulary},
			wantNum: 2,
			check: func(t *testing.T, tb TermBase) {
				// c2 has no subjectField, so the fallback domain applies.
				c2 := conceptByID(t, tb, "c2")
				assert.Equal(t, "fallback-domain", c2.Domain)
				assert.Equal(t, TermSourceBrandVocabulary, c2.Source)
				// c1 declares its own subjectField, which wins over the fallback.
				c1 := conceptByID(t, tb, "c1")
				assert.Equal(t, "software", c1.Domain)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := NewInMemoryTermBase()
			n, err := ImportTBX(tb, strings.NewReader(tt.doc), tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNum, n)
			assert.Equal(t, tt.wantNum, tb.Count())
			if tt.check != nil {
				tt.check(t, tb)
			}
		})
	}
}

func TestImportTBXGeneratedIDs(t *testing.T) {
	doc := `<?xml version="1.0"?>
<tbx><text><body>
  <conceptEntry>
    <langSec xml:lang="en"><termSec><term>alpha</term></termSec></langSec>
  </conceptEntry>
  <conceptEntry>
    <langSec xml:lang="en"><termSec><term>beta</term></termSec></langSec>
  </conceptEntry>
</body></text></tbx>`

	tb := NewInMemoryTermBase()
	n, err := ImportTBX(tb, strings.NewReader(doc), TBXImportOptions{IDPrefix: "gen"})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	_, ok := tb.GetConcept("gen-1")
	assert.True(t, ok)
	_, ok = tb.GetConcept("gen-2")
	assert.True(t, ok)
}

func TestImportTBXMalformed(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{name: "broken XML", doc: `<tbx><text><body><conceptEntry></tbx`},
		{name: "unknown root", doc: `<?xml version="1.0"?><glossary><entry/></glossary>`},
		{name: "empty", doc: ``},
		{name: "not XML at all", doc: `this is not xml { json: true }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := NewInMemoryTermBase()
			n, err := ImportTBX(tb, strings.NewReader(tt.doc), TBXImportOptions{})
			require.Error(t, err)
			assert.Equal(t, 0, n)
			assert.Equal(t, 0, tb.Count())
		})
	}
}

func TestExportTBXRoundTrip(t *testing.T) {
	src := NewInMemoryTermBase()
	require.NoError(t, src.AddConcept(Concept{
		ID:         "rt1",
		Domain:     "software",
		Definition: "A piece of software for editing text.",
		Terms: []Term{
			{Text: "text editor", Locale: "en", Status: model.TermPreferred, PartOfSpeech: "noun"},
			{Text: "editor", Locale: "en", Status: model.TermAdmitted},
			{Text: "éditeur de texte", Locale: "fr", Status: model.TermApproved, Gender: "masculine", Note: "common usage"},
		},
	}))
	require.NoError(t, src.AddConcept(Concept{
		ID: "rt2",
		Terms: []Term{
			{Text: "obsolete", Locale: "en", Status: model.TermDeprecated},
			{Text: "banned", Locale: "en", Status: model.TermForbidden},
			{Text: "draft", Locale: "en", Status: model.TermProposed},
		},
	}))

	var buf bytes.Buffer
	require.NoError(t, ExportTBX(src, &buf, TBXExportOptions{}))

	// The export must be a TBX-Basic v3 document.
	assert.True(t, strings.Contains(buf.String(), "<tbx"), "export should use <tbx> root")
	assert.True(t, strings.Contains(buf.String(), "<conceptEntry"), "export should use conceptEntry")

	// Re-import the exported document.
	dst := NewInMemoryTermBase()
	n, err := ImportTBX(dst, bytes.NewReader(buf.Bytes()), TBXImportOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, n)

	assertConceptsEquivalent(t, src, dst)
}

// assertConceptsEquivalent verifies that two termbases hold equivalent concepts
// (ignoring timestamps and term ordering within a locale).
func assertConceptsEquivalent(t *testing.T, a, b TermBase) {
	t.Helper()
	ac := a.Concepts()
	bc := b.Concepts()
	require.Equal(t, len(ac), len(bc), "concept counts must match")

	for _, want := range ac {
		got, ok := b.GetConcept(want.ID)
		require.True(t, ok, "concept %q missing after round-trip", want.ID)
		assert.Equal(t, want.Definition, got.Definition, "definition for %s", want.ID)
		assert.Equal(t, want.Domain, got.Domain, "domain for %s", want.ID)
		assertTermsEquivalent(t, want.ID, want.Terms, got.Terms)
	}
}

func assertTermsEquivalent(t *testing.T, conceptID string, want, got []Term) {
	t.Helper()
	require.Equal(t, len(want), len(got), "term count for %s", conceptID)

	key := func(tm Term) string {
		return string(tm.Locale) + "|" + tm.Text
	}
	sortTerms := func(ts []Term) {
		sort.Slice(ts, func(i, j int) bool { return key(ts[i]) < key(ts[j]) })
	}
	w := append([]Term{}, want...)
	g := append([]Term{}, got...)
	sortTerms(w)
	sortTerms(g)

	for i := range w {
		assert.Equal(t, w[i].Text, g[i].Text, "term text in %s", conceptID)
		assert.Equal(t, w[i].Locale, g[i].Locale, "term locale in %s", conceptID)
		assert.Equal(t, w[i].Status, g[i].Status, "term status for %q in %s", w[i].Text, conceptID)
		assert.Equal(t, w[i].PartOfSpeech, g[i].PartOfSpeech, "PoS for %q in %s", w[i].Text, conceptID)
		assert.Equal(t, w[i].Gender, g[i].Gender, "gender for %q in %s", w[i].Text, conceptID)
		assert.Equal(t, w[i].Note, g[i].Note, "note for %q in %s", w[i].Text, conceptID)
	}
}

func TestExportTBXSourceLocaleFilter(t *testing.T) {
	tb := NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(Concept{
		ID:    "has-en",
		Terms: []Term{{Text: "hello", Locale: "en", Status: model.TermApproved}},
	}))
	require.NoError(t, tb.AddConcept(Concept{
		ID:    "no-en",
		Terms: []Term{{Text: "bonjour", Locale: "fr", Status: model.TermApproved}},
	}))

	var buf bytes.Buffer
	require.NoError(t, ExportTBX(tb, &buf, TBXExportOptions{SourceLocale: "en"}))

	out := buf.String()
	assert.Contains(t, out, `id="has-en"`)
	assert.NotContains(t, out, `id="no-en"`, "concept without the source locale should be filtered out")
}
