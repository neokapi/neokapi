package klf

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarshalDoesNotHTMLEscape asserts the wire-level "no HTML escaping"
// contract end to end: a document whose runs carry `<`, `>`, and `&` in their
// data (paired-code tags, logical-and JSX nodes) must serialize with those
// bytes literal — not `<` / `&` — so the Go output matches the
// TypeScript mirror's JSON.stringify output and the content hash is
// implementation-independent. Guards the regression where Run.MarshalJSON's
// inner encoder re-introduced HTML escaping.
func TestMarshalDoesNotHTMLEscape(t *testing.T) {
	data, err := Marshal(fixtureDocument())
	require.NoError(t, err)
	out := string(data)
	// The literal markup must be present, unescaped — filesHeading's <span>
	// paired code and tagChip's `&&` logical-and node. (JSON escapes the inner
	// double quotes as \", so match a quote-free slice.)
	assert.Contains(t, out, `<span className=`)
	assert.Contains(t, out, `index !== undefined &&`)
	// And none of it may appear in the \u-escaped form json.Marshal would emit.
	assert.NotContains(t, out, `u003c`, "must not escape '<' as \\u003c")
	assert.NotContains(t, out, `u003e`, "must not escape '>' as \\u003e")
	assert.NotContains(t, out, `u0026`, "must not escape '&' as \\u0026")
}

func TestRoundTripDocument(t *testing.T) {
	doc := fixtureDocument()
	buf, err := Marshal(doc)
	require.NoError(t, err)
	require.NotEmpty(t, buf)

	got, err := Unmarshal(buf)
	require.NoError(t, err)
	assert.Equal(t, doc, got, "document must round-trip through Marshal/Unmarshal without loss")
}

func TestRoundTripBlocks(t *testing.T) {
	// Each of the three reference blocks must round-trip byte-for-byte
	// on a second Marshal pass.
	for _, tc := range []struct {
		name  string
		block *Block
	}{
		{"files-heading", filesHeading()},
		{"tag-chip", tagChip()},
		{"shopping-cart", shoppingCart()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf1, err := MarshalBlock(tc.block)
			require.NoError(t, err)
			var decoded Block
			require.NoError(t, decode(buf1, &decoded))
			buf2, err := MarshalBlock(&decoded)
			require.NoError(t, err)
			assert.Equal(t, string(buf1), string(buf2), "second marshal pass must match first byte-for-byte")
		})
	}
}

func TestRejectsUnknownMajorVersion(t *testing.T) {
	data := []byte(`{"schemaVersion":"2.0","kind":"kapi-localization-format","generator":{"id":"x","version":"1"},"project":{"id":"p","sourceLocale":"en"},"documents":[]}`)
	_, err := Unmarshal(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported major")
}

func TestRejectsUnknownKind(t *testing.T) {
	data := []byte(`{"schemaVersion":"1.0","kind":"not-a-klf","generator":{"id":"x","version":"1"},"project":{"id":"p","sourceLocale":"en"},"documents":[]}`)
	_, err := Unmarshal(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected kind")
}

func TestRejectsMultipleDiscriminators(t *testing.T) {
	bad := []byte(`{"text":"hi","ph":{"id":"1","type":"jsx:var","data":"x","equiv":"x"}}`)
	var r Run
	err := r.UnmarshalJSON(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple discriminators")
}

func TestRejectsMissingDiscriminator(t *testing.T) {
	bad := []byte(`{}`)
	var r Run
	err := r.UnmarshalJSON(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no discriminator")
}

func TestRenderBlockHTML(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block *Block
		want  string
	}{
		{"files-heading", filesHeading(), filesHeadingExpectedHTML},
		{"tag-chip", tagChip(), tagChipExpectedHTML},
		{"shopping-cart", shoppingCart(), shoppingCartExpectedHTML},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderBlockHTML(tc.block, DefaultJSXVocabulary())
			assert.Equal(t, tc.want, got, "preview output must match the TS reference byte-for-byte")
		})
	}
}

func TestValidateBlock_FilesHeading(t *testing.T) {
	errs := ValidateBlock(filesHeading())
	assert.Empty(t, errs)
}

func TestValidateBlock_UnclosedPairedCode(t *testing.T) {
	b := filesHeading()
	// Drop the final pcClose run.
	b.Source = b.Source[:len(b.Source)-1]
	errs := ValidateBlock(b)
	require.Len(t, errs, 1)
	assert.Equal(t, ErrUnclosedPairedCode, errs[0].Kind)
}

func TestValidateBlock_UnmatchedCloseCode(t *testing.T) {
	b := filesHeading()
	// Remove the pcOpen but keep the pcClose.
	b.Source = append([]Run{b.Source[0]}, b.Source[2:]...)
	errs := ValidateBlock(b)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Kind == ErrUnmatchedCloseCode {
			found = true
		}
	}
	assert.True(t, found)
}

func TestValidateTargetAgainstSource_Passes(t *testing.T) {
	b := filesHeading()
	// A German target that preserves both required placeholders
	// ("muted" and "count").
	target := []Run{
		{Text: &TextRun{Text: "Dateien "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
			Data: `<span className="muted">`, Equiv: "muted"}},
		{Text: &TextRun{Text: "("}},
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
			Data: "{count}", Equiv: "count"}},
		{Text: &TextRun{Text: " passend)"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
			Data: "</span>", Equiv: "muted"}},
	}
	assert.Empty(t, ValidateTargetAgainstSource(b, target))
}

func TestValidateTargetAgainstSource_MissingPlaceholder(t *testing.T) {
	b := filesHeading()
	// Target that dropped the {count} variable — should flag.
	target := []Run{
		{Text: &TextRun{Text: "Dateien"}},
	}
	errs := ValidateTargetAgainstSource(b, target)
	require.Len(t, errs, 2) // muted + count
	kinds := map[string]bool{}
	for _, e := range errs {
		kinds[e.Placeholder] = true
	}
	assert.True(t, kinds["muted"])
	assert.True(t, kinds["count"])
}

func TestValidateTargetAgainstSource_OptionalDrops(t *testing.T) {
	b := tagChip()
	// A target that drops both optional jsx:node placeholders and
	// only carries the required 'label' variable is valid.
	target := []Run{
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "string",
			Data: "{label}", Equiv: "label"}},
	}
	assert.Empty(t, ValidateTargetAgainstSource(b, target))
}

func TestResolveAnchor_Block(t *testing.T) {
	b := filesHeading()
	res := ResolveAnchor(b, AnnotationAnchor{Kind: AnchorBlock, Block: "files-heading"})
	require.True(t, res.OK)
	assert.Equal(t, b, res.BlockTarget)
}

func TestResolveAnchor_BlockNotFound(t *testing.T) {
	b := filesHeading()
	res := ResolveAnchor(b, AnnotationAnchor{Kind: AnchorBlock, Block: "other"})
	require.False(t, res.OK)
	assert.Equal(t, ReasonBlockNotFound, res.Err)
}

func TestResolveAnchor_Run(t *testing.T) {
	b := tagChip()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind: AnchorRun, Block: "tag-chip",
		Path:  RunPath{{Kind: StepIndex, Index: 2}},
		RunID: "2",
	})
	require.True(t, res.OK)
	require.NotNil(t, res.RunTarget)
	require.NotNil(t, res.RunTarget.Ph)
	assert.Equal(t, "label", res.RunTarget.Ph.Equiv)
}

func TestResolveAnchor_RunIDMismatch(t *testing.T) {
	b := tagChip()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind: AnchorRun, Block: "tag-chip",
		Path:  RunPath{{Kind: StepIndex, Index: 2}},
		RunID: "mismatched",
	})
	require.False(t, res.OK)
	assert.Equal(t, ReasonRunIDMismatch, res.Err)
}

func TestResolveAnchor_PathOutOfBounds(t *testing.T) {
	b := filesHeading()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind: AnchorRun, Block: "files-heading",
		Path:  RunPath{{Kind: StepIndex, Index: 99}},
		RunID: "1",
	})
	require.False(t, res.OK)
	assert.Equal(t, ReasonPathOutOfBounds, res.Err)
}

func TestResolveAnchor_PathWrongKind(t *testing.T) {
	b := filesHeading()
	// Index 0 is a text run; run anchor should fail.
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind: AnchorRun, Block: "files-heading",
		Path:  RunPath{{Kind: StepIndex, Index: 0}},
		RunID: "x",
	})
	require.False(t, res.OK)
	assert.Equal(t, ReasonPathWrongKind, res.Err)
}

func TestResolveAnchor_RangeOK(t *testing.T) {
	b := filesHeading()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind:   AnchorRange,
		Block:  "files-heading",
		Path:   RunPath{{Kind: StepIndex, Index: 0}},
		Offset: 0,
		Length: 5,
	})
	require.True(t, res.OK)
	assert.Equal(t, "Files ", res.RangeText)
}

func TestResolveAnchor_RangeOutOfBounds(t *testing.T) {
	b := filesHeading()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind:   AnchorRange,
		Block:  "files-heading",
		Path:   RunPath{{Kind: StepIndex, Index: 0}},
		Offset: 0,
		Length: 999,
	})
	require.False(t, res.OK)
	assert.Equal(t, ReasonRangeOutOfBounds, res.Err)
}

func TestResolveAnchor_FormOK(t *testing.T) {
	b := shoppingCart()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind:  AnchorForm,
		Block: "shopping-cart-plural",
		Path:  RunPath{{Kind: StepIndex, Index: 0}},
		Key:   "other",
	})
	require.True(t, res.OK)
	require.Len(t, res.FormRuns, 2)
}

func TestResolveAnchor_FormNotFound(t *testing.T) {
	b := shoppingCart()
	res := ResolveAnchor(b, AnnotationAnchor{
		Kind:  AnchorForm,
		Block: "shopping-cart-plural",
		Path:  RunPath{{Kind: StepIndex, Index: 0}},
		Key:   "does-not-exist",
	})
	require.False(t, res.OK)
	assert.Equal(t, ReasonFormNotFound, res.Err)
}

func TestValidateAnchor_SuccessReturnsNil(t *testing.T) {
	b := filesHeading()
	ann := Annotation{Type: "annotation", ID: "a",
		Anchor: AnnotationAnchor{Kind: AnchorBlock, Block: "files-heading"}}
	assert.Nil(t, ValidateAnchor(b, ann))
}

func TestValidateAnchor_FailureReturnsError(t *testing.T) {
	b := filesHeading()
	ann := Annotation{Type: "annotation", ID: "a",
		Anchor: AnnotationAnchor{Kind: AnchorBlock, Block: "nonexistent"}}
	err := ValidateAnchor(b, ann)
	require.NotNil(t, err)
	assert.Equal(t, ReasonBlockNotFound, err.Reason)
}

func TestAnnotationFileRoundTrip(t *testing.T) {
	input := exampleAnnotationFile()
	var buf bytes.Buffer
	require.NoError(t, EncodeAnnotationFile(&buf, input))

	decoded, err := DecodeAnnotationFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, input.Header, decoded.Header)
	require.Len(t, decoded.Annotations, len(input.Annotations))
	for i := range input.Annotations {
		assert.Equal(t, input.Annotations[i].ID, decoded.Annotations[i].ID)
		assert.Equal(t, input.Annotations[i].Anchor, decoded.Annotations[i].Anchor)
	}
}

func TestAnnotationFileResolvesAgainstFixtures(t *testing.T) {
	// Mirror packages/kapi-format/examples/validate.ts: every annotation
	// in the example file must resolve against the example blocks.
	blocks := map[string]*Block{
		"files-heading":        filesHeading(),
		"tag-chip":             tagChip(),
		"shopping-cart-plural": shoppingCart(),
	}
	file := exampleAnnotationFile()
	for _, ann := range file.Annotations {
		b, ok := blocks[ann.Anchor.Block]
		require.Truef(t, ok, "annotation %q refers to unknown block %q", ann.ID, ann.Anchor.Block)
		res := ResolveAnchor(b, ann.Anchor)
		require.Truef(t, res.OK, "annotation %q failed to resolve: %s", ann.ID, res.Err)
	}
}

func TestAnnotationFileOrphanDetection(t *testing.T) {
	// An annotation whose block id doesn't match any known block
	// must surface via ValidateAnchor.
	b := filesHeading()
	orphan := Annotation{Type: "annotation", ID: "orphan",
		Anchor: AnnotationAnchor{Kind: AnchorBlock, Block: "gone"}}
	err := ValidateAnchor(b, orphan)
	require.NotNil(t, err)
	assert.Equal(t, ReasonBlockNotFound, err.Reason)
}

func TestOrderedPluralForms(t *testing.T) {
	// Sanity: the preview renderer walks plural forms in ICU order.
	m := map[PluralForm][]Run{
		PluralOther: nil, PluralZero: nil, PluralOne: nil,
	}
	got := orderedPluralForms(m)
	assert.Equal(t, []PluralForm{PluralZero, PluralOne, PluralOther}, got)
}

func TestExpandTemplateLeavesUnknownBracesAlone(t *testing.T) {
	out := expandTemplate("abc {unknown} def", map[string]string{})
	assert.Equal(t, "abc  def", out, "unknown keys expand to empty string (same as TS reference)")
}

// TestSkeletonRoundTrip pins the canonical Skeleton wire shape
// ({ ref, inline }) — issue #717 aligned the TS mirror
// (packages/kapi-format/src/block.ts) to this Go-canonical shape after
// it had drifted to { ref, digest }. The serialized field names here
// are what packages/kapi-format/tests/skeleton.test.ts asserts on the
// TS side, keeping the two implementations consistent.
func TestSkeletonRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		skel     *Skeleton
		wantJSON string // the document's "skeleton" object, or "" if omitted
	}{
		{
			name:     "ref and inline both present, in canonical order",
			skel:     &Skeleton{Ref: "skel://1", Inline: "<root>{0}</root>"},
			wantJSON: `"skeleton": {` + "\n" + `        "ref": "skel://1",` + "\n" + `        "inline": "<root>{0}</root>"` + "\n" + `      }`,
		},
		{
			name:     "ref only",
			skel:     &Skeleton{Ref: "skel://only"},
			wantJSON: `"skeleton": {` + "\n" + `        "ref": "skel://only"` + "\n" + `      }`,
		},
		{
			name:     "inline only",
			skel:     &Skeleton{Inline: "payload"},
			wantJSON: `"skeleton": {` + "\n" + `        "inline": "payload"` + "\n" + `      }`,
		},
		{
			name:     "omitted entirely when nil",
			skel:     nil,
			wantJSON: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file := &File{
				SchemaVersion: SchemaVersion,
				Kind:          Kind,
				Generator:     GeneratorInfo{ID: "test", Version: "1.0"},
				Project:       ProjectInfo{ID: "p", SourceLocale: "en"},
				Documents: []Document{{
					ID:           "doc1",
					DocumentType: DocumentTypeJSX,
					Path:         "src/App.tsx",
					Skeleton:     tc.skel,
					Blocks:       []Block{{ID: "b1", Translatable: true, Type: BlockTypeJSXElement, Source: []Run{{Text: &TextRun{Text: "Hello"}}}}},
				}},
			}

			buf, err := Marshal(file)
			require.NoError(t, err)
			out := string(buf)

			// The retired field name must never appear on the wire.
			assert.NotContains(t, out, "digest")

			if tc.wantJSON == "" {
				assert.NotContains(t, out, `"skeleton"`)
			} else {
				assert.Contains(t, out, tc.wantJSON)
			}

			// Round-trips back to the identical struct.
			got, err := Unmarshal(buf)
			require.NoError(t, err)
			assert.Equal(t, tc.skel, got.Documents[0].Skeleton)
		})
	}
}

func TestMarshalIsStableAcrossWrites(t *testing.T) {
	doc := fixtureDocument()
	a, err := Marshal(doc)
	require.NoError(t, err)
	b, err := Marshal(doc)
	require.NoError(t, err)
	assert.Equal(t, a, b, "Marshal must be deterministic for stable manifest hashing")
}

// exampleAnnotationFile mirrors
// packages/kapi-format/examples/annotations.ts — four records covering
// all four anchor kinds.
func exampleAnnotationFile() *AnnotationFile {
	return &AnnotationFile{
		Header: AnnotationFileHeader{
			Type:              "header",
			AnnotationType:    "@neokapi/example",
			AnnotationVersion: "1.0.0",
			Producer: AnnotationProducer{
				ID: "@neokapi/kapi-format-examples", Version: "0.0.1",
			},
			Created:       "2026-04-15T12:00:00Z",
			TargetArchive: "sha256:deadbeef",
		},
		Annotations: []Annotation{
			{
				Type: "annotation", ID: "review-1",
				Anchor: AnnotationAnchor{Kind: AnchorBlock, Block: "files-heading"},
				Data:   []byte(`{"kind":"review","locale":"de","status":"approved"}`),
			},
			{
				Type: "annotation", ID: "term-1",
				Anchor: AnnotationAnchor{
					Kind: AnchorRun, Block: "tag-chip",
					Path:  RunPath{{Kind: StepIndex, Index: 2}},
					RunID: "2",
				},
				Data: []byte(`{"kind":"protected-term","term":"label"}`),
			},
			{
				Type: "annotation", ID: "mt-1",
				Anchor: AnnotationAnchor{
					Kind: AnchorForm, Block: "shopping-cart-plural",
					Path: RunPath{{Kind: StepIndex, Index: 0}},
					Key:  "other",
				},
				Data: []byte(`{"kind":"mt-confidence","locale":"de","confidence":0.87}`),
			},
			{
				Type: "annotation", ID: "term-2",
				Anchor: AnnotationAnchor{
					Kind: AnchorRange, Block: "files-heading",
					Path:   RunPath{{Kind: StepIndex, Index: 0}},
					Offset: 0,
					Length: 5,
				},
				Data: []byte(`{"kind":"glossary-match","term":"Files"}`),
			},
		},
	}
}

func decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
