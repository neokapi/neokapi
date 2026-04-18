package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExtractorScript writes a shell script that reads NUL-separated
// paths from stdin and emits one fake NDJSON block per path. Used
// as the `command:` target in tests — avoids needing a real
// kapi-react.
func fakeExtractorScript(t *testing.T, dir, name string) string {
	t.Helper()
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

func TestPlanExtractResolvesExecFormat(t *testing.T) {
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
    items:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "`+bin+`"
`)

	plans, err := PlanExtract(proj)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "ui", plans[0].Collection)
	assert.Equal(t, "i18n/ui.klz", plans[0].Archive)
	assert.Equal(t, []string{bin}, plans[0].Command)
	assert.ElementsMatch(t,
		[]string{"src/App.tsx", "src/Button.tsx"},
		plans[0].Files)
}

func TestPlanExtractSkipsNonExecItems(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sample.md"), []byte("x"), 0o644))

	// No exec format declared: plan should exclude the collection.
	proj := writeExtractProject(t, dir, `
version: v1
content:
  - name: docs
    archive: i18n/docs.klz
    items:
      - path: "*.md"
        format: markdown
`)

	plans, err := PlanExtract(proj)
	require.NoError(t, err)
	assert.Empty(t, plans)
}

func TestRunExtractWritesArchive(t *testing.T) {
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
    items:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "`+bin+`"
`)

	var out bytes.Buffer
	require.NoError(t, runExtract(context.Background(), &out, proj, 0))
	s := out.String()
	assert.Contains(t, s, "ui →")
	assert.Contains(t, s, "2 block(s)")

	status, err := CollectProjectStatus(proj)
	require.NoError(t, err)
	require.Len(t, status.Collections, 1)
	assert.Equal(t, 2, status.Collections[0].BlockCount)
}

func TestRunPackFromNDJSONStdin(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "out.klz")

	stdin := strings.NewReader(strings.Join([]string{
		`scanning files`,
		`{"type":"block","document":"a.tsx","block":{"id":"b1","hash":"H1","translatable":true,"type":"jsx:element","source":[{"text":"Hello"}]}}`,
		`{"type":"block","document":"b.tsx","block":{"id":"b2","hash":"H2","translatable":true,"type":"jsx:element","source":[{"text":"World"}]}}`,
		``,
	}, "\n"))

	var out bytes.Buffer
	require.NoError(t, runPack(context.Background(), stdin, &out, "", archive))
	assert.FileExists(t, archive)
	assert.Contains(t, out.String(), "2 block(s) across 2 document(s)")
}

func TestRunPackFromKLFDirectory(t *testing.T) {
	dir := t.TempDir()
	klfDir := filepath.Join(dir, "klf")
	require.NoError(t, os.MkdirAll(klfDir, 0o755))

	// Minimal hand-built .klf — one document, one block.
	klf1 := `{
      "schemaVersion": "1.0",
      "kind": "kapi-localization-format",
      "generator": {"id": "test", "version": "0.0.0"},
      "project": {"id": "demo", "sourceLocale": "en"},
      "documents": [{
        "id": "d1",
        "documentType": "jsx",
        "path": "src/App.tsx",
        "blocks": [{
          "id": "b1",
          "hash": "H1",
          "translatable": true,
          "type": "jsx:element",
          "source": [{"text": "Hello"}],
          "placeholders": [],
          "properties": {"file": "src/App.tsx", "line": 1, "component": "App", "jsxPath": "h1", "element": "h1"}
        }]
      }]
    }`
	require.NoError(t, os.WriteFile(filepath.Join(klfDir, "App.klf"), []byte(klf1), 0o644))

	archive := filepath.Join(dir, "out.klz")
	var out bytes.Buffer
	require.NoError(t, runPack(context.Background(), strings.NewReader(""), &out, klfDir, archive))
	assert.FileExists(t, archive)
	assert.Contains(t, out.String(), "1 block(s)")
}

func TestSplitCommandHandlesQuotedArgs(t *testing.T) {
	assert.Equal(t, []string{"vp", "kapi-react", "extract", "--stream"},
		splitCommand("vp kapi-react extract --stream"))
	assert.Equal(t, []string{"node", "run", "fancy thing with spaces"},
		splitCommand(`node run "fancy thing with spaces"`))
	assert.Equal(t, []string{"sh", "-c", "echo hi"},
		splitCommand(`sh -c "echo hi"`))
}
