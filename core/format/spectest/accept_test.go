package spectest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// acceptTestSpecYAML is a minimal two-case spec written to a temp dir: one
// class: valid case with a file-backed blocks view, one class: invalid case
// with an error view. Loaded so the Spec carries a dir for fixture resolution.
const acceptTestSpecYAML = `format: okf_accepttest
mime_type: text/plain
features:
  - id: ok
    name: valid blocks case
    examples:
      - name: good
        id: TC01
        class: valid
        input_xml: "hello"
        cite: { spec: demo, url: "https://example.com#x" }
        expected:
          blocks: cases/TC01.events.jsonl
  - id: bad
    name: invalid case
    examples:
      - name: rejected
        id: TC02
        class: invalid
        input_xml: "garbage"
        cite: { spec: demo, url: "https://example.com#y" }
        expected:
          error: { category: syntax }
`

func loadAcceptSpec(t *testing.T) (*spec.Spec, spec.Example, spec.Example) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(path, []byte(acceptTestSpecYAML), 0o644); err != nil {
		t.Fatalf("write temp spec: %v", err)
	}
	s, err := spec.Load(path)
	if err != nil {
		t.Fatalf("load temp spec: %v", err)
	}
	var valid, invalid spec.Example
	for _, f := range s.Features {
		for _, ex := range f.Examples {
			switch ex.CaseID() {
			case "TC01":
				valid = ex
			case "TC02":
				invalid = ex
			}
		}
	}
	if valid.CaseID() != "TC01" || invalid.CaseID() != "TC02" {
		t.Fatalf("could not find the two cases in the loaded spec")
	}
	return s, valid, invalid
}

func sampleParts() []*model.Part {
	return []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc", Format: "plaintext"}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID: "b1", Translatable: true,
			Source: []model.Run{{Text: &model.TextRun{Text: "hello"}}},
		}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc"}},
	}
}

// TestAcceptMode_RegeneratesBlocksFixture proves accept-mode writes a
// file-backed expected.blocks fixture from the live dump, byte-for-byte equal
// to DumpBlockEvents.
func TestAcceptMode_RegeneratesBlocksFixture(t *testing.T) {
	s, valid, _ := loadAcceptSpec(t)
	parts := sampleParts()

	path, err := UpdateBlocksFixture(s, valid, parts)
	if err != nil {
		t.Fatalf("UpdateBlocksFixture: %v", err)
	}
	if filepath.Base(path) != "TC01.events.jsonl" {
		t.Errorf("unexpected fixture path: %s", path)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written fixture: %v", err)
	}
	want, err := spec.DumpBlockEvents(parts)
	if err != nil {
		t.Fatalf("DumpBlockEvents: %v", err)
	}
	if string(written) != string(want) {
		t.Errorf("written fixture != live dump\n--- written ---\n%s\n--- want ---\n%s", written, want)
	}
}

// TestAcceptMode_RefusesInvalidCase proves the tree-sitter guard rail:
// accept-mode never rewrites a class: invalid (error-class) case.
func TestAcceptMode_RefusesInvalidCase(t *testing.T) {
	s, _, invalid := loadAcceptSpec(t)
	parts := sampleParts()

	// RefuseAcceptForCase names the refusal directly.
	if err := RefuseAcceptForCase(invalid); err == nil {
		t.Fatal("RefuseAcceptForCase should refuse a class: invalid case")
	} else if !strings.Contains(err.Error(), "refuse") {
		t.Errorf("refusal message should mention 'refuse', got: %v", err)
	}

	// UpdateBlocksFixture must propagate that refusal and write nothing.
	if _, err := UpdateBlocksFixture(s, invalid, parts); err == nil {
		t.Fatal("UpdateBlocksFixture should refuse a class: invalid case")
	} else if !strings.Contains(err.Error(), "refuse") {
		t.Errorf("refusal message should mention 'refuse', got: %v", err)
	}
}

// TestAcceptMode_RefusesInlineBlocks proves inline JSONL fixtures are not
// auto-rewritable (only file-backed fixtures are).
func TestAcceptMode_RefusesInlineBlocks(t *testing.T) {
	s, valid, _ := loadAcceptSpec(t)
	valid.Expected = &spec.Expected{Blocks: `{"layer_start":{"id":"doc"}}`}
	if _, err := UpdateBlocksFixture(s, valid, sampleParts()); err == nil {
		t.Fatal("UpdateBlocksFixture should refuse an inline blocks fixture")
	}
}
