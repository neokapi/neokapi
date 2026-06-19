package docmeta

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func TestApply(t *testing.T) {
	layer := &model.Layer{ID: "doc1"}
	entries := []Entry{
		{Key: "pdf:title", Value: "Quarterly Report", Translatable: true, Role: model.RoleTitle},
		{Key: "pdf:subject", Value: "Sales", Translatable: true},
		{Key: "pdf:author", Value: "Jane Doe"},        // non-translatable → property
		{Key: "pdf:producer", Value: ""},              // empty → skipped entirely
		{Key: "pdf:keywords", Value: "", Translatable: true}, // empty → skipped
	}

	blocks := Apply(layer, entries, "meta")

	if len(blocks) != 2 {
		t.Fatalf("got %d metadata blocks, want 2", len(blocks))
	}
	if blocks[0].SourceText() != "Quarterly Report" || blocks[0].SemanticRole() != model.RoleTitle {
		t.Errorf("title block = %q/%q", blocks[0].SourceText(), blocks[0].SemanticRole())
	}
	for _, b := range blocks {
		if !b.Translatable {
			t.Errorf("metadata block %s should be translatable", b.ID)
		}
		if st, ok := b.Structure(); !ok || st.Layer != model.LayerMetadata {
			t.Errorf("block %s should be on the metadata plane", b.ID)
		}
		if b.Properties[MetadataFieldProperty] == "" {
			t.Errorf("block %s missing %s property", b.ID, MetadataFieldProperty)
		}
	}
	if blocks[1].Properties[MetadataFieldProperty] != "pdf:subject" {
		t.Errorf("subject field key = %q", blocks[1].Properties[MetadataFieldProperty])
	}

	// Non-translatable goes to layer properties; empty values are not recorded.
	if layer.Properties["pdf:author"] != "Jane Doe" {
		t.Errorf("author property = %q, want Jane Doe", layer.Properties["pdf:author"])
	}
	if _, ok := layer.Properties["pdf:producer"]; ok {
		t.Error("empty producer should not be recorded as a property")
	}
	if _, ok := layer.Properties["pdf:title"]; ok {
		t.Error("translatable title should be a block, not a property")
	}
}

func TestApply_NilLayer(t *testing.T) {
	if Apply(nil, []Entry{{Key: "k", Value: "v"}}, "meta") != nil {
		t.Error("nil layer should yield nil")
	}
}
