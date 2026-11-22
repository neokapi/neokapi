//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// A TMX import with no -s reads each unit in the store's source language
// when the unit holds it, else in the language the unit or the file declares.
// A unit read in another region of the store's language is stored under its
// own tag, and the import says so; nothing is folded.
func TestStoreImportChoosesTheSourcePerUnit(t *testing.T) {
	p := newStoreProject(t)

	// The header says fr, the unit holds fr and the project's en-GB: the
	// unit is read in en-GB, so a lookup of the English finds it.
	p.run("memory", "import", p.write("fr.tmx", tmxUnits("fr", tuv("fr", "Bouée"), tuv("en-GB", "Buoy"))), "-t", "fr")
	assert.Equal(t, 1, p.total("memory", "lookup", "Buoy", "-t", "fr"), "the en-GB/fr pair is stored")

	// The header says en-US and the unit holds en-GB: en-GB, with no note.
	out := p.run("memory", "import", p.write("mislabel.tmx", tmxUnits("en-US", tuv("en-GB", "Harbour"), tuv("fr", "Port"))), "-t", "fr")
	assert.NotContains(t, out, "note:", "a unit read in the store's language needs no note")
	assert.Equal(t, 1, p.total("memory", "lookup", "Harbour", "-t", "fr"))

	// An en-US unit is stored as en-US, and the note says what happened
	// without offering a flag that would not change it.
	out = p.run("memory", "import", p.write("us.tmx", tmx("en-US", "en-US", "Color")), "-t", "fr")
	assert.Contains(t, out, "1 unit(s) of us.tmx hold en-US text and no en-GB text; they were imported as en-US")
	assert.NotContains(t, out, "--source-locale")
	assert.Equal(t, 1, p.total("memory", "lookup", "Color", "-s", "en-US", "-t", "fr"))

	// An explicit -s the units do not hold imports nothing, names why, and
	// prints no note claiming an import.
	out, err := p.try("memory", "import", p.write("us2.tmx", tmx("en-US", "en-US", "Gray")), "-s", "en-GB", "-t", "fr")
	require.Error(t, err, "an import that keeps no unit fails:\n%s", out)
	assert.Contains(t, out, "1 unit(s) hold no en-GB text")
	assert.NotContains(t, out, "were imported")
	assert.Zero(t, p.total("memory", "lookup", "Gray", "-s", "en-US", "-t", "fr"), "nothing was stored")
}

// Nothing is skipped silently: the summary and --json count each reason.
func TestStoreImportCountsWhatItSkips(t *testing.T) {
	p := newStoreProject(t)
	mixed := p.write("mixed.tmx", tmxUnits("",
		"<tu>"+tuv("en-GB", "Quay")+tuv("fr", "Quai")+"</tu>",
		"<tu>"+tuv("de", "Kai")+tuv("fr", "Quai")+"</tu>",
		"<tu srclang=\"fr\">"+tuv("fr", "Quai")+tuv("de", "Kai")+"</tu>",
		"<tu>"+tuv("en-GB", "Wharf")+"</tu>"))
	res := importCounts(t, p.run("memory", "import", mixed, "-t", "fr", "--json"))
	assert.Equal(t, 1, res.Imported)
	assert.Equal(t, 1, res.NoSource, "the de/fr unit holds neither en-GB nor a declared language")
	assert.Equal(t, 1, res.SameLanguage, "the unit declaring fr would pair fr with fr")
	assert.Equal(t, 1, res.NoTarget, "the en-GB unit holds no fr")

	out := p.run("memory", "import", mixed, "-t", "fr")
	assert.Contains(t, out, "skipped 1 unit(s) with no text in the source language")
	assert.Contains(t, out, "skipped 1 unit(s) whose source would equal the target")
	assert.Contains(t, out, "skipped 1 unit(s) with no translation")
}

// A TBX import keeps every concept and every language: a terms store holds
// concepts across languages.
func TestTermsImportKeepsEveryTBXConcept(t *testing.T) {
	p := newStoreProject(t)
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<tbx style="dca" type="TBX-Basic"><text><body>
  <conceptEntry id="c1"><langSec xml:lang="en-GB"><termSec><term>buoy</term></termSec></langSec><langSec xml:lang="fr"><termSec><term>bouée</term></termSec></langSec></conceptEntry>
  <conceptEntry id="c2"><langSec xml:lang="en"><termSec><term>harbour</term></termSec></langSec><langSec xml:lang="fr"><termSec><term>port</term></termSec></langSec></conceptEntry>
  <conceptEntry id="c3"><langSec xml:lang="de"><termSec><term>Hafen</term></termSec></langSec><langSec xml:lang="fr"><termSec><term>port</term></termSec></langSec></conceptEntry>
</body></text></tbx>
`
	store := filepath.Join(p.dir, "standalone.db")
	out := p.run("terms", "import", p.write("three.tbx", doc), "--file", store)
	assert.Contains(t, out, "3 concepts", "every concept is imported:\n%s", out)
}

// A mixed-language store exports srclang="*all*" with a srclang on each unit;
// imported into an empty standalone store with no -s, every pair comes back.
func TestMemoryExportOfAMixedStoreImportsBack(t *testing.T) {
	p := newStoreProject(t)
	src := filepath.Join(p.dir, "src.db")
	p.run("memory", "import", p.write("gb.tmx", tmx("en-GB", "en-GB", "Harbour")), "-t", "fr", "--file", src)
	p.run("memory", "import", p.write("de.tmx", tmx("de", "de", "Hafen")), "-t", "fr", "--file", src)
	exported := filepath.Join(p.dir, "mixed.tmx")
	p.run("memory", "export", "-o", exported, "--format", "tmx", "--file", src)
	data, err := os.ReadFile(exported)
	require.NoError(t, err)
	require.Contains(t, string(data), `srclang="*all*"`)

	dst := filepath.Join(p.dir, "dst.db")
	out := p.run("memory", "import", exported, "-t", "fr", "--file", dst, "--json")
	assert.Equal(t, 2, importCounts(t, out).Imported, "every pair is imported:\n%s", out)
}

// importCounts decodes the counts of `memory import --json`.
func importCounts(t *testing.T, out string) (c struct {
	Imported     int `json:"imported"`
	NoSource     int `json:"skipped_no_source"`
	SameLanguage int `json:"skipped_source_equals_target"`
	NoTarget     int `json:"skipped_no_target"`
}) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(jsonObject(t, out)), &c), out)
	return c
}

// tuv is one TUV.
func tuv(lang, text string) string {
	return fmt.Sprintf(`<tuv xml:lang="%s"><seg>%s</seg></tuv>`, lang, text)
}

// tmxUnits is a TMX whose header declares srclang (empty for none) and whose
// body holds units: each a full <tu> element, or bare TUVs wrapped in one.
func tmxUnits(srclang string, units ...string) string {
	header := `<header creationtool="e2e" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" datatype="plaintext"`
	if srclang != "" {
		header += fmt.Sprintf(` srclang="%s"`, srclang)
	}
	var body strings.Builder
	if len(units) > 0 && strings.HasPrefix(units[0], "<tuv") {
		body.WriteString("<tu>" + strings.Join(units, "") + "</tu>")
	} else {
		body.WriteString(strings.Join(units, "\n"))
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  ` + header + `/>
  <body>` + body.String() + `</body>
</tmx>
`
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

// An import that keeps no unit fails, and --json carries its counts beside
// the error.
func TestMemoryImportFailureReportsItsCounts(t *testing.T) {
	p := newStoreProject(t)
	file := p.write("none.tmx", tmxUnits("de", "<tu>"+tuv("de", "Kai")+tuv("es", "Muelle")+"</tu>", "<tu></tu>"))
	out, err := p.try("memory", "import", file, "-t", "fr", "--json")
	require.Error(t, err, out)
	var res struct {
		Error    string `json:"error"`
		NoSource int    `json:"skipped_no_source"`
		NoTarget int    `json:"skipped_no_target"`
		Empty    int    `json:"skipped_empty"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonObject(t, out)), &res), out)
	assert.Contains(t, res.Error, "nothing was imported")
	assert.Equal(t, 1, res.NoTarget, "the de/es unit holds no fr")
	assert.Equal(t, 1, res.Empty, "the empty unit")
}

// jsonObject is out from its first "{", the JSON a command printed after any
// note lines.
func jsonObject(t *testing.T, out string) string {
	t.Helper()
	i := strings.Index(out, "{")
	require.GreaterOrEqual(t, i, 0, "no JSON object in:\n%s", out)
	return out[i:]
}
