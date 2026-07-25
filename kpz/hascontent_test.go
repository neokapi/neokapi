package kpz

import (
	"testing"

	"github.com/neokapi/neokapi/memory/kmb"
)

func TestPackageHasContent(t *testing.T) {
	// Metadata-only packages are empty: nothing worth packing. Recipe, Sources
	// identity, InterchangeTask, and History are metadata, not content.
	empty := &Package{
		Kind:            KindProject,
		Sources:         []SourceIdentity{{SourcePath: "a.json"}},
		InterchangeTask: &InterchangeTask{SourceLocale: "en", TargetLocale: "fr"},
		History:         []byte("{}\n"),
	}
	if empty.HasContent() {
		t.Errorf("HasContent() = true for a metadata-only package, want false")
	}

	// An empty content-memory file is not content.
	if (&Package{Memory: &kmb.File{}}).HasContent() {
		t.Errorf("HasContent() = true for an empty content-memory file, want false")
	}

	// Any real content member makes it non-empty.
	cases := map[string]*Package{
		"blocks":    {Blocks: []BlockDoc{{Path: "blocks/x.kbf.json"}}},
		"overlays":  {Overlays: []OverlayDoc{{Kind: "targets/fr", Source: "a.json"}}},
		"skeletons": {Skeletons: []SkeletonDoc{{Path: "skeletons/x", SourcePath: "a.json"}}},
		"source":    {Source: []SourceDoc{{Path: "source/a.json", Content: BytesContent([]byte("{}"))}}},
		"tm":        {Memory: &kmb.File{Entries: make([]kmb.Entry, 1)}},
	}
	for name, pkg := range cases {
		if !pkg.HasContent() {
			t.Errorf("HasContent() = false for a package with %s, want true", name)
		}
	}
}
