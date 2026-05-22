package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile writes content to dir/rel, creating parent dirs.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func TestCollectBridge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "manifest.json", `{
	  "name": "okapi", "version": "1.0.0",
	  "capabilities": [
	    {"type":"format","id":"okf_demo","display_name":"Demo","capabilities":["read","write"],
	     "mime_types":["text/x"],"extensions":[".x"],
	     "schema":"formats/okf_demo/schema.json","doc":"formats/okf_demo/doc.json","presets_dir":"formats/okf_demo/presets/"},
	    {"type":"tool","id":"demo-tool","display_name":"Demo Tool","description":"Does a demo.",
	     "category":"transform","inputs":["block"],"schema":"tools/demo-tool/schema.json"}
	  ]
	}`)
	writeFile(t, dir, "formats/okf_demo/schema.json", `{"title":"Demo Format","description":"A demo format.","type":"object","properties":{"p1":{"type":"string","description":"param one"}}}`)
	writeFile(t, dir, "formats/okf_demo/doc.json", `{"overview":"Demo overview.","parameters":{"p1":{"help":"help one","values":"any"}},"examples":[{"title":"e1","config":"p1: x"}],"propertySuggestions":{"p1":{"title":"P1","description":"desc one"}}}`)
	writeFile(t, dir, "formats/okf_demo/presets/default.json", `{"p1":"x"}`)
	writeFile(t, dir, "tools/demo-tool/schema.json", `{"title":"Demo Tool","type":"object","properties":{}}`)

	formats, tools, err := collectBridge(dir)
	require.NoError(t, err)
	require.Len(t, formats, 1)
	require.Len(t, tools, 1)

	f := formats[0]
	assert.Equal(t, "okf_demo", f.ID)
	assert.Equal(t, SourceOkapi, f.Source)
	assert.Equal(t, KindFormat, f.Kind)
	assert.True(t, f.HasReader)
	assert.True(t, f.HasWriter)
	assert.Equal(t, []string{".x"}, f.Extensions)
	assert.Equal(t, "A demo format.", f.Description, "description falls back to schema title/description")
	require.NotNil(t, f.Doc)
	assert.Equal(t, "Demo overview.", f.Doc.Overview)
	assert.Equal(t, "help one", f.Doc.Parameters["p1"].Help)
	assert.Equal(t, "desc one", f.Doc.Parameters["p1"].Description, "propertySuggestions fold into param description")
	require.Len(t, f.Doc.Examples, 1)
	assert.Equal(t, "p1: x", f.Doc.Examples[0].Config)
	assert.Equal(t, "x", f.Presets["default"]["p1"])

	tl := tools[0]
	assert.Equal(t, "demo-tool", tl.ID)
	assert.Equal(t, "Does a demo.", tl.Description)
	assert.Equal(t, "transform", tl.Category)
}

func TestCollectBridgeMissingDir(t *testing.T) {
	_, _, err := collectBridge(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestApplyNativeDoc(t *testing.T) {
	e := Entry{ID: "json", Source: SourceBuiltIn, Kind: KindFormat, DisplayName: "JSON"}
	ndf := &nativeDocFile{
		DisplayName: "JSON (overridden)",
		Description: "JavaScript Object Notation.",
		Overview:    "The JSON format extracts string values.",
		Parameters: map[string]nativeDocParam{
			"extractAllPairs": {Help: "Extract every pair.", Values: "true/false", DependsOn: []nativeDocDepends{{Property: "exceptions", Condition: "ignored when set"}}},
		},
		Examples: []nativeDocExample{{Title: "Basic", Config: "extractAllPairs: true"}},
	}
	applyNativeDoc(&e, ndf)

	assert.Equal(t, "JSON (overridden)", e.DisplayName)
	assert.Equal(t, "JavaScript Object Notation.", e.Description)
	require.NotNil(t, e.Doc)
	assert.Equal(t, "The JSON format extracts string values.", e.Doc.Overview)
	require.Contains(t, e.Doc.Parameters, "extractAllPairs")
	assert.Equal(t, "Extract every pair.", e.Doc.Parameters["extractAllPairs"].Help)
	require.Len(t, e.Doc.Parameters["extractAllPairs"].DependsOn, 1)
	assert.Equal(t, "exceptions", e.Doc.Parameters["extractAllPairs"].DependsOn[0].Property)
	require.Len(t, e.Doc.Examples, 1)
	assert.Equal(t, "extractAllPairs: true", e.Doc.Examples[0].Config)
}

func TestApplyNativeDocEmptyKeepsNilDoc(t *testing.T) {
	e := Entry{ID: "plaintext", DisplayName: "Plain Text"}
	applyNativeDoc(&e, &nativeDocFile{})
	assert.Nil(t, e.Doc, "an empty sidecar must not attach an empty doc")
}

func TestDetectGaps(t *testing.T) {
	withProp := json.RawMessage(`{"properties":{"x":{"type":"string"}}}`)
	withDescribedProp := json.RawMessage(`{"properties":{"x":{"type":"string","description":"set"}}}`)

	entries := []Entry{
		{Kind: KindFormat, Source: SourceBuiltIn, ID: "bare", Schema: withProp}, // missing desc, overview, prop desc
		{Kind: KindTool, Source: SourceOkapi, ID: "complete", Description: "d",
			Doc:    &Doc{Overview: "o", Examples: []DocExample{{Title: "e"}}},
			Schema: withDescribedProp},
	}
	gaps := detectGaps(entries)

	fields := map[string]int{}
	for _, g := range gaps {
		if g.ID == "bare" {
			fields[g.Field]++
		}
		assert.NotEqual(t, "complete", g.ID, "fully documented entry must have no gaps")
	}
	assert.Equal(t, 1, fields["description"])
	assert.Equal(t, 1, fields["doc.overview"])
	assert.Equal(t, 1, fields["property:x"])

	sum := summarize(gaps)
	assert.Equal(t, len(gaps), sum["total"])
}
