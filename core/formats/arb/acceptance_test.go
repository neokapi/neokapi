//go:build acceptance

package arb_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// TestAcceptanceTranslatedOutputValidatesAgainstSchema writes a translated ARB
// document and runs the REAL consumer-side validators against it:
//   - `jq .` confirms the output is well-formed JSON.
//   - `npx ajv-cli@5 validate` confirms it conforms to the de-facto ARB JSON
//     Schema (testdata/schema/arb.schema.json), which captures the Flutter
//     gen_l10n / ARB structure (message keys, "@<id>" attribute objects,
//     "@@<global>" metadata).
//
// If a validator rejects kapi's output, that is a spec-compliance bug in the
// writer. Each external tool is gated on presence and skipped (not failed) when
// unavailable or offline.
func TestAcceptanceTranslatedOutputValidatesAgainstSchema(t *testing.T) {
	jqPath, haveJq := lookTool("jq")
	npxPath, haveNpx := lookTool("npx")
	if !haveJq && !haveNpx {
		t.Skip("neither jq nor npx available")
	}

	inputs := []string{
		filepath.Join("testdata", "simple_en.arb"),
		filepath.Join("testdata", "icu_en.arb"),
	}
	corpus, _ := filepath.Glob(filepath.Join("testdata", "corpus", "*.arb"))
	inputs = append(inputs, corpus...)

	schema := filepath.Join("testdata", "schema", "arb.schema.json")

	for _, in := range inputs {
		t.Run(filepath.Base(in), func(t *testing.T) {
			out := translateARBToFr(t, in)

			dir := t.TempDir()
			outPath := filepath.Join(dir, "translated.json")
			require.NoError(t, os.WriteFile(outPath, out, 0o644))

			if haveJq {
				cmd := exec.CommandContext(t.Context(), jqPath, ".", outPath)
				combined, err := cmd.CombinedOutput()
				require.NoErrorf(t, err, "jq must accept kapi's ARB output as well-formed JSON: %s", combined)
			} else {
				t.Log("jq not available; skipped well-formedness check")
			}

			if haveNpx {
				cmd := exec.CommandContext(t.Context(), npxPath, "--yes", "ajv-cli@5",
					"validate", "-s", schema, "-d", outPath)
				combined, err := cmd.CombinedOutput()
				if err != nil && isLikelyOffline(combined) {
					t.Skipf("ajv-cli unavailable (offline?): %s", combined)
				}
				require.NoErrorf(t, err, "ajv must accept kapi's ARB output: %s", combined)
			} else {
				t.Log("npx not available; skipped schema validation")
			}
		})
	}
}

// translateARBToFr reads an ARB file, sets a synthetic "fr" target on every
// message (preserving ICU placeholders), and returns the writer output written
// for locale "fr".
func translateARBToFr(t *testing.T, path string) []byte {
	t.Helper()
	parts, _ := readParts(t, path)
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		var nr []model.Run
		for _, r := range b.SourceRuns() {
			if r.Text != nil {
				nr = append(nr, model.Run{Text: &model.TextRun{Text: "T:" + r.Text.Text}})
			} else {
				nr = append(nr, r)
			}
		}
		b.SetTargetRuns("fr", nr)
	}
	return writeParts(t, parts, "fr")
}
