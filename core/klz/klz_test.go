package klz

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePartPath(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"documents/foo.klf", false},
		{"annotations/@vendor-name.klfl", false},
		{"", true},
		{"/absolute", true},
		{"../escape", true},
		{"a/../b", true},
		{"a//b", true},
		{"a/./b", true},
		{"back\\slash", true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			_, err := validatePartPath(tc.in)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClassifyPartRole(t *testing.T) {
	assert.Equal(t, RoleDocument, classifyPartRole("documents/x.klf"))
	assert.Equal(t, RoleTarget, classifyPartRole("targets/de/x.klf"))
	assert.Equal(t, RoleSkeleton, classifyPartRole("skeletons/x.skl"))
	assert.Equal(t, RoleAnnotation, classifyPartRole("annotations/p.klfl"))
	assert.Equal(t, RoleMeta, classifyPartRole("manifest.json"))
	assert.Equal(t, RoleMeta, classifyPartRole("meta.json"))
	assert.Equal(t, RoleAsset, classifyPartRole("extras/mystery.bin"))
}

// buildExampleArchive writes a .klz with the three example blocks,
// a German target overlay for the files-heading block, and an
// example annotation sidecar. Returns the bytes.
func buildExampleArchive(t *testing.T) []byte {
	t.Helper()

	w := NewWriter(WriterOptions{
		Generator: ManifestGenerator{ID: "@neokapi/format-examples", Version: "0.0.1"},
		Project:   ManifestProject{ID: "neokapi-format-examples", SourceLocale: "en", TargetLocales: []string{"de"}},
		Created:   "2026-04-15T10:00:00Z",
	})

	doc := exampleDocument()
	require.NoError(t, w.AddDocument("documents/examples.klf", doc, map[string]any{"documentId": "examples"}))

	target := exampleTargetDocument()
	require.NoError(t, w.AddTarget("targets/de/examples.klf", target, map[string]any{"locale": "de"}))

	require.NoError(t, w.AddSkeleton("skeletons/examples.skl", []byte("opaque-skeleton-bytes"), nil))

	annFile := exampleAnnotationFile()
	require.NoError(t, w.AddAnnotationFile("annotations/@neokapi-example.klfl", annFile, nil))

	var buf bytes.Buffer
	_, err := w.Write(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

func TestWriterReaderRoundTrip(t *testing.T) {
	data := buildExampleArchive(t)

	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	assert.Equal(t, ManifestVersion, r.Manifest().KapiLocalizationFormat)
	assert.Equal(t, "@neokapi/format-examples", r.Manifest().Generator.ID)

	// Manifest hash must be deterministic and consistent across
	// Reader instances over the same byte payload — that's the
	// stability contract the Phase-4 runtime cache will key off.
	hash1 := r.ManifestHash()
	r2, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r2.Close()
	assert.Equal(t, hash1, r2.ManifestHash())
	assert.NotEmpty(t, hash1)

	// VerifyAll should report zero problems on a freshly-written
	// archive.
	errs := r.VerifyAll()
	assert.Empty(t, errs)
}

func TestReaderDocuments(t *testing.T) {
	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	docs, err := r.Documents()
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Len(t, docs[0].Documents, 1)
	assert.Equal(t, 3, len(docs[0].Documents[0].Blocks))
}

func TestReaderTargets(t *testing.T) {
	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	targets, err := r.Targets("de")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Len(t, targets[0].Documents, 1)
}

func TestReaderAnnotationFiles(t *testing.T) {
	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	files, err := r.AnnotationFiles()
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "@neokapi/example", files[0].File.Header.AnnotationType)
	assert.Len(t, files[0].File.Annotations, 4)
}

func TestBlockByID(t *testing.T) {
	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	block, err := r.BlockByID(context.Background(), "tag-chip")
	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "tag-chip", block.ID)

	missing, err := r.BlockByID(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestVerifyAllCatchesTampering(t *testing.T) {
	data := buildExampleArchive(t)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	defer r.Close()

	// Corrupt one of the manifest SHA256 entries in-place so
	// VerifyAll reports a hash mismatch on the next pass.
	for i := range r.manifest.Parts {
		if r.manifest.Parts[i].Path == "documents/examples.klf" {
			r.manifest.Parts[i].SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
			break
		}
	}
	errs := r.VerifyAll()
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Kind == VerifyHashMismatch && e.Path == "documents/examples.klf" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestZIPSlipRejected(t *testing.T) {
	w := NewWriter(WriterOptions{
		Generator: ManifestGenerator{ID: "x", Version: "1"},
		Project:   ManifestProject{ID: "p", SourceLocale: "en"},
	})
	err := w.AddSkeleton("../escape.skl", []byte("x"), nil)
	require.Error(t, err)
}

func TestDuplicatePartPathRejected(t *testing.T) {
	w := NewWriter(WriterOptions{
		Generator: ManifestGenerator{ID: "x", Version: "1"},
		Project:   ManifestProject{ID: "p", SourceLocale: "en"},
	})
	require.NoError(t, w.AddSkeleton("skeletons/x.skl", []byte("a"), nil))
	err := w.AddSkeleton("skeletons/x.skl", []byte("b"), nil)
	require.Error(t, err)
}

func TestManifestVersionCheckRejectsUnknownMajor(t *testing.T) {
	// Build valid archive, then fudge the manifest version bytes.
	data := buildExampleArchive(t)
	_, err := NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	// Tamper: decode the manifest, set an unknown major, re-encode.
	// Easiest path: unmarshal the payload manually.
	bad := []byte(`{"kapiLocalizationFormat":"99.0","generator":{"id":"x","version":"1"},"project":{"id":"p","sourceLocale":"en"},"parts":[]}`)
	_, err = UnmarshalManifest(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported manifest major version")
}

// exampleDocument mirrors the Go-side fixture used for core/klf
// tests so the two packages agree on shape.
func exampleDocument() *klf.File {
	return &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "@neokapi/format-examples", Version: "0.0.1"},
		Project:       klf.ProjectInfo{ID: "neokapi-format-examples", SourceLocale: "en"},
		Documents: []klf.Document{
			{
				ID:           "examples",
				DocumentType: klf.DocumentTypeJSX,
				Path:         "examples/all.tsx",
				Blocks: []klf.Block{
					*fixtureFilesHeading(),
					*fixtureTagChip(),
					*fixtureShoppingCart(),
				},
			},
		},
	}
}

// exampleTargetDocument is a sparse target overlay that translates
// files-heading into German.
func exampleTargetDocument() *klf.File {
	b := klf.Block{
		ID:           "files-heading",
		Hash:         "2xykvb",
		Translatable: true,
		Type:         klf.BlockTypeJSXElement,
		Source:       fixtureFilesHeading().Source,
		Targets: map[string][]klf.Run{
			"de": {
				{Text: &klf.TextRun{Text: "Dateien "}},
				{PcOpen: &klf.PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: `<span className="muted">`, Equiv: "muted"}},
				{Text: &klf.TextRun{Text: "("}},
				{Ph: &klf.PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
					Data: "{count}", Equiv: "count"}},
				{Text: &klf.TextRun{Text: " passend)"}},
				{PcClose: &klf.PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: "</span>", Equiv: "muted"}},
			},
		},
		Placeholders: []klf.Placeholder{
			{Name: "muted", Kind: klf.PlaceholderElement, SourceExpr: `<span className="muted">...</span>`, JSType: "ReactNode"},
			{Name: "count", Kind: klf.PlaceholderVariable, SourceExpr: "count", JSType: "number"},
		},
		Properties: klf.BlockProperties{File: "src/FilesHeading.tsx", Line: 4,
			Component: "FilesHeading", JSXPath: "FilesHeading > h2", Element: "h2"},
	}
	return &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "@neokapi/format-examples", Version: "0.0.1"},
		Project:       klf.ProjectInfo{ID: "neokapi-format-examples", SourceLocale: "en"},
		Documents: []klf.Document{
			{
				ID:           "examples",
				DocumentType: klf.DocumentTypeJSX,
				Path:         "examples/all.tsx",
				Blocks:       []klf.Block{b},
			},
		},
	}
}

// exampleAnnotationFile mirrors klf_test.go's
// exampleAnnotationFile (scoped to the klz package for test
// isolation).
func exampleAnnotationFile() *klf.AnnotationFile {
	return &klf.AnnotationFile{
		Header: klf.AnnotationFileHeader{
			Type:              "header",
			AnnotationType:    "@neokapi/example",
			AnnotationVersion: "1.0.0",
			Producer: klf.AnnotationProducer{
				ID: "@neokapi/format-examples", Version: "0.0.1",
			},
			Created:       "2026-04-15T12:00:00Z",
			TargetArchive: "sha256:deadbeef",
		},
		Annotations: []klf.Annotation{
			{Type: "annotation", ID: "review-1",
				Anchor: klf.AnnotationAnchor{Kind: klf.AnchorBlock, Block: "files-heading"},
				Data:   []byte(`{"kind":"review"}`)},
			{Type: "annotation", ID: "term-1",
				Anchor: klf.AnnotationAnchor{
					Kind: klf.AnchorRun, Block: "tag-chip",
					Path:  klf.RunPath{{Kind: klf.StepIndex, Index: 2}},
					RunID: "2",
				},
				Data: []byte(`{"kind":"protected-term"}`)},
			{Type: "annotation", ID: "mt-1",
				Anchor: klf.AnnotationAnchor{
					Kind: klf.AnchorForm, Block: "shopping-cart-plural",
					Path: klf.RunPath{{Kind: klf.StepIndex, Index: 0}},
					Key:  "other",
				},
				Data: []byte(`{"kind":"mt-confidence"}`)},
			{Type: "annotation", ID: "term-2",
				Anchor: klf.AnnotationAnchor{
					Kind: klf.AnchorRange, Block: "files-heading",
					Path:   klf.RunPath{{Kind: klf.StepIndex, Index: 0}},
					Offset: 0,
					Length: 5,
				},
				Data: []byte(`{"kind":"glossary-match"}`)},
		},
	}
}

func fixtureFilesHeading() *klf.Block {
	return &klf.Block{
		ID: "files-heading", Hash: "2xykvb", Translatable: true,
		Type: klf.BlockTypeJSXElement,
		Source: []klf.Run{
			{Text: &klf.TextRun{Text: "Files "}},
			{PcOpen: &klf.PcOpenRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data:  `<span className="muted">`,
				Equiv: "muted", Disp: "span",
			}},
			{Text: &klf.TextRun{Text: "("}},
			{Ph: &klf.PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "number",
				Data: "{count}", Equiv: "count", Disp: "count",
			}},
			{Text: &klf.TextRun{Text: " matched)"}},
			{PcClose: &klf.PcCloseRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data: "</span>", Equiv: "muted",
			}},
		},
		Placeholders: []klf.Placeholder{
			{Name: "muted", Kind: klf.PlaceholderElement,
				SourceExpr: `<span className="muted">...</span>`, JSType: "ReactNode"},
			{Name: "count", Kind: klf.PlaceholderVariable,
				SourceExpr: "count", JSType: "number"},
		},
		Properties: klf.BlockProperties{
			File: "src/FilesHeading.tsx", Line: 4,
			Component: "FilesHeading", JSXPath: "FilesHeading > h2", Element: "h2",
		},
	}
}

func fixtureTagChip() *klf.Block {
	return &klf.Block{
		ID: "tag-chip", Hash: "2GcSuQ", Translatable: true,
		Type: klf.BlockTypeJSXElement,
		Source: []klf.Run{
			{Ph: &klf.PlaceholderRun{
				ID: "1", Type: "jsx:node", SubType: "logical-and",
				Data:  `index !== undefined && <span className="badge">{index}</span>`,
				Equiv: "badge", Disp: "⟨badge⟩",
			}},
			{Text: &klf.TextRun{Text: " "}},
			{Ph: &klf.PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "string",
				Data: "{label}", Equiv: "label", Disp: "label",
			}},
			{Text: &klf.TextRun{Text: " "}},
			{Ph: &klf.PlaceholderRun{
				ID: "3", Type: "jsx:node", SubType: "logical-and",
				Data:  `!deletable && <span className="required">*</span>`,
				Equiv: "required", Disp: "⟨required⟩",
			}},
		},
		Placeholders: []klf.Placeholder{
			{Name: "badge", Kind: klf.PlaceholderNode,
				SourceExpr: `index !== undefined && <span className="badge">{index}</span>`,
				JSType:     "ReactNode", Optional: true},
			{Name: "label", Kind: klf.PlaceholderVariable, SourceExpr: "label", JSType: "string"},
			{Name: "required", Kind: klf.PlaceholderNode,
				SourceExpr: `!deletable && <span className="required">*</span>`,
				JSType:     "ReactNode", Optional: true},
		},
		Properties: klf.BlockProperties{
			File: "src/TagChip.tsx", Line: 3,
			Component: "TagChip", JSXPath: "TagChip > span[data-tag-chip]", Element: "span",
			LocNote: "Tag chip shown in the sidebar list of filters.",
		},
	}
}

func fixtureShoppingCart() *klf.Block {
	return &klf.Block{
		ID: "shopping-cart-plural", Hash: "9QpZ11", Translatable: true,
		Type: klf.BlockTypeJSXElement,
		Source: []klf.Run{
			{Plural: &klf.PluralRun{
				Pivot: "count",
				Forms: map[klf.PluralForm][]klf.Run{
					klf.PluralZero:  {{Text: &klf.TextRun{Text: "Your cart is empty"}}},
					klf.PluralOne:   {{Text: &klf.TextRun{Text: "1 item in your cart"}}},
					klf.PluralOther: {
						{Ph: &klf.PlaceholderRun{
							ID: "1", Type: "jsx:var", SubType: "number",
							Data: "{count}", Equiv: "count", Disp: "count",
						}},
						{Text: &klf.TextRun{Text: " items in your cart"}},
					},
				},
			}},
		},
		Placeholders: []klf.Placeholder{
			{Name: "count", Kind: klf.PlaceholderICUPivot,
				SourceExpr: "items", JSType: "number"},
		},
		Properties: klf.BlockProperties{
			File: "src/ShoppingCart.tsx", Line: 4,
			Component: "ShoppingCart", JSXPath: "ShoppingCart > p > Plural", Element: "Plural",
		},
	}
}
