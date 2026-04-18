package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExtractorScript writes a shell script that reads NUL-separated
// paths from stdin and emits one fake NDJSON block per path. Used as
// the `exec:` target in tests — avoids needing a real kapi-react.
func fakeExtractorScript(t *testing.T, dir, name string) string {
	t.Helper()
	// The script reads stdin, splits on NUL, and emits one block per
	// path with a deterministic hash derived from the path length.
	script := `#!/bin/sh
set -e
n=0
while IFS= read -r -d '' path; do
  printf '{"type":"block","document":"%s","block":{"id":"b%d","hash":"h-%s","translatable":true,"type":"jsx:element","source":[{"text":"Hello from %s"}]}}\n' "$path" "$n" "$(echo "$path" | wc -c | tr -d ' ')" "$path"
  n=$((n + 1))
done
`
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func writeExtractProject(t *testing.T, dir string, kapi string) string {
	t.Helper()
	p := filepath.Join(dir, "project.kapi")
	require.NoError(t, os.WriteFile(p, []byte(kapi), 0o644))
	return p
}

func TestPlanExtractHonoursExplicitExtractor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "App.tsx"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Button.tsx"), []byte("x"), 0o644))

	bin := fakeExtractorScript(t, dir, "fake-extract.sh")
	proj := writeExtractProject(t, dir, `
version: v1
content:
  - name: ui
    archive: i18n/ui.klz
    extractor:
      exec: ["`+bin+`"]
    items:
      - path: "src/**/*.tsx"
`)

	plans, err := PlanExtract(proj)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "ui", plans[0].Collection)
	assert.Equal(t, "i18n/ui.klz", plans[0].Archive)
	assert.ElementsMatch(t,
		[]string{"src/App.tsx", "src/Button.tsx"},
		plans[0].Files)
}

func TestPlanExtractReportsUnhandledExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "App.tsx"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "x.foo"), []byte("x"), 0o644))

	// Only .tsx has a plugin registered; .foo should surface as
	// unhandled.
	nm := filepath.Join(dir, "node_modules", "@neokapi", "kapi-react")
	require.NoError(t, os.MkdirAll(nm, 0o755))
	bin := fakeExtractorScript(t, dir, "fake-react.sh")
	require.NoError(t, os.WriteFile(filepath.Join(nm, "package.json"), []byte(`{
	    "name": "@neokapi/kapi-react",
	    "kapi-plugin": {
	      "extensions": [".tsx", ".jsx"],
	      "extract": {"exec": ["`+bin+`"]}
	    }
	  }`), 0o644))

	proj := writeExtractProject(t, dir, `
version: v1
content:
  - name: mixed
    archive: i18n/mixed.klz
    items:
      - path: "src/**/*"
`)

	_, err := PlanExtract(proj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no registered extractor")
	assert.Contains(t, err.Error(), ".foo")
}

func TestRunExtractWritesArchiveFromExplicitExtractor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "App.tsx"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Home.tsx"), []byte("x"), 0o644))

	bin := fakeExtractorScript(t, dir, "fake-extract.sh")
	proj := writeExtractProject(t, dir, `
version: v1
content:
  - name: ui
    archive: i18n/ui.klz
    extractor:
      exec: ["`+bin+`"]
    items:
      - path: "src/**/*.tsx"
`)

	var out bytes.Buffer
	require.NoError(t, runExtract(context.Background(), &out, proj, 0))
	s := out.String()
	assert.Contains(t, s, "ui →")
	assert.Contains(t, s, "2 block(s)")
	assert.Contains(t, s, "2 document(s)")

	// Archive on disk — delegate to CollectProjectStatus, which
	// already reads the archive and returns block counts.
	status, err := CollectProjectStatus(proj)
	require.NoError(t, err)
	require.Len(t, status.Collections, 1)
	assert.Equal(t, 2, status.Collections[0].BlockCount,
		"archive contains both extracted blocks")
}

func TestRunExtractAutoDiscoversKapiPlugin(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "App.tsx"), []byte("x"), 0o644))

	nm := filepath.Join(dir, "node_modules", "@neokapi", "kapi-react")
	require.NoError(t, os.MkdirAll(nm, 0o755))
	bin := fakeExtractorScript(t, dir, "fake-react.sh")
	require.NoError(t, os.WriteFile(filepath.Join(nm, "package.json"), []byte(`{
	    "name": "@neokapi/kapi-react",
	    "kapi-plugin": {
	      "extensions": [".tsx", ".jsx"],
	      "extract": {"exec": ["`+bin+`"]}
	    }
	  }`), 0o644))

	proj := writeExtractProject(t, dir, `
version: v1
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
`)

	var out bytes.Buffer
	require.NoError(t, runExtract(context.Background(), &out, proj, 0))
	assert.Contains(t, out.String(), "@neokapi/kapi-react",
		"auto-discovered extractor is named in the progress output")

	status, err := CollectProjectStatus(proj)
	require.NoError(t, err)
	assert.Equal(t, 1, status.Collections[0].BlockCount)
}
