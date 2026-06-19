package klz

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/sievepen/klftm"
	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/klftb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleBlocks() *klf.File {
	return &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "tool", Version: "1"},
		Project:       klf.ProjectInfo{ID: "p", SourceLocale: "en"},
		Documents: []klf.Document{{
			ID: "d", DocumentType: klf.DocumentTypeJSX, Path: "a.tsx",
			Blocks: []klf.Block{{
				ID: "b1", Hash: "h1", Translatable: true, Type: klf.BlockTypeJSXElement,
				Source:       []klf.Run{{Text: &klf.TextRun{Text: "Hi <b>there</b> & more"}}},
				Targets:      map[string][]klf.Run{"fr": {{Text: &klf.TextRun{Text: "Salut"}}}},
				Placeholders: []klf.Placeholder{},
				Properties:   klf.BlockProperties{File: "a.tsx", Line: 1, Component: "C", JSXPath: "p", Element: "p"},
			}},
		}},
	}
}

func sampleAnnotations() *klf.AnnotationFile {
	return &klf.AnnotationFile{
		Header: klf.AnnotationFileHeader{
			Type: "header", AnnotationType: "@neokapi/test", AnnotationVersion: "1.0.0",
			Producer: klf.AnnotationProducer{ID: "tool", Version: "1"},
			Created:  "2026-01-01T00:00:00Z", TargetArchive: "sha256:x",
		},
		Annotations: []klf.Annotation{{
			Type: "annotation", ID: "a1",
			Anchor: klf.AnnotationAnchor{Kind: klf.AnchorBlock, Block: "b1"},
		}},
	}
}

func sampleTM() []sievepen.TMEntry {
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return []sievepen.TMEntry{{
		ID: "tm-1", ProjectID: "p", HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
		Entities:   []sievepen.EntityMapping{{PlaceholderID: "e1", Type: "person", ConceptID: "c-1", Values: map[model.LocaleID]sievepen.EntityValue{"en": {Text: "Ada", Start: 0, End: 3}}}},
		Properties: map[string]string{"domain": "greeting"},
		Note:       "ok", CreatedAt: t0, UpdatedAt: t0,
	}}
}

func sampleTermbase() []termbase.Concept {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	return []termbase.Concept{{
		ID: "c-1", Domain: "software", Definition: "primary CTA",
		Source: termbase.TermSourceBrandVocabulary,
		Terms: []termbase.Term{
			{Text: "Get started", Locale: "en", Status: model.TermStatus("preferred")},
			{Text: "BadBrand", Locale: "en", Status: model.TermStatus("forbidden"), CompetitorTerm: true},
		},
		Properties: map[string]string{"tone": "friendly"},
		CreatedAt:  t0, UpdatedAt: t0,
	}}
}

func samplePackage() *Package {
	return &Package{
		Created:     "2026-06-04T00:00:00Z",
		Generator:   &GeneratorInfo{ID: "kapi", Version: "1"},
		Blocks:      []BlockDoc{{Path: "blocks/a.klf", File: sampleBlocks()}},
		Annotations: []AnnotationDoc{{Path: "annotations/a.klfl", File: sampleAnnotations()}},
		TM:          klftm.FromModel(sampleTM(), nil),
		Termbase:    klftb.FromConcepts(sampleTermbase()),
		Media:       []Media{{Path: "media/logo.bin", Content: BytesContent([]byte{0x89, 0x50, 0x4e, 0x47})}},
	}
}

func TestPackageRoundTrip(t *testing.T) {
	pkg := samplePackage()
	data, err := pkg.Marshal()
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)

	// Whole-package round-trip: re-marshaling the unpacked package yields the
	// exact same bytes — every member survived losslessly.
	data2, err := got.Marshal()
	require.NoError(t, err)
	require.Equal(t, data, data2, "package must round-trip byte-for-byte")

	// Structural sanity across all content types.
	require.Len(t, got.Blocks, 1)
	require.Len(t, got.Annotations, 1)
	require.NotNil(t, got.TM)
	require.NotNil(t, got.Termbase)
	require.Len(t, got.Media, 1)
	gotMedia, err := ReadAll(got.Media[0].Content)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, gotMedia)
	assert.Equal(t, "tm-1", got.TM.Entries[0].ID)
	assert.Equal(t, "c-1", got.Termbase.Concepts[0].ID)
	// The block's markup with < > & survived unescaped through the KLF member.
	assert.Equal(t, "Hi <b>there</b> & more", got.Blocks[0].File.Documents[0].Blocks[0].Source[0].Text.Text)
}

func TestMarshalDeterministic(t *testing.T) {
	a, err := samplePackage().Marshal()
	require.NoError(t, err)
	b, err := samplePackage().Marshal()
	require.NoError(t, err)
	assert.Equal(t, a, b, "Marshal must be byte-deterministic")
}

func TestManifestIntegrity(t *testing.T) {
	data, err := samplePackage().Marshal()
	require.NoError(t, err)

	// Flip a byte somewhere in the archive payload; Unmarshal must reject it
	// (either checksum or zip-structure failure).
	corrupt := make([]byte, len(data))
	copy(corrupt, data)
	corrupt[len(corrupt)/2] ^= 0xff
	_, err = Unmarshal(corrupt)
	require.Error(t, err, "a corrupted package must not silently parse")
}

func TestUnmarshalRejectsNonPackage(t *testing.T) {
	_, err := Unmarshal([]byte("not a zip"))
	require.Error(t, err)
}

// TestCacheInternalRoundTrip is the headline guarantee: populate real TM and
// termbase stores, pack them into a .klz, unpack into FRESH stores, and prove
// the cache content survived losslessly — by re-packing the fresh stores and
// asserting byte-identical output. Exercises the actual sievepen / termbase
// stores end to end, not just the formats in isolation.
func TestCacheInternalRoundTrip(t *testing.T) {
	// 1. Populate real stores.
	tm1 := sievepen.NewInMemoryTM()
	for _, e := range sampleTM() {
		require.NoError(t, tm1.Add(t.Context(), e))
	}
	require.NoError(t, tm1.CreateImportSession(t.Context(), sievepen.ImportSession{ID: "sess-1", FileKey: "x.tmx", ToolName: "neokapi", ImportedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}))

	tb1 := termbase.NewInMemoryTermBase()
	for _, c := range sampleTermbase() {
		require.NoError(t, tb1.AddConcept(t.Context(), c))
	}

	// 2. Pack straight from the stores.
	tm1Entries, err := tm1.Entries(t.Context())
	require.NoError(t, err)
	tm1Sessions, err := tm1.ListImportSessions(t.Context())
	require.NoError(t, err)
	tb1Concepts, err := tb1.Concepts(t.Context())
	require.NoError(t, err)
	pkg := &Package{
		TM:       klftm.FromModel(tm1Entries, tm1Sessions),
		Termbase: klftb.FromConcepts(tb1Concepts),
	}
	data, err := pkg.Marshal()
	require.NoError(t, err)

	// 3. Unpack into FRESH stores.
	got, err := Unmarshal(data)
	require.NoError(t, err)

	tm2 := sievepen.NewInMemoryTM()
	for _, e := range got.TM.ModelEntries() {
		require.NoError(t, tm2.Add(t.Context(), e))
	}
	for _, s := range got.TM.ModelImportSessions() {
		require.NoError(t, tm2.CreateImportSession(t.Context(), s))
	}
	tb2 := termbase.NewInMemoryTermBase()
	for _, c := range got.Termbase.Concepts {
		require.NoError(t, tb2.AddConcept(t.Context(), c))
	}

	// 4. Re-pack from the fresh stores; identical canonical bytes ⇒ the
	//    store→pack→unpack→store cycle lost nothing.
	tm2Entries, err := tm2.Entries(t.Context())
	require.NoError(t, err)
	tm2Sessions, err := tm2.ListImportSessions(t.Context())
	require.NoError(t, err)
	tb2Concepts, err := tb2.Concepts(t.Context())
	require.NoError(t, err)
	repacked, err := (&Package{
		TM:       klftm.FromModel(tm2Entries, tm2Sessions),
		Termbase: klftb.FromConcepts(tb2Concepts),
	}).Marshal()
	require.NoError(t, err)
	require.Equal(t, data, repacked, "store→pack→unpack→store must be lossless (byte-identical)")

	// And the AI-native fields TMX/TBX would have dropped are present.
	require.Equal(t, "c-1", tm2Entries[0].Entities[0].ConceptID, "TM↔termbase cross-link survived the store cycle")
	require.True(t, tb2Concepts[0].Terms[1].CompetitorTerm || tb2Concepts[0].Terms[0].CompetitorTerm, "competitor flag survived the store cycle")
}
