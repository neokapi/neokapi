package pdfreader

import (
	"testing"

	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"

	"github.com/neokapi/neokapi/core/docmeta"
	"github.com/neokapi/neokapi/core/model"
)

type fakeMeta struct{ vals map[string]string }

func (f fakeMeta) FPDF_GetMetaText(r *requests.FPDF_GetMetaText) (*responses.FPDF_GetMetaText, error) {
	return &responses.FPDF_GetMetaText{Tag: r.Tag, Value: f.vals[r.Tag]}, nil
}

// TestMetadataBlocks maps the PDF Info dictionary onto the content model:
// Title/Subject/Keywords become translatable metadata-plane blocks; the rest are
// "pdf:"-namespaced document-layer properties.
func TestMetadataBlocks(t *testing.T) {
	root := &model.Layer{ID: "doc1"}
	var ref references.FPDF_DOCUMENT
	fm := fakeMeta{vals: map[string]string{
		"Title":    "Annual Report",
		"Subject":  "Finance",
		"Keywords": "report, finance",
		"Author":   "Jane Doe",
		"Producer": "PDFlib",
		"ModDate":  "D:20240101000000Z",
		// Creator/CreationDate intentionally empty → skipped
	}}

	blocks := metadataBlocks(fm, ref, root)

	// Three translatable fields → three metadata blocks, all on the metadata plane.
	if len(blocks) != 3 {
		t.Fatalf("got %d metadata blocks, want 3 (title/subject/keywords)", len(blocks))
	}
	wantText := map[string]string{
		"pdf:title":    "Annual Report",
		"pdf:subject":  "Finance",
		"pdf:keywords": "report, finance",
	}
	for _, b := range blocks {
		if !b.Translatable {
			t.Errorf("block %s should be translatable", b.ID)
		}
		if st, ok := b.Structure(); !ok || st.Layer != model.LayerMetadata {
			t.Errorf("block %s should be on the metadata plane", b.ID)
		}
		key := b.Properties[docmeta.MetadataFieldProperty]
		if want, ok := wantText[key]; !ok || b.SourceText() != want {
			t.Errorf("block %s (%s) = %q, want %q", b.ID, key, b.SourceText(), want)
		}
	}
	if blocks[0].SemanticRole() != model.RoleTitle {
		t.Errorf("title block role = %q, want title", blocks[0].SemanticRole())
	}

	// Non-translatable fields → namespaced layer properties; empties absent.
	if root.Properties["pdf:author"] != "Jane Doe" {
		t.Errorf("pdf:author = %q", root.Properties["pdf:author"])
	}
	if root.Properties["pdf:producer"] != "PDFlib" {
		t.Errorf("pdf:producer = %q", root.Properties["pdf:producer"])
	}
	if _, ok := root.Properties["pdf:title"]; ok {
		t.Error("translatable title must be a block, not a property")
	}
	if _, ok := root.Properties["pdf:creator"]; ok {
		t.Error("empty creator should not be recorded")
	}
}
