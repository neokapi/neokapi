//go:build acceptance

package xcstrings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcceptanceTranslatedOutputValidatesAgainstSchema writes a translated
// String Catalog and runs the REAL consumer-side validators against it:
//   - `jq .` confirms the output is well-formed JSON.
//   - `ajv validate` (a real `ajv` on PATH, else `npx --yes ajv-cli@5 validate`)
//     confirms it conforms to the de-facto xcstrings JSON Schema
//     (testdata/schema/xcstrings.schema.json).
//
// If a validator RUNS and rejects kapi's output, that is a spec-compliance bug
// in the writer and the test FAILs. Each external tool is gated on presence and
// SKIPPED (not failed) when it is unavailable, offline, or otherwise unable to
// execute (e.g. a non-executable npx-cached bin) — that is a tooling failure,
// not a kapi failure.
func TestAcceptanceTranslatedOutputValidatesAgainstSchema(t *testing.T) {
	jqPath, haveJq := lookTool("jq")
	// Prefer a real `ajv` on PATH (CI installs ajv-cli@5 globally); otherwise
	// provision it on demand via npx --yes ajv-cli@5.
	_, haveAjv := lookTool("ajv")
	_, haveNpx := lookTool("npx")
	haveAjvRunner := haveAjv || haveNpx

	if !haveJq && !haveAjvRunner {
		t.Skip("neither jq nor an ajv runner (ajv/npx) available")
	}

	// Produce a translated catalog for every fixture and every corpus file.
	inputs := fixtures()
	for i, n := range inputs {
		inputs[i] = filepath.Join("testdata", n)
	}
	corpus, _ := filepath.Glob(filepath.Join("testdata", "corpus", "*.xcstrings"))
	inputs = append(inputs, corpus...)

	schema := filepath.Join("testdata", "schema", "xcstrings.schema.json")

	for _, in := range inputs {
		t.Run(filepath.Base(in), func(t *testing.T) {
			out := translateXCStringsToFr(t, in)

			dir := t.TempDir()
			outPath := filepath.Join(dir, "translated.json")
			require.NoError(t, os.WriteFile(outPath, out, 0o644))

			if haveJq {
				runValidator(t, "jq", jqPath, []string{".", outPath})
			} else {
				t.Log("jq not available; skipped well-formedness check")
			}

			if haveAjvRunner {
				// Prefer a real `ajv` executable on PATH; fall back to npx
				// provisioning ajv-cli@5 on demand.
				name, prefix := ajvCommand()
				args := append(append([]string{}, prefix...),
					"validate", "-s", schema, "-d", outPath)
				cmd := exec.CommandContext(t.Context(), name, args...)
				combined, err := cmd.CombinedOutput()
				if err != nil && toolCouldNotRun(err, string(combined)) {
					t.Skipf("ajv could not run (tooling/environment, not a kapi failure): %s", combined)
				}
				assert.NoErrorf(t, err, "ajv must accept kapi's xcstrings output: %s", combined)
			} else {
				t.Log("ajv runner not available; skipped schema validation")
			}
		})
	}
}

// translateXCStringsToFr reads a catalog, sets a synthetic French translation on
// every leaf whose language is "fr" (or, if none, leaves it unchanged), and
// returns the writer output. The translation preserves placeholders.
func translateXCStringsToFr(t *testing.T, path string) []byte {
	t.Helper()
	parts, _ := readParts(t, path)

	srcLang := ""
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			srcLang = p.Resource.(*model.Layer).Properties["xcstrings.sourceLanguage"]
		}
	}
	targets := targetLocalesOf(testutil.FilterBlocks(parts), srcLang)
	tgt := "fr"
	for l := range targets {
		tgt = l
		break
	}

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Properties["xcstrings.lang"] != tgt {
			continue
		}
		lang := model.LocaleID(tgt)
		var nr []model.Run
		for _, r := range b.TargetRuns(lang) {
			if r.Text != nil {
				nr = append(nr, model.Run{Text: &model.TextRun{Text: "T:" + r.Text.Text}})
			} else {
				nr = append(nr, r)
			}
		}
		b.SetTargetRuns(lang, nr)
		b.Properties["state"] = "translated"
	}
	return writeParts(t, parts, model.LocaleID(tgt))
}
