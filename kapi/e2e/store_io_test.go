//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeProject is a copy of the compass sample (source language en-GB) with
// its own data root and config directory.
type storeProject struct {
	t                      *testing.T
	dir, dataRoot, cfgRoot string
}

func newStoreProject(t *testing.T) *storeProject {
	t.Helper()
	p := &storeProject{t: t}
	for _, d := range []*string{&p.dir, &p.dataRoot, &p.cfgRoot} {
		v, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		*d = v
	}
	copyFixture(t, filepath.Join(findRoot(), "samples", "compass"), p.dir)
	return p
}

// try runs kapi in the project and returns its combined output and error.
func (p *storeProject) try(args ...string) (string, error) {
	p.t.Helper()
	cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
	cmd.Dir = p.dir
	cmd.Env = append(append(os.Environ(), isoEnv...), "KAPI_DATA_DIR="+p.dataRoot, "KAPI_CONFIG_DIR="+p.cfgRoot)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// run runs kapi in the project and fails the test when it fails.
func (p *storeProject) run(args ...string) string {
	p.t.Helper()
	out, err := p.try(args...)
	require.NoError(p.t, err, "kapi %v failed:\n%s", args, out)
	return out
}

// total runs a lookup with --json and returns its match count.
func (p *storeProject) total(args ...string) int {
	p.t.Helper()
	cmd := exec.Command(kapiBin, append(append(args, "--json"), "-p", ".")...)
	cmd.Dir = p.dir
	cmd.Env = append(append(os.Environ(), isoEnv...), "KAPI_DATA_DIR="+p.dataRoot, "KAPI_CONFIG_DIR="+p.cfgRoot)
	out, err := cmd.Output()
	require.NoError(p.t, err, "kapi %v failed:\n%s", args, out)
	var res struct {
		Total int `json:"total"`
	}
	require.NoError(p.t, json.Unmarshal(out, &res), string(out))
	return res.Total
}

// write writes a file into the project and returns its path.
func (p *storeProject) write(name, body string) string {
	p.t.Helper()
	path := filepath.Join(p.dir, name)
	require.NoError(p.t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// tmx is a one-unit TMX whose header declares srclang and whose unit holds
// source text in src and a French translation.
func tmx(srclang, src, text string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="e2e" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="%s" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="%s"><seg>%s</seg></tuv><tuv xml:lang="fr"><seg>Traduction</seg></tuv></tu>
  </body>
</tmx>
`, srclang, src, text)
}

// tbx is a one-concept TBX whose root declares lang and whose concept holds a
// term in lang and a French one.
func tbx(lang, term string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tbx style="dca" type="TBX-Basic" xml:lang="%s"><text><body>
  <conceptEntry id="c1">
    <langSec xml:lang="%s"><termSec><term>%s</term></termSec></langSec>
    <langSec xml:lang="fr"><termSec><term>terme</term></termSec></langSec>
  </conceptEntry>
</body></text></tbx>
`, lang, lang, term)
}

// Store imports and exports with no -s use the project's source language
// for the project's own store, when the file declares none. Inside the
// compass sample (en-GB), a TMX that declares srclang="*all*" and a
// source-and-target CSV import in en-GB, the lookups find them, and a terms
// export reads the en-GB source terms.
func TestStoreImportExportUseTheProjectSourceLanguage(t *testing.T) {
	p := newStoreProject(t)

	p.run("memory", "import", p.write("all.tmx", tmx("*all*", "en-GB", "Harbour master")), "-t", "fr")
	assert.Equal(t, 1, p.total("memory", "lookup", "Harbour master", "-t", "fr"),
		"memory import with no -s reads a TMX that declares no single language in the project's en-GB")

	p.run("terms", "import", filepath.Join(testdata, "terms.csv"), "--format", "csv", "-t", "fr", "--header")
	assert.Equal(t, 1, p.total("terms", "lookup", "password", "-t", "fr"),
		"terms import with no -s stores the CSV's source column as en-GB")

	assert.Contains(t, p.run("terms", "export", "--format", "csv", "-t", "fr"), "password,mot de passe",
		"terms export with no -s reads the en-GB source terms")
}

// A TMX or TBX import with no -s reads in the language the file declares.
// When that differs from the store's in region only, the import says so on
// standard error. When no unit or concept holds the source language, the
// import fails, names both languages and stores nothing.
func TestStoreImportUsesTheFileSourceLanguage(t *testing.T) {
	p := newStoreProject(t)

	usTMX := p.write("us.tmx", tmx("en-US", "en-US", "Color"))
	out := p.run("memory", "import", usTMX, "-t", "fr")
	assert.Contains(t, out, "us.tmx declares en-US and this store reads in en-GB", "the region-only difference is reported")
	assert.Equal(t, 1, p.total("memory", "lookup", "Color", "-s", "en-US", "-t", "fr"),
		"the en-US text is imported as en-US")

	out, err := p.try("memory", "import", usTMX, "-s", "en-GB", "-t", "fr")
	require.Error(t, err, "an import of which no unit holds the source language fails:\n%s", out)
	assert.Contains(t, out, "en-GB")
	assert.Contains(t, out, "declares en-US")
	assert.Zero(t, p.total("memory", "lookup", "Color", "-s", "en-GB", "-t", "fr"), "nothing was stored")

	usTBX := p.write("us.tbx", tbx("en-US", "colour chart"))
	out = p.run("terms", "import", usTBX, "-t", "fr")
	assert.Contains(t, out, "us.tbx declares en-US and this store reads in en-GB")
	assert.Equal(t, 1, p.total("terms", "lookup", "colour chart", "-s", "en-US", "-t", "fr"))

	other := newStoreProject(t)
	usTBX = other.write("us.tbx", tbx("en-US", "colour chart"))
	out, err = other.try("terms", "import", usTBX, "-s", "en-GB", "-t", "fr")
	require.Error(t, err, "a TBX import with no en-GB term fails:\n%s", out)
	assert.Contains(t, out, "en-GB")
	assert.Contains(t, out, "declares en-US")
	assert.Zero(t, other.total("terms", "lookup", "colour chart", "-s", "en-US", "-t", "fr"), "nothing was stored")

	// A standalone store reads in en; an en-GB TMX is imported as en-GB with
	// the note.
	legacy := filepath.Join(p.dir, "legacy.db")
	out = p.run("memory", "import", p.write("gb.tmx", tmx("en-GB", "en-GB", "Harbour")), "-t", "fr", "--file", legacy)
	assert.Contains(t, out, "gb.tmx declares en-GB and this store reads in en")
	assert.Equal(t, 1, p.total("memory", "lookup", "Harbour", "-s", "en-GB", "-t", "fr", "--file", legacy))
}

// A standalone store holds content of no particular project, so with no -s
// an import that declares no language, and an export, read in en, for each
// selector of memory and terms.
func TestStoreImportExportInAStandaloneStoreKeepTheDefault(t *testing.T) {
	p := newStoreProject(t)
	allTMX := p.write("all.tmx", tmx("*all*", "en", "Settings"))
	for _, sel := range [][]string{
		{"--file", filepath.Join(p.dir, "legacy.db")},
		{"--name", "legacy"},
		{"--local"},
	} {
		t.Run(sel[0], func(t *testing.T) {
			p.run(append([]string{"memory", "import", allTMX, "-t", "fr"}, sel...)...)
			assert.Equal(t, 1, p.total(append([]string{"memory", "lookup", "Settings", "-s", "en", "-t", "fr"}, sel...)...),
				"memory import %s with no -s reads in en", sel[0])

			p.run(append([]string{"terms", "import", filepath.Join(testdata, "terms.csv"), "--format", "csv", "-t", "fr", "--header"}, sel...)...)
			assert.Equal(t, 1, p.total(append([]string{"terms", "lookup", "password", "-s", "en", "-t", "fr"}, sel...)...),
				"terms import %s with no -s stores en terms", sel[0])
			assert.Contains(t, p.run(append([]string{"terms", "export", "--format", "csv", "-t", "fr"}, sel...)...), "password,mot de passe",
				"terms export %s with no -s reads the en terms", sel[0])
		})
	}
}

// A directory import with a file that imports nothing exits non-zero, names
// the count, and keeps what the other files imported.
func TestMemoryImportDirFailsWhenAFileImportsNothing(t *testing.T) {
	p := newStoreProject(t)
	dir := filepath.Join(p.dir, "tmx")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.tmx"), []byte(tmx("en-GB", "en-GB", "Harbour")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "us.tmx"), []byte(tmx("en-US", "en-US", "Color")), 0o644))
	store := filepath.Join(p.dir, "dir.db")

	// import-dir takes no -p (its -p is --pattern), so it writes a named file.
	cmd := exec.Command(kapiBin, "memory", "import-dir", dir, "-s", "en-GB", "-t", "fr", "--file", store)
	cmd.Dir = p.dir
	cmd.Env = append(append(os.Environ(), isoEnv...), "KAPI_DATA_DIR="+p.dataRoot, "KAPI_CONFIG_DIR="+p.cfgRoot)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "a directory import with a failed file fails:\n%s", out)
	assert.Contains(t, string(out), "1 of 2 files failed to import")
	assert.Equal(t, 1, p.total("memory", "lookup", "Harbour", "-s", "en-GB", "-t", "fr", "--file", store), "the readable file's entry is kept")
}
