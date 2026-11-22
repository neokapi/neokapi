package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
)

// inspectRecord is the part of a `kapi inspect` record these tests read.
type inspectRecord struct {
	Ref struct {
		Doc   string `json:"doc" yaml:"doc"`
		Block string `json:"block" yaml:"block"`
	} `json:"ref" yaml:"ref"`
	Rev   string `json:"rev" yaml:"rev"`
	Text  string `json:"text" yaml:"text"`
	Role  string `json:"role" yaml:"role"`
	Level int    `json:"level" yaml:"level"`
}

// runInspectFixture writes content to a temp file of the given name and runs
// `kapi inspect` over it, returning captured stdout.
func runInspectFixture(t *testing.T, name, content string, args ...string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	app := newAppForTest(t)
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	// Parent it under a root carrying the persistent flags, because the shared
	// output-format axis lives there — the same tree the user drives.
	root := &cobra.Command{Use: "kapi"}
	AddPersistentFlags(app, root)
	AddCommandGroups(app, root)
	root.AddCommand(NewInspectCmd(app))

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"inspect"}, append(args, path)...))
	require.NoError(t, root.Execute())
	return out.String()
}

// Each record carries the reference an edit copies into "at" and the
// revision it sends as if_match.
func TestInspect_RecordsCarryReferencesAndRevisions(t *testing.T) {
	out := runInspectFixture(t, "en.json", `{"greeting":"Hello","farewell":"Bye"}`)

	var blocks []inspectRecord
	require.NoError(t, json.Unmarshal([]byte(out), &blocks))
	require.Len(t, blocks, 2)
	keys := map[string]string{}
	for _, b := range blocks {
		assert.True(t, strings.HasSuffix(b.Ref.Doc, "en.json"), b.Ref.Doc)
		assert.Regexp(t, `^r:[0-9a-f]{16}$`, b.Rev)
		keys[b.Ref.Block] = b.Text
	}
	assert.Equal(t, map[string]string{"greeting": "Hello", "farewell": "Bye"}, keys)
}

func TestInspect_JSONLStreamsOnePerLine(t *testing.T) {
	out := runInspectFixture(t, "en.json", `{"a":"one","b":"two","c":"three"}`, "--jsonl")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3, "three blocks => three JSONL lines")
	for _, ln := range lines {
		var b inspectRecord
		require.NoError(t, json.Unmarshal([]byte(ln), &b), "each line is a JSON object")
		assert.NotEmpty(t, b.Rev)
		assert.NotEmpty(t, b.Text)
	}
}

// TestInspect_YAMLSequence pins inspect to the shared format axis: YAML is
// requested the same way as for every other command, not with a private flag,
// and carries the same record.
func TestInspect_YAMLSequence(t *testing.T) {
	out := runInspectFixture(t, "en.json", `{"greeting":"Hello","farewell":"Bye"}`, "--output-format", "yaml")

	var blocks []inspectRecord
	require.NoError(t, yamlv3.Unmarshal([]byte(out), &blocks), out)
	require.Len(t, blocks, 2)
	for _, b := range blocks {
		assert.NotEmpty(t, b.Text)
		assert.Regexp(t, `^r:[0-9a-f]{16}$`, b.Rev)
		assert.NotEmpty(t, b.Ref.Block)
	}
}

// Markdown carries structural roles, so a heading block reports its role.
func TestInspect_StructuralRole(t *testing.T) {
	out := runInspectFixture(t, "page.md", "# Title\n\nA paragraph.\n")

	var blocks []inspectRecord
	require.NoError(t, json.Unmarshal([]byte(out), &blocks))

	var heading *inspectRecord
	for i := range blocks {
		if blocks[i].Text == "Title" {
			heading = &blocks[i]
		}
	}
	require.NotNil(t, heading, "heading block not found")
	assert.Equal(t, model.RoleHeading, heading.Role)
	assert.Equal(t, 1, heading.Level)
}

// A cell in a hidden spreadsheet column reports its visibility, so a reader
// of the records can tell text no one sees from text on the sheet.
func TestInspect_ReportsHiddenContent(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entry := func(name, body string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	const rel = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
	entry("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`)
	entry("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="`+rel+`officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	entry("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="`+strings.TrimSuffix(rel, "/")+`"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	entry("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="`+rel+`worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	entry("xl/worksheets/sheet1.xml", `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><cols><col min="2" max="2" hidden="1"/></cols><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Shown</t></is></c><c r="B1" t="inlineStr"><is><t>Not shown</t></is></c></row></sheetData></worksheet>`)
	require.NoError(t, zw.Close())

	out := runInspectFixture(t, "book.xlsx", buf.String())
	var blocks []struct {
		Text       string `json:"text"`
		Visibility string `json:"visibility"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &blocks), out)
	vis := map[string]string{}
	for _, b := range blocks {
		vis[b.Text] = b.Visibility
	}
	require.Contains(t, vis, "Shown")
	require.Contains(t, vis, "Not shown")
	assert.Empty(t, vis["Shown"])
	assert.Equal(t, model.VisibilityHidden, vis["Not shown"])
}

// A read of several files names each block's document in its reference: the
// path a change set names it by.
func TestInspect_ReferencesNameTheirFile(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	app := newAppForTest(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	require.NoError(t, os.WriteFile(a, []byte(`{"x":"one"}`), 0o644))
	require.NoError(t, os.WriteFile(b, []byte(`{"y":"two","z":"three"}`), 0o644))

	cmd := NewInspectCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{a, b})
	require.NoError(t, cmd.Execute())

	var blocks []inspectRecord
	require.NoError(t, json.Unmarshal(out.Bytes(), &blocks))
	require.Len(t, blocks, 3)
	assert.Equal(t, filepath.ToSlash(a), blocks[0].Ref.Doc)
	assert.Equal(t, filepath.ToSlash(b), blocks[1].Ref.Doc)
	assert.Equal(t, filepath.ToSlash(b), blocks[2].Ref.Doc)
}

func TestInspect_RenderRendersBlocksPerFormat(t *testing.T) {
	out := runInspectFixture(t, "doc.md", "# Title\n\nSome **bold** text.\n", "--render", "html,markdown")

	var recs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &recs))
	require.NotEmpty(t, recs)

	// Find the paragraph record and check its projected fragments carry faithful
	// markup (not the flattened Text anchor).
	var para map[string]any
	for _, r := range recs {
		if proj, ok := r["projected"].(map[string]any); ok {
			if h, _ := proj["html"].(string); strings.Contains(h, "<strong>") {
				para = r
			}
		}
	}
	require.NotNil(t, para, "paragraph projection not found")
	proj := para["projected"].(map[string]any)
	assert.Equal(t, "<p>Some <strong>bold</strong> text.</p>", proj["html"])
	assert.Equal(t, "Some **bold** text.", proj["markdown"])
}

func TestInspect_RenderRejectsUnknownFormat(t *testing.T) {
	app := newAppForTest(t)
	cmd := NewInspectCmd(app)
	cmd.SetArgs([]string{"--render", "pdf", "-"})
	cmd.SetIn(strings.NewReader("x"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}
