package flow_test

import (
	"context"
	encxml "encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runXMLWithSubfilters pseudo-translates an XML document whose named elements
// are delegated to an HTML sub-reader, through the runner a `kapi run` uses,
// and returns the file that was written.
func runXMLWithSubfilters(t *testing.T, source string, mappings []any) string {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.xml")
	require.NoError(t, os.WriteFile(inputPath, []byte(source), 0o644))
	outputPath := filepath.Join(dir, "out", "input.xml")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
		ConfigureReader: func(reader format.DataFormatReader, formatName registry.FormatID) error {
			if formatName != "xml" {
				return nil
			}
			return reader.Config().ApplyMap(map[string]any{"subfilters": mappings})
		},
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudoTool}, inputPath, outputPath, "qps"))

	out, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	return string(out)
}

// A delegated element's translated child layer reaches the file, and it goes
// back in the carrier it came out of. The two carriers are the two ways XML
// spells character data (XML 1.0 §2.4, §2.7), so neither is converted into the
// other: a CDATA section leaves as a CDATA section, and escaped text leaves
// escaped — which it must, or the markup the sub-reader handed back would
// close the element it is inside.
func TestFileRunner_XMLSubfilterChildLayerReachesTheFile(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		mappings     []any
		wantContains []string
		wantMissing  []string
	}{
		{
			name: "escaped character data comes back escaped",
			source: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<menu><a>&lt;p&gt;File&lt;/p&gt;</a><b>&lt;p&gt;Edit&lt;/p&gt;</b></menu>`,
			mappings: []any{
				map[string]any{"pattern": "menu.a", "format": "html"},
				map[string]any{"pattern": "menu.b", "format": "html"},
			},
			// The `<p>` the sub-reader was handed is text inside <a>, so it
			// returns as `&lt;p&gt;` — the document still has two children of
			// <menu> and no third element.
			wantContains: []string{"<a>&lt;p&gt;", "<b>&lt;p&gt;", "&lt;/p&gt;</a>"},
			// Raw markup would close <a> early; the source words are gone
			// because the translation reached the file rather than the store.
			wantMissing: []string{"<a><p>", "File", "Edit"},
		},
		{
			name: "a CDATA section comes back a CDATA section",
			source: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<menu><a><![CDATA[<p>File</p>]]></a><b><![CDATA[<p>Edit</p>]]></b></menu>`,
			mappings: []any{
				map[string]any{"pattern": "menu.a", "format": "html"},
				map[string]any{"pattern": "menu.b", "format": "html"},
			},
			wantContains: []string{"<a><![CDATA[<p>", "]]></a>", "<b><![CDATA[<p>"},
			wantMissing:  []string{"File", "Edit", "&lt;p&gt;"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := runXMLWithSubfilters(t, tt.source, tt.mappings)
			for _, want := range tt.wantContains {
				assert.Containsf(t, out, want, "output was %q", out)
			}
			for _, missing := range tt.wantMissing {
				assert.NotContainsf(t, out, missing, "output was %q", out)
			}
			// Each delegated element kept its own translation.
			assert.NotEqual(t, elementText(t, out, "a"), elementText(t, out, "b"),
				"the second delegated element came back holding the first's text")
		})
	}
}

// A run that translates nothing writes the document it read, delegation and
// all: the carriers survive because neither is rewritten as the other.
func TestFileRunner_XMLSubfilterUntranslatedIsByteExact(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	source := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<menu>` + "\n" +
		`  <a><![CDATA[<p>File</p>]]></a>` + "\n" +
		`  <b>&lt;p&gt;Edit&lt;/p&gt;</b>` + "\n" +
		`</menu>` + "\n"

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.xml")
	require.NoError(t, os.WriteFile(inputPath, []byte(source), 0o644))
	outputPath := filepath.Join(dir, "out", "input.xml")

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
		ConfigureReader: func(reader format.DataFormatReader, formatName registry.FormatID) error {
			if formatName != "xml" {
				return nil
			}
			return reader.Config().ApplyMap(map[string]any{"subfilters": []any{
				map[string]any{"pattern": "menu.a", "format": "html"},
				map[string]any{"pattern": "menu.b", "format": "html"},
			}})
		},
	})
	require.NoError(t, runner.RunFile(context.Background(), "noop",
		noopTools(), inputPath, outputPath, ""))

	out, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, source, string(out))
}

// elementText returns the character data of the first element with the given
// local name, read as text so a CDATA section and its escaped equivalent
// compare the same way.
func elementText(t *testing.T, xmlBody, local string) string {
	t.Helper()
	d := encxml.NewDecoder(strings.NewReader(xmlBody))
	for {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		se, ok := tok.(encxml.StartElement)
		if !ok || se.Name.Local != local {
			continue
		}
		var text string
		require.NoError(t, d.DecodeElement(&text, &se))
		return text
	}
}
