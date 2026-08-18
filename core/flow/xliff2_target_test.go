package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2078. A `<unit>` with a `<source>` and no `<target>` is the normal shape of an
// extraction handed to a translation step, and it is exactly the shape that came
// back from a run unchanged: the skeleton write path substitutes into elements
// the reader saw, there was no `<target>` element to see, and the translation
// stayed in the store. Exit 0, file written, nothing in it — and a later merge
// reads the overlays and reports the work as done.
//
// The writer emits the element instead. XLIFF 2.0's content model makes
// `<target>` optional and puts it after `<source>` (xliff_core_2.0.xsd declares
// the pair as an xs:sequence), so both the file that arrives and the file that
// leaves are conforming documents; upstream Okapi's XLIFF 2 writer creates the
// element the same way, from `part.hasTarget()`, with no `xml:lang` on it
// (XLIFFWriter.writeFragment) because in XLIFF 2 the target language is the
// document's `trgLang`. The XLIFF 1.2 pair here has done this since it was
// written — its reader records a `target-inject` position for a `<trans-unit>`
// with no `<target>` — so this is the sibling format catching up.
func TestXLIFF2_ASourceOnlyDocumentComesBackTranslated(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		// wantAttr is the root attribute the output must carry. XLIFF 2.1's
		// Schematron (F1) requires trgLang if and only if the document holds
		// `<target>` children of `<segment>`/`<ignorable>`, so writing the first
		// target into a document that declares none obliges it.
		wantAttr string
	}{
		{
			name: "the document declares the language it is going to",
			doc: `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="qps">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>File</source>
      </segment>
    </unit>
    <unit id="2">
      <segment>
        <source>Edit</source>
      </segment>
    </unit>
    <unit id="3">
      <segment>
        <source>Help</source>
      </segment>
    </unit>
  </file>
</xliff>`,
			wantAttr: `trgLang="qps"`,
		},
		{
			name: "the document declares no target language at all",
			doc: `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>File</source>
      </segment>
    </unit>
    <unit id="2">
      <segment>
        <source>Edit</source>
      </segment>
    </unit>
    <unit id="3">
      <segment>
        <source>Help</source>
      </segment>
    </unit>
  </file>
</xliff>`,
			wantAttr: `trgLang="qps"`,
		},
		{
			name: "the document is not pretty-printed",
			doc: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="qps">` +
				`<file id="f1">` +
				`<unit id="1"><segment><source>File</source></segment></unit>` +
				`<unit id="2"><segment><source>Edit</source></segment></unit>` +
				`<unit id="3"><segment><source>Help</source></segment></unit>` +
				`</file></xliff>`,
			wantAttr: `trgLang="qps"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := runXLIFF2(t, tc.doc)
			for _, want := range []string{
				"<target>▒ Ƒîļé ▒</target>",
				"<target>▒ Éđîţ ▒</target>",
				"<target>▒ Ĥéļþ ▒</target>",
			} {
				assert.Contains(t, written, want,
					"a translation the run produced has to reach the file, not only the store")
			}
			assert.Contains(t, written, tc.wantAttr,
				"a document carrying targets has to declare the language they are in")
		})
	}
}

// TestXLIFF2_AnInjectedTargetKeepsTheDocumentsShape pins the whitespace: the
// element lands where a hand-written one would, so a file that goes through a
// run still reads as the file it was.
func TestXLIFF2_AnInjectedTargetKeepsTheDocumentsShape(t *testing.T) {
	written := runXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="qps">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>File</source>
      </segment>
    </unit>
  </file>
</xliff>`)
	assert.Equal(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="qps">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>File</source>
        <target>▒ Ƒîļé ▒</target>
      </segment>
    </unit>
  </file>
</xliff>`, written)
}

// TestXLIFF2_ATranslatedDocumentRunsAgainUnchanged closes the loop: what the
// injection wrote is an ordinary `<target>` on the way back in, so the second
// run substitutes into the element the first one created rather than adding
// another beside it.
func TestXLIFF2_ATranslatedDocumentRunsAgainUnchanged(t *testing.T) {
	first := runXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="qps">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>File</source>
      </segment>
    </unit>
  </file>
</xliff>`)
	assert.Equal(t, first, runXLIFF2(t, first))
}

// TestXLIFF2_ARunThatTranslatesNothingChangesNothing is the other half: the
// position the reader records is zero-width, so a pass-through — a flow with no
// translator, or a locale nothing was produced for — replays the source bytes
// and writes the document it read.
func TestXLIFF2_ARunThatTranslatesNothingChangesNothing(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>File</source>
      </segment>
    </unit>
  </file>
</xliff>`
	root, _, store, reg := overlayKeyFixture(t)
	src := filepath.Join(root, "src", "menu.xlf")
	require.NoError(t, os.WriteFile(src, []byte(doc), 0o644))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg: reg, SourceLocale: "en", Store: store, ProjectRoot: root,
	})
	out := filepath.Join(root, "out", "menu.xlf")
	// A tool that reads and produces nothing: the run still writes the file.
	require.NoError(t, runner.RunFile(context.Background(), "inspect-shaped",
		[]tool.Tool{&tool.BaseTool{ToolName: "no-op"}}, src, out, "qps"))

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, doc, string(written),
		"nothing was translated, so nothing about the document may change")
}

// TestXLIFF12_ASourceOnlyDocumentComesBackTranslated is #2078's sibling
// question: XLIFF 1.2 does not share the hole. Its reader records a
// `target-inject` position for a `<trans-unit>` that carries no `<target>` and
// its writer synthesizes the element there — which is where the XLIFF 2 pair's
// behaviour came from. This says so rather than leaving it to a reading.
func TestXLIFF12_ASourceOnlyDocumentComesBackTranslated(t *testing.T) {
	written := runXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
 <file source-language="en" target-language="qps" datatype="plaintext" original="menu">
  <body>
   <trans-unit id="1"><source>File</source></trans-unit>
   <trans-unit id="2"><source>Edit</source></trans-unit>
   <trans-unit id="3"><source>Help</source></trans-unit>
  </body>
 </file>
</xliff>`)
	for _, want := range []string{"▒ Ƒîļé ▒", "▒ Éđîţ ▒", "▒ Ĥéļþ ▒"} {
		assert.Contains(t, written, want)
	}
	assert.Equal(t, 3, strings.Count(written, "<target"),
		"one synthesized target per trans-unit that had none")
}

// runXLIFF2 pseudo-translates one XLIFF document through the runner a
// `kapi run` uses and returns what was written.
func runXLIFF2(t *testing.T, doc string) string {
	t.Helper()
	root, _, store, reg := overlayKeyFixture(t)
	src := filepath.Join(root, "src", "menu.xlf")
	require.NoError(t, os.WriteFile(src, []byte(doc), 0o644))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en",
		Store:        store,
		ProjectRoot:  root,
		// `.xlf` is claimed by both XLIFF versions; an empty answer defers to
		// the runner's own content-aware detection, which reads the file head.
		DetectFormat: func(string) registry.FormatID { return "" },
	})
	out := filepath.Join(root, "out", "menu.xlf")
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		pseudoTools(t, "qps"), src, out, "qps"))

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	return string(written)
}
