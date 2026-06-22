package doclang_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/model"
)

// writeDocLang serializes a Part stream with the given config (nil = defaults).
func writeDocLang(t *testing.T, cfg *doclangfmt.Config, parts ...*model.Part) string {
	t.Helper()
	var buf bytes.Buffer
	w := doclangfmt.NewWriter()
	if cfg != nil {
		w.SetConfig(cfg)
	}
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatalf("SetOutputWriter: %v", err)
	}
	ch := make(chan *model.Part)
	go func() {
		for _, p := range parts {
			ch <- p
		}
		close(ch)
	}()
	if err := w.Write(context.Background(), ch); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.String()
}

func dlLayerStart() *model.Part {
	return &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{Locale: model.LocaleEnglish}}
}
func dlLayerEnd() *model.Part { return &model.Part{Type: model.PartLayerEnd, Resource: &model.Layer{}} }

func dlBlock(id, text, role string, level int) *model.Part {
	b := model.NewBlock(id, text)
	b.SetSemanticRole(role, level)
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func dlGroupStart(id, typ string) *model.Part {
	return &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: typ, Type: typ}}
}
func dlGroupEnd(id string) *model.Part {
	return &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}}
}

// TestDocLangIndentation verifies nested elements are indented two spaces per
// level (matching the canonical DocLang sample): top-level blocks at one level,
// list items at two.
func TestDocLangIndentation(t *testing.T) {
	out := writeDocLang(t, nil,
		dlLayerStart(),
		dlBlock("h", "Title", model.RoleHeading, 1),
		dlGroupStart("g", "list"),
		dlBlock("i1", "First", model.RoleListItem, 0),
		dlGroupEnd("g"),
		dlLayerEnd(),
	)
	for _, want := range []string{
		"\n  <heading level=\"1\">Title</heading>\n",
		"\n  <list class=\"unordered\">\n",
		"\n    <ldiv/><text>First</text>\n",
		"\n  </list>\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in indented output; got:\n%s", want, out)
		}
	}
}

// TestDocLangCompactOutput verifies CompactOutput suppresses indentation (block
// elements still newline-terminated, but flush-left).
func TestDocLangCompactOutput(t *testing.T) {
	cfg := &doclangfmt.Config{}
	cfg.Reset()
	cfg.CompactOutput = true
	out := writeDocLang(t, cfg,
		dlLayerStart(),
		dlGroupStart("g", "list"),
		dlBlock("i1", "First", model.RoleListItem, 0),
		dlGroupEnd("g"),
		dlLayerEnd(),
	)
	if strings.Contains(out, "  <list") || strings.Contains(out, "    <ldiv") {
		t.Errorf("compact mode should not indent; got:\n%s", out)
	}
	if !strings.Contains(out, "<list class=\"unordered\">\n") {
		t.Errorf("compact mode should keep newline-terminated elements; got:\n%s", out)
	}
}
