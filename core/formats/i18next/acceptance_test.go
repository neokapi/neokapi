//go:build acceptance

// Package i18next acceptance tests run kapi's i18next writer output through a
// real downstream consumer to prove the output is still a well-formed i18next
// resource bundle that the broader toolchain will accept.
//
// Two gated consumers run here, both shelled out via os/exec (no new Go module
// deps):
//
//   - jq (well-formedness): the output must be parseable JSON.
//   - ajv-cli (structural validity): the output must validate against the
//     vendored structural i18next JSON Schema (testdata/schema/i18next.schema.json).
//
// i18next has no official JSON Schema (it is defined by library convention),
// so the vendored schema is a faithful de-facto structural schema: a recursive
// object of string | nested-object | array-of-string. Every external call is
// gated on exec.LookPath; the test t.Skips (not fails) when the tool is missing
// or the machine is offline (npx --yes provisions ajv-cli on demand).
//
// Run with: go test -tags acceptance ./core/formats/i18next/
package i18next_test

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

// lookPath reports the resolved path of bin, or skips the test if it is absent.
func lookPath(t *testing.T, bin string) string {
	t.Helper()
	p, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not on PATH; skipping consumer-acceptance check", bin)
	}
	return p
}

// runCmd runs name+args with a generous timeout and returns combined output and
// the run error. A nil error means the consumer accepted the input.
func runCmd(t *testing.T, dir, name string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return out, err
}

// writeI18nOutput reads a fixture, translates a couple of values to fr-FR, and
// writes the i18next output to a temp file, returning its path.
func writeI18nOutput(t *testing.T, fixture string, translate map[string]string) string {
	t.Helper()
	resolver := newResolver(t)
	parts, _ := readParts(t, filepath.Join("testdata", fixture), resolver)
	const fr = model.LocaleID("fr-FR")
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
	out := writeParts(t, parts, fr, resolver)

	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	require.NoError(t, os.WriteFile(path, out, 0o644))
	return path
}

// TestAcceptanceJQWellFormed verifies the translated i18next output is
// well-formed JSON, as judged by jq (the canonical JSON CLI). Gated on jq.
func TestAcceptanceJQWellFormed(t *testing.T) {
	jq := lookPath(t, "jq")

	fixtures := map[string]map[string]string{
		"plurals_v4_en.json": {
			"/key_one":   "{{count}} article",
			"/key_other": "{{count}} articles",
		},
		"namespaces_en.json": {
			"/home/title": "Bienvenue sur {{appName}}",
		},
		"context_en.json": {
			"/friend_male": "Un ami",
		},
	}
	for fixture, tr := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			out := writeI18nOutput(t, fixture, tr)
			res, err := runCmd(t, t.TempDir(), jq, ".", out)
			require.NoError(t, err, "jq rejected the i18next output as malformed JSON:\n%s", res)
		})
	}

	// Also assert every corpus file's untranslated round-trip output is
	// well-formed JSON.
	for _, path := range corpusFiles(t) {
		t.Run("corpus/"+filepath.Base(path), func(t *testing.T) {
			resolver := newResolver(t)
			parts, _ := readParts(t, path, resolver)
			out := writeParts(t, parts, "", resolver)
			tmp := filepath.Join(t.TempDir(), "out.json")
			require.NoError(t, os.WriteFile(tmp, out, 0o644))
			res, err := runCmd(t, t.TempDir(), jq, ".", tmp)
			require.NoError(t, err, "jq rejected corpus output as malformed JSON:\n%s", res)
		})
	}
}

// TestAcceptanceSchemaValid verifies the translated i18next output validates
// against the vendored structural i18next JSON Schema using ajv-cli. Gated on
// node/npx (and network, since npx --yes provisions ajv-cli on first run).
func TestAcceptanceSchemaValid(t *testing.T) {
	lookPath(t, "node")
	npx := lookPath(t, "npx")

	schema := filepath.Join("testdata", "schema", "i18next.schema.json")
	abs, err := filepath.Abs(schema)
	require.NoError(t, err)
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("vendored i18next schema missing: %v", err)
	}

	// Probe that ajv-cli can be provisioned; skip (offline) rather than fail.
	if _, err := runCmd(t, ".", npx, "--yes", "ajv-cli@5", "help"); err != nil {
		t.Skip("ajv-cli not provisionable (offline?); skipping schema validation")
	}

	validate := func(t *testing.T, outPath string) {
		t.Helper()
		res, err := runCmd(t, ".", npx, "--yes", "ajv-cli@5", "validate",
			"--strict=false", "-s", abs, "-d", outPath)
		// ajv prints "<file> valid" on success; surface output on failure.
		require.NoError(t, err,
			"ajv rejected the i18next output against the structural schema:\n%s", res)
		assert.Contains(t, string(res), "valid", "ajv should report the output as valid")
	}

	// Translated outputs.
	t.Run("plurals_translated", func(t *testing.T) {
		out := writeI18nOutput(t, "plurals_v4_en.json", map[string]string{
			"/key_one":          "{{count}} article",
			"/key_other":        "{{count}} articles",
			"/cart/items_zero":  "Votre panier est vide",
			"/cart/items_other": "{{count}} articles dans votre panier",
		})
		validate(t, out)
	})
	t.Run("context_translated", func(t *testing.T) {
		out := writeI18nOutput(t, "context_en.json", map[string]string{
			"/friend_male":   "Un ami",
			"/friend_female": "Une amie",
		})
		validate(t, out)
	})

	// Every corpus file's untranslated output.
	for _, path := range corpusFiles(t) {
		t.Run("corpus/"+filepath.Base(path), func(t *testing.T) {
			resolver := newResolver(t)
			parts, _ := readParts(t, path, resolver)
			out := writeParts(t, parts, "", resolver)
			tmp := filepath.Join(t.TempDir(), "out.json")
			require.NoError(t, os.WriteFile(tmp, out, 0o644))
			validate(t, tmp)
		})
	}
}
