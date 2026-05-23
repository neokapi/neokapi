//go:build acceptance

// Package designtokens acceptance tests run kapi's DTCG writer output through a
// real downstream consumer to prove the output is still a well-formed,
// schema-valid design-tokens document.
//
// Two gated consumers run here, both shelled out via os/exec (no new Go module
// deps):
//
//   - jq (well-formedness): the output must be parseable JSON.
//   - ajv-cli against the OFFICIAL W3C DTCG JSON Schema (the 2025.10 stable
//     revision, vendored verbatim under testdata/schema/dtcg/2025.10/).
//
// Schema-validity contract. The official DTCG schema is faithful to the spec
// but, like any JSON Schema, it cannot model the DTCG $type cascade (a group's
// $type inherited by leaf tokens). Tokens that omit an explicit $type and carry
// a bare ambiguous string value (e.g. a fontWeight keyword such as "thin") fall
// into the schema's catch-all branch and are rejected — even though the file is
// valid DTCG once cascade is resolved. This is a documented schema limitation,
// not a kapi defect. The acceptance contract is therefore EQUIVALENCE: kapi's
// output must validate against the official schema if and only if the SOURCE
// did. kapi must never turn a schema-valid token file into an invalid one (and
// vice versa). The translated $description path is exercised on a fixture that
// the official schema accepts at the source, so a positive validation is also
// asserted there.
//
// Every external call is gated on exec.LookPath; the test t.Skips (not fails)
// when the tool is missing or the machine is offline (npx --yes provisions
// ajv-cli on demand).
//
// Run with: go test -tags acceptance ./core/formats/designtokens/
package designtokens_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lookPath(t *testing.T, bin string) string {
	t.Helper()
	p, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not on PATH; skipping consumer-acceptance check", bin)
	}
	return p
}

func runCmd(t *testing.T, dir, name string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return out, err
}

// dtcgSchemaArgs returns the ajv-cli args that load the vendored official DTCG
// schema set (root + all referenced sub-schemas), relative to the package dir.
func dtcgSchemaArgs(t *testing.T) []string {
	t.Helper()
	base := filepath.Join("testdata", "schema", "dtcg", "2025.10")
	root := filepath.Join(base, "format.json")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("vendored DTCG schema missing: %v", err)
	}
	return []string{
		"--strict=false",
		"-s", root,
		"-r", filepath.Join(base, "format", "*.json"),
		"-r", filepath.Join(base, "format", "values", "*.json"),
	}
}

// ajvValidates reports whether ajv-cli accepts data against the official DTCG
// schema. The boolean is the validation result; combined output is returned for
// diagnostics. ajv exits non-zero on invalid data, which is a normal result
// here (not a harness error).
func ajvValidates(t *testing.T, npx, dataPath string) (bool, []byte) {
	t.Helper()
	args := append([]string{"--yes", "ajv-cli@5", "validate"}, dtcgSchemaArgs(t)...)
	args = append(args, "-d", dataPath)
	out, err := runCmd(t, ".", npx, args...)
	return err == nil, out
}

// writeDTOutput reads a fixture, optionally translates $description values to
// fr-FR, writes the DTCG output, and returns the temp output path.
func writeDTOutput(t *testing.T, fixture string, translate map[string]string) string {
	t.Helper()
	parts, _ := readParts(t, filepath.Join("testdata", fixture))
	const fr = model.LocaleID("fr-FR")
	locale := model.LocaleID("")
	if len(translate) > 0 {
		locale = fr
		for _, p := range parts {
			if p.Type != model.PartBlock {
				continue
			}
			b, ok := p.Resource.(*model.Block)
			if !ok {
				continue
			}
			if tr, ok := translate[b.Name]; ok {
				b.SetTargetText(fr, tr)
			}
		}
	}
	out := writeParts(t, parts, locale)
	path := filepath.Join(t.TempDir(), "out.tokens.json")
	require.NoError(t, os.WriteFile(path, out, 0o644))
	return path
}

// TestAcceptanceJQWellFormed verifies the DTCG output (translated and
// untranslated corpus) is well-formed JSON per jq. Gated on jq.
func TestAcceptanceJQWellFormed(t *testing.T) {
	jq := lookPath(t, "jq")

	t.Run("translated", func(t *testing.T) {
		out := writeDTOutput(t, "tokens.tokens.json", map[string]string{
			"/color/primary/$description": "Couleur de marque principale.",
			"/spacing/small/$description": "Espacement serré.",
		})
		res, err := runCmd(t, t.TempDir(), jq, ".", out)
		require.NoError(t, err, "jq rejected the DTCG output as malformed JSON:\n%s", res)
	})

	for _, path := range corpusFiles(t) {
		t.Run("corpus/"+filepath.Base(path), func(t *testing.T) {
			parts, _ := readParts(t, path)
			out := writeParts(t, parts, "")
			tmp := filepath.Join(t.TempDir(), "out.tokens.json")
			require.NoError(t, os.WriteFile(tmp, out, 0o644))
			res, err := runCmd(t, t.TempDir(), jq, ".", tmp)
			require.NoError(t, err, "jq rejected corpus DTCG output as malformed JSON:\n%s", res)
		})
	}
}

// TestAcceptanceSchemaEquivalence validates kapi's output against the OFFICIAL
// W3C DTCG JSON Schema and asserts kapi never changes a file's schema-validity.
// It also asserts a positive validation on a fixture the official schema
// accepts, after translating its $description prose. Gated on node/npx (and
// network for npx --yes provisioning).
func TestAcceptanceSchemaEquivalence(t *testing.T) {
	lookPath(t, "node")
	npx := lookPath(t, "npx")

	// Probe that ajv-cli can be provisioned; skip (offline) rather than fail.
	if _, err := runCmd(t, ".", npx, "--yes", "ajv-cli@5", "help"); err != nil {
		t.Skip("ajv-cli not provisionable (offline?); skipping schema validation")
	}

	// Positive path: the in-package tokens.tokens.json validates against the
	// official schema at the source (its cascade tokens use unambiguous values),
	// so its translated output MUST validate too — proving kapi's writer does not
	// break schema-validity while translating $description.
	t.Run("translated_fixture_validates", func(t *testing.T) {
		srcOK, srcOut := ajvValidates(t, npx, filepath.Join("testdata", "tokens.tokens.json"))
		require.True(t, srcOK,
			"precondition: in-package fixture must validate against the official DTCG schema:\n%s", srcOut)

		out := writeDTOutput(t, "tokens.tokens.json", map[string]string{
			"/color/primary/$description":     "Couleur de marque principale pour les appels à l'action.",
			"/button/background/$description": "La surface du bouton.",
		})
		outOK, outRes := ajvValidates(t, npx, out)
		assert.True(t, outOK,
			"translated DTCG output must remain valid against the official DTCG schema:\n%s", outRes)
	})

	// Equivalence path: across the real corpus, kapi's untranslated output must
	// validate against the official schema IFF the source did. (The
	// style-dictionary demo file uses $type cascade with bare fontWeight
	// keywords, which the official schema cannot resolve — so source and output
	// both fail, equivalently; this documents the schema limitation, not a bug.)
	for _, path := range corpusFiles(t) {
		t.Run("corpus_equivalence/"+filepath.Base(path), func(t *testing.T) {
			srcOK, _ := ajvValidates(t, npx, path)

			parts, _ := readParts(t, path)
			out := writeParts(t, parts, "")
			tmp := filepath.Join(t.TempDir(), "out.tokens.json")
			require.NoError(t, os.WriteFile(tmp, out, 0o644))
			outOK, outRes := ajvValidates(t, npx, tmp)

			assert.Equal(t, srcOK, outOK,
				"kapi must not change a DTCG file's schema-validity (source valid=%v, output valid=%v):\n%s",
				srcOK, outOK, outRes)
			if !srcOK {
				t.Logf("note: %s does not validate against the official DTCG schema at the source "+
					"(known $type-cascade limitation of JSON Schema); kapi output matches that result",
					filepath.Base(path))
			}
		})
	}
}
