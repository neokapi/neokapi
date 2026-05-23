//go:build acceptance

// Package mdx acceptance tests run kapi's MDX writer output through the REAL
// @mdx-js/mdx compiler to prove the translated MDX is still valid MDX that the
// official toolchain accepts.
//
// The consumer is the genuine MDX compiler (`@mdx-js/mdx@3`), shelled out via
// os/exec (no new Go module deps). `compile()` performs full MDX parsing
// (remark + micromark MDX extensions + the MDX-to-JS transform) and throws on
// any syntax error, so a clean compile is strong evidence the output is valid
// MDX. It does NOT resolve component imports (those bind at bundle time), which
// is exactly what we want: we are validating MDX SYNTAX, not the docs runtime.
//
// Provisioning. The test creates a temp npm project and installs
// `@mdx-js/mdx@3` into it, then compiles each MDX file with a small ESM script.
// Every step is gated: the test t.Skips (not fails) when node/npx are missing
// or the install fails (offline). When it DOES run, a failure to compile is a
// real writer bug.
//
// Run with: go test -tags acceptance ./core/formats/mdx/
package mdx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// compileScript is the ESM driver that compiles an MDX file with the real
// compiler and prints a sentinel on success. It is written into the temp
// project so the dynamic import resolves against the installed package.
const compileScript = `import { readFileSync } from 'node:fs';
const file = process.argv[2];
const src = readFileSync(file, 'utf8');
const { compile } = await import('@mdx-js/mdx');
try {
  await compile(src);
  console.log('MDX_COMPILE_OK');
} catch (e) {
  console.error('MDX_COMPILE_FAIL: ' + (e && e.message ? e.message : e));
  process.exit(2);
}
`

func lookPath(t *testing.T, bin string) string {
	t.Helper()
	p, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not on PATH; skipping MDX compiler acceptance check", bin)
	}
	return p
}

func runCmd(t *testing.T, dir string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return out, err
}

// mdxCompiler provisions @mdx-js/mdx@3 into a temp npm project once per test and
// returns a closure that compiles an MDX file and reports (ok, output). The
// closure compiles via `node <script> <file>`; ok is true only when the real
// compiler accepts the file.
func mdxCompiler(t *testing.T) func(t *testing.T, mdxPath string) (bool, []byte) {
	t.Helper()
	node := lookPath(t, "node")
	npm := lookPath(t, "npm")

	proj := t.TempDir()
	// Minimal ESM package.json so the dynamic import of an ESM-only package works.
	require.NoError(t, os.WriteFile(filepath.Join(proj, "package.json"),
		[]byte(`{"name":"mdx-acceptance","version":"0.0.0","private":true,"type":"module"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "compile.mjs"), []byte(compileScript), 0o644))

	// Install the real MDX compiler. Skip (offline) rather than fail if it can't
	// be provisioned.
	if out, err := runCmd(t, proj, 300*time.Second, npm, "install", "--no-audit", "--no-fund", "@mdx-js/mdx@3"); err != nil {
		t.Skipf("could not provision @mdx-js/mdx@3 (offline?); skipping MDX compiler acceptance:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(proj, "node_modules", "@mdx-js", "mdx")); err != nil {
		t.Skip("@mdx-js/mdx not installed (offline?); skipping MDX compiler acceptance")
	}

	return func(t *testing.T, mdxPath string) (bool, []byte) {
		t.Helper()
		// The compiler runs from the temp project dir, so resolve to an absolute
		// path regardless of the caller's cwd.
		abs, err := filepath.Abs(mdxPath)
		require.NoError(t, err)
		out, err := runCmd(t, proj, 120*time.Second, node, "compile.mjs", abs)
		return err == nil && strings.Contains(string(out), "MDX_COMPILE_OK"), out
	}
}

// TestAcceptanceTranslatedMDXCompiles is the headline consumer check: for every
// real corpus MDX file, kapi pseudo-translates the prose, writes the output,
// and the REAL @mdx-js/mdx compiler must accept that translated output. This
// proves the translated MDX is still valid MDX.
func TestAcceptanceTranslatedMDXCompiles(t *testing.T) {
	compile := mdxCompiler(t)

	const fr = model.LocaleID("fr-FR")
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err)

			// Sanity: the SOURCE compiles (precondition for a meaningful check).
			srcOK, srcOut := compile(t, path)
			require.True(t, srcOK, "precondition: source MDX must compile:\n%s", srcOut)

			// Pseudo-translate the prose and write the output.
			parts, store := readParts(t, src)
			for _, p := range parts {
				if p.Type == model.PartBlock {
					b := p.Resource.(*model.Block)
					if strings.ContainsAny(b.Source[0].Text(), "abcdefghijklmnopqrstuvwxyz") {
						pseudoTranslate(b, fr)
					}
				}
			}
			out := writeParts(t, parts, store, fr)

			outPath := filepath.Join(t.TempDir(), "translated.mdx")
			require.NoError(t, os.WriteFile(outPath, out, 0o644))

			ok, res := compile(t, outPath)
			require.True(t, ok,
				"the real @mdx-js/mdx compiler rejected kapi's translated MDX output for %s:\n%s",
				filepath.Base(path), res)
		})
	}
}

// TestAcceptanceUntranslatedRoundTripCompiles verifies the untranslated
// read→write output (byte-identical to source) still compiles — a fast,
// independent guard that the writer never emits compiler-breaking bytes even
// without translation.
func TestAcceptanceUntranslatedRoundTripCompiles(t *testing.T) {
	compile := mdxCompiler(t)
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err)
			out := roundTrip(t, src)
			outPath := filepath.Join(t.TempDir(), "roundtrip.mdx")
			require.NoError(t, os.WriteFile(outPath, out, 0o644))
			ok, res := compile(t, outPath)
			require.True(t, ok,
				"the real @mdx-js/mdx compiler rejected kapi's round-trip MDX output for %s:\n%s",
				filepath.Base(path), res)
		})
	}
}
