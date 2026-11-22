//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Store imports and exports read and write in the project's source language
// when no -s is given, as the lookups do. Inside the compass sample (en-GB),
// an en-GB TMX and a source-and-target CSV import into the project's stores
// with no -s, the lookups find them, and a terms export with no -s reads the
// en-GB source terms.
func TestStoreImportExportUseTheProjectSourceLanguage(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	copyFixture(t, filepath.Join(findRoot(), "samples", "compass"), dir)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...), "KAPI_DATA_DIR="+dataRoot)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "kapi %v failed:\n%s", args, out)
		return string(out)
	}
	total := func(out string) int {
		t.Helper()
		var res struct {
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &res), out)
		return res.Total
	}

	tmx := filepath.Join(dir, "legacy.tmx")
	require.NoError(t, os.WriteFile(tmx, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="e2e" creationtoolversion="1" segtype="sentence" o-tmf="unknown" adminlang="en-GB" srclang="en-GB" datatype="plaintext"/>
  <body>
    <tu>
      <tuv xml:lang="en-GB"><seg>Harbour master</seg></tuv>
      <tuv xml:lang="fr"><seg>Capitaine de port</seg></tuv>
    </tu>
  </body>
</tmx>
`), 0o644))
	run("memory", "import", tmx, "-t", "fr")
	assert.Equal(t, 1, total(run("memory", "lookup", "Harbour master", "-t", "fr", "--json")),
		"memory import with no -s reads the TMX in the project's en-GB")

	run("terms", "import", filepath.Join(testdata, "terms.csv"), "--format", "csv", "-t", "fr", "--header")
	assert.Equal(t, 1, total(run("terms", "lookup", "password", "-t", "fr", "--json")),
		"terms import with no -s stores the CSV's source column as en-GB")

	csv := run("terms", "export", "--format", "csv", "-t", "fr")
	assert.Contains(t, csv, "password,mot de passe", "terms export with no -s reads the en-GB source terms")
}
