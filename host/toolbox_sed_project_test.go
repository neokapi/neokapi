package host

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// sedProjectRecipe declares Markdown documents, text notes bound to the
// Markdown format, and a JSON catalog whose translations live in files of
// their own.
const sedProjectRecipe = `version: v1
name: demo
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - path: "docs/**/*.md"
    format: markdown
  - path: "notes/*.txt"
    format: markdown
  - path: loc/en.json
    format: json
    target: "loc/{lang}.json"
`

// sedProject writes a project with sedProjectRecipe under a new directory,
// makes it the working directory with project discovery on, and returns its
// root.
func sedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"kapi.yaml":         sedProjectRecipe,
		"docs/sub/guide.md": "# Guide\n\nVisit the shop today.\n",
		"notes/a.txt":       "# Note\n\nThe shop is *open*.\n",
		"loc/en.json":       `{"a": "Open the shop", "b": "Close"}` + "\n",
		"loc/nb.json":       `{"a": "Apne butikken shop", "b": "Lukk"}` + "\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	inProject(t, root)
	return root
}

// inProject makes dir the working directory with project discovery on.
func inProject(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", "")
	t.Chdir(dir)
}

// printOps runs ksed --print-ops over files and returns the change set it
// printed.
func printOps(t *testing.T, app *App, script string, files ...string) string {
	t.Helper()
	prog, err := ParseSedProgram([]string{script})
	require.NoError(t, err)
	out, err := captureStdout(t, func() error {
		return app.RunSed(context.Background(), sedCommand(), files, prog, SedOptions{PrintOps: true})
	})
	require.NoError(t, err)
	return out
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// Inside a project, ksed reads a file as kapi apply does: by its
// project-relative path, with the format the recipe binds, and a
// translation's file as an edition of its source. So the change set
// --print-ops prints is the one kapi apply applies, from any directory of the
// project, and ksed -i writes the same bytes.
func TestSedPrintOpsAppliesInAProject(t *testing.T) {
	t.Run("from a subdirectory", func(t *testing.T) {
		root := sedProject(t)
		t.Chdir(filepath.Join(root, "docs", "sub"))
		app := newToolboxApp(t)
		printed := printOps(t, app, "s/shop/store/g", "guide.md")
		set, err := change.Decode(strings.NewReader(printed))
		require.NoError(t, err, printed)
		require.Len(t, set.Ops, 1)
		assert.Equal(t, "docs/sub/guide.md", set.Ops[0].At.Doc, "a project's document is named from its root")

		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), printed, ApplyOptions{})
		require.NoError(t, err)
		assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, "# Guide\n\nVisit the store today.\n", fileText(t, "guide.md"))
	})

	t.Run("a file the recipe binds to a format detection would not pick", func(t *testing.T) {
		root := sedProject(t)
		app := newToolboxApp(t)
		printed := printOps(t, app, "s/shop/store/g", "notes/a.txt")
		set, err := change.Decode(strings.NewReader(printed))
		require.NoError(t, err, printed)
		require.Len(t, set.Ops, 1)
		assert.Equal(t, change.Ref{Doc: "notes/a.txt", Block: "note/p"}, set.Ops[0].At, "the block is the Markdown paragraph the recipe's format reads")

		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), printed, ApplyOptions{})
		require.NoError(t, err)
		assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, "# Note\n\nThe store is *open*.\n", fileText(t, filepath.Join(root, "notes", "a.txt")))
	})

	t.Run("a translation's file", func(t *testing.T) {
		root := sedProject(t)
		app := newToolboxApp(t)
		printed := printOps(t, app, "s/shop/store/g", "loc/nb.json")
		set, err := change.Decode(strings.NewReader(printed))
		require.NoError(t, err, printed)
		require.Len(t, set.Ops, 1)
		at := set.Ops[0].At
		assert.Equal(t, "loc/en.json", at.Doc, "the translation is an edition of its source")
		assert.Equal(t, "a", at.Block)
		assert.Equal(t, "nb", string(at.Edition.Locale))

		res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), printed, ApplyOptions{})
		require.NoError(t, err)
		assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		assert.Equal(t, `{"a": "Apne butikken store", "b": "Lukk"}`+"\n", fileText(t, filepath.Join(root, "loc", "nb.json")))
		assert.Equal(t, `{"a": "Open the shop", "b": "Close"}`+"\n", fileText(t, filepath.Join(root, "loc", "en.json")), "the source stays")
	})

	t.Run("ksed -i writes what kapi apply writes, and prints the edited file without touching it", func(t *testing.T) {
		root := sedProject(t)
		app := newToolboxApp(t)
		prog, err := ParseSedProgram([]string{"s/shop/store/g"})
		require.NoError(t, err)
		files := []string{"notes/a.txt", "loc/nb.json"}

		out, err := captureStdout(t, func() error {
			return app.RunSed(context.Background(), sedCommand(), files, prog, SedOptions{})
		})
		require.NoError(t, err)
		assert.Equal(t, "# Note\n\nThe store is *open*.\n"+`{"a": "Apne butikken store", "b": "Lukk"}`+"\n", out)
		assert.Equal(t, "# Note\n\nThe shop is *open*.\n", fileText(t, filepath.Join(root, "notes", "a.txt")), "printing leaves the file as it is")

		require.NoError(t, app.RunSed(context.Background(), sedCommand(), files, prog, SedOptions{InPlace: true}))
		assert.Equal(t, "# Note\n\nThe store is *open*.\n", fileText(t, filepath.Join(root, "notes", "a.txt")))
		assert.Equal(t, `{"a": "Apne butikken store", "b": "Lukk"}`+"\n", fileText(t, filepath.Join(root, "loc", "nb.json")))
	})
}

// ksed and kapi apply lock a document of a project in the project's lock
// directory, so the two exclude each other between reading a file and
// renaming the edit onto it. A read takes no lock and writes nothing into the
// project.
func TestSedLocksAProjectFileWhereKapiApplyDoes(t *testing.T) {
	root := sedProject(t)
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app := newToolboxApp(t)

	printOps(t, app, "s/shop/store/g", "docs/sub/guide.md")
	inspectJSONL(t, app, "docs/sub/guide.md")
	assert.NoDirExists(t, filepath.Join(root, ".kapi"), "reading a project's document leaves the project as it was")

	prog, err := ParseSedProgram([]string{"s/shop/store/g"})
	require.NoError(t, err)
	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{"docs/sub/guide.md"}, prog, SedOptions{InPlace: true}))
	locks, err := os.ReadDir(filepath.Join(root, ".kapi", "work", "locks"))
	require.NoError(t, err)
	require.Len(t, locks, 1)
	assert.FileExists(t, filepath.Join(root, ".kapi", ".gitignore"), "the lock directory is kept out of a commit")
	assert.NoDirExists(t, filepath.Join(DataDir(), "locks"), "no lock outside the project")

	rec := recordOf(t, inspectJSONL(t, app, "docs/sub/guide.md"), "guide/p")
	_, _, err = runApply(t, app, NewEnvCommand(t.Context(), "apply"),
		changeSetOf(t, map[string]any{"op": "set_content", "at": rec.Ref, "if_match": rec.Rev, "text": "Visit the store now."}), ApplyOptions{})
	require.NoError(t, err)
	again, err := os.ReadDir(filepath.Join(root, ".kapi", "work", "locks"))
	require.NoError(t, err)
	require.Len(t, again, 1)
	assert.Equal(t, locks[0].Name(), again[0].Name(), "kapi apply takes the lock ksed took")
}

// --target edits the translation a bilingual or multilingual file holds,
// whichever way its format keeps it: a Qt Linguist catalog and an Xcode
// string catalog as much as XLIFF.
func TestSedTargetEditsTheTranslationEveryBilingualFormatHolds(t *testing.T) {
	noProject(t)
	for name, tc := range map[string]struct {
		file, src, script string
		want, kept        []string
	}{
		"qt linguist": {
			file: "app_fr.ts",
			src: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Hello</source>
        <translation>Bonjour</translation>
    </message>
</context>
</TS>
`,
			script: "s/Bonjour/Salut/",
			want:   []string{"<translation>Salut</translation>"},
			kept:   []string{"<source>Hello</source>"},
		},
		"xcode string catalog": {
			file: "Localizable.xcstrings",
			src: `{
  "sourceLanguage" : "en",
  "strings" : {
    "Account" : {
      "localizations" : {
        "fr" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Compte"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
			script: "s/Compte/Profil/",
			want:   []string{`"value" : "Profil"`},
			kept:   []string{`"Account" : {`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := newToolboxApp(t)
			path := filepath.Join(t.TempDir(), tc.file)
			require.NoError(t, os.WriteFile(path, []byte(tc.src), 0o644))
			prog, err := ParseSedProgram([]string{tc.script})
			require.NoError(t, err)
			require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{path}, prog, SedOptions{InPlace: true, Target: "fr"}))
			got := fileText(t, path)
			for _, w := range tc.want {
				assert.Contains(t, got, w)
			}
			for _, k := range tc.kept {
				assert.Contains(t, got, k)
			}
		})
	}
}

// Each format whose file keeps translations beside the source holds its
// editions in the file; any other holds one.
func TestEditionsOfAFormat(t *testing.T) {
	app := newToolboxApp(t)
	for _, name := range []string{"po", "xliff", "xliff2", "tmx", "ts", "xcstrings"} {
		assert.Equal(t, change.EditionsInFile, app.editionsOf(name), name)
	}
	for _, name := range []string{"json", "html", "markdown", "plaintext"} {
		assert.Equal(t, change.EditionsPerFile, app.editionsOf(name), name)
	}
}

// Inside a project, a bilingual source whose translations the recipe writes to
// files of their own keeps them there, though the source file can hold a
// translation: the French edition of a Qt Linguist catalog lives in the French
// file, and the French of a PO template in po/fr.po, read in French, whatever
// target language the service was told. A catalog whose collection names no
// target holds its translation in place.
func TestProjectPlacesTheEditionsOfABilingualSource(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	const ts = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE TS>\n<TS version=\"2.1\" language=\"en\" sourcelanguage=\"en\">\n<context>\n    <name>MainWindow</name>\n    <message>\n        <source>Hello</source>\n        <translation>Hello</translation>\n    </message>\n</context>\n</TS>\n"
	write("kapi.yaml", `version: v1
name: bilingual
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - path: i18n/app_en.ts
    format: ts
    target: "i18n/app_{lang}.ts"
  - path: po/messages.pot
    format: po
    target: "po/{lang}.po"
  - path: work/fr.po
    format: po
`)
	write("i18n/app_en.ts", ts)
	write("po/messages.pot", "msgid \"\"\nmsgstr \"\"\n\nmsgid \"Hello\"\nmsgstr \"\"\n")
	write("work/fr.po", "msgid \"\"\nmsgstr \"\"\n\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n")
	app := newToolboxApp(t)
	pl, err := app.newProjectLayout(ChangeServiceOptions{Project: filepath.Join(root, "kapi.yaml"), TargetLocale: "fr"})
	require.NoError(t, err)

	d, err := pl.Locate(t.Context(), "i18n/app_en.ts")
	require.NoError(t, err)
	assert.Equal(t, change.EditionsPerFile, d.Editions)
	f, ok := d.EditionFile(model.EditionKey{Locale: "fr"})
	require.True(t, ok)
	assert.Equal(t, "i18n/app_fr.ts", f.Ref)

	d, err = pl.Locate(t.Context(), "po/messages.pot")
	require.NoError(t, err)
	assert.Equal(t, change.EditionsPerFile, d.Editions)
	assert.Empty(t, d.TargetLocale, "the template holds no translation of its own")
	f, ok = d.EditionFile(model.EditionKey{Locale: "fr"})
	require.True(t, ok)
	assert.Equal(t, "po/fr.po", f.Ref)
	assert.True(t, f.Bilingual, "po/fr.po is read in French")

	d, err = pl.Locate(t.Context(), "work/fr.po")
	require.NoError(t, err)
	assert.Equal(t, change.EditionsInFile, d.Editions)
	assert.Equal(t, model.LocaleID("fr"), d.TargetLocale)
}

// A block whose key another block of the document shares is edited all the
// same: ksed addresses it by the id its reader gave it, which names one
// block, in every mode, and kapi apply resolves the printed id too.
func TestSedEditsBlocksThatShareAKey(t *testing.T) {
	noProject(t)
	dir := t.TempDir()
	t.Chdir(dir)
	app := newToolboxApp(t)
	const page = `<html><body><p id="x">The shop one.</p><p id="x">The shop two.</p></body></html>` + "\n"
	const edited = `<html><body><p id="x">The store one.</p><p id="x">The store two.</p></body></html>` + "\n"
	files := map[string]string{
		"inplace.html": page, "printed.html": page,
		"dup.json": `{"a": "shop", "a": "shop two"}` + "\n",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(name, []byte(body), 0o644))
	}
	prog, err := ParseSedProgram([]string{"s/shop/store/g"})
	require.NoError(t, err)

	out, err := captureStdout(t, func() error {
		return app.RunSed(context.Background(), sedCommand(), []string{"inplace.html"}, prog, SedOptions{})
	})
	require.NoError(t, err)
	assert.Equal(t, edited, out)

	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{"inplace.html", "dup.json"}, prog, SedOptions{InPlace: true}))
	assert.Equal(t, edited, fileText(t, "inplace.html"))
	assert.Equal(t, `{"a": "store", "a": "store two"}`+"\n", fileText(t, "dup.json"))

	printed := printOps(t, app, "s/shop/store/g", "printed.html")
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), printed, ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, edited, fileText(t, "printed.html"))
}

// writeZip writes an archive holding files, in the order given.
func writeZip(t *testing.T, path string, files ...[2]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f[0])
		require.NoError(t, err)
		_, err = w.Write([]byte(f[1]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

// ksed -i edits an archive all or nothing: every member's change is checked
// before any is written, so a member whose edit is refused leaves every
// member as it was, and when none is refused every member lands.
func TestSedEditsAnArchiveAllOrNothing(t *testing.T) {
	noProject(t)
	dir := t.TempDir()
	t.Chdir(dir)
	app := newToolboxApp(t)
	members := [][2]string{
		{"a.html", "<html><body><p>The shop A.</p></body></html>\n"},
		{"b.html", `<html><body><p id="x">The shop one.</p><p id="x">The shop two.</p></body></html>` + "\n"},
		{"c.html", "<html><body><p>The shop C.</p></body></html>\n"},
	}
	writeZip(t, "site.zip", members...)
	prog, err := ParseSedProgram([]string{"s/shop/store/g"})
	require.NoError(t, err)

	// Another writer changes b.html once ksed has read every member, so the
	// edit of b.html is refused stale when it is checked.
	changed := `<html><body><p id="x">The shop one, changed.</p><p id="x">The shop two.</p></body></html>` + "\n"
	sedArchiveCompiled = func(string) {
		writeZip(t, "site.zip", members[0], [2]string{"b.html", changed}, members[2])
	}
	t.Cleanup(func() { sedArchiveCompiled = nil })
	stderr, err := captureStderr(t, func() error {
		return app.RunSed(context.Background(), sedCommand(), []string{"site.zip"}, prog, SedOptions{InPlace: true, BackupSuffix: ".bak"})
	})
	sedArchiveCompiled = nil
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, stderr, "stale")
	writeZip(t, "theirs.zip", members[0], [2]string{"b.html", changed}, members[2])
	assert.Equal(t, fileText(t, "theirs.zip"), fileText(t, "site.zip"),
		"a refused member leaves every member as the other writer left it, a.html included")
	assert.NoFileExists(t, "site.zip.bak", "nothing was replaced, so nothing was backed up")

	writeZip(t, "site.zip", members...)
	before := fileText(t, "site.zip")
	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{"site.zip"}, prog, SedOptions{InPlace: true, BackupSuffix: ".bak"}))
	zr, err := zip.OpenReader("site.zip")
	require.NoError(t, err)
	defer zr.Close()
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		var b bytes.Buffer
		_, err = b.ReadFrom(rc)
		require.NoError(t, err)
		_ = rc.Close()
		assert.Contains(t, b.String(), "store", f.Name)
		assert.NotContains(t, b.String(), "shop", f.Name)
	}
	assert.Equal(t, before, fileText(t, "site.zip.bak"), "the backup holds the archive as it was before the run")
}

// --print-ops prints nothing of a file whose operations did not all compile:
// an archive whose second member cannot be read contributes none of the
// first member's operations, as ksed -i would write none of them.
func TestSedPrintOpsLeavesOutAFileThatFailed(t *testing.T) {
	noProject(t)
	dir := t.TempDir()
	t.Chdir(dir)
	app := newToolboxApp(t)
	require.NoError(t, os.WriteFile("good.json", []byte(`{"a": "shop"}`), 0o644))
	// b.xml is a member whose name a format claims and whose bytes do not
	// parse as that format.
	writeZip(t, "site.zip",
		[2]string{"a.html", "<html><body><p>The shop A.</p></body></html>\n"},
		[2]string{"b.xml", "<root><a>The shop</b></root>\n"})
	prog, err := ParseSedProgram([]string{"s/shop/store/"})
	require.NoError(t, err)
	var printed string
	stderr, err := captureStderr(t, func() error {
		var perr error
		printed, perr = captureStdout(t, func() error {
			return app.RunSed(context.Background(), sedCommand(), []string{"site.zip", "good.json"}, prog, SedOptions{PrintOps: true})
		})
		return perr
	})
	require.Error(t, err, "a member that could not be read is an error")
	assert.Contains(t, stderr, "site.zip")
	set, derr := change.Decode(strings.NewReader(printed))
	require.NoError(t, derr, printed)
	require.Len(t, set.Ops, 1, "%+v", set.Ops)
	assert.Equal(t, "good.json", set.Ops[0].At.Doc)
}

// A copy layout reads every document as the layout it wraps does and writes
// only the copy.
func TestCopyLayoutRedirectsOnlyTheCopiedFile(t *testing.T) {
	root := sedProject(t)
	app := newToolboxApp(t)
	recipe := filepath.Join(root, "kapi.yaml")
	pl, err := app.newProjectLayout(ChangeServiceOptions{Project: recipe})
	require.NoError(t, err)
	file := filepath.Join(root, "loc", "nb.json")
	copyPath := filepath.Join(t.TempDir(), "nb.json")
	l := copyLayout{inner: pl, file: file, copy: copyPath}

	d, err := l.Locate(t.Context(), "loc/nb.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "loc", "en.json"), d.Path, "the source is read where it lies")
	require.NotNil(t, d.Edition)
	f, ok := d.EditionFile(*d.Edition)
	require.True(t, ok)
	assert.Equal(t, copyPath, f.Path, "the translation's file is the copy")
	assert.Empty(t, d.Derived)

	d, err = l.Locate(t.Context(), "notes/a.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "notes", "a.txt"), d.Path, "a file not copied is read where it lies")
}

// A printing run records nothing, an edit made outside kapi included: the read
// that compiles the change set it prints is left unobserved.
func TestSedPrintOpsRecordsNothingItFinds(t *testing.T) {
	root := sedProject(t)
	app := newToolboxApp(t)
	recorded := func() int {
		t.Helper()
		db, err := app.ProjectDB(t.Context(), root)
		require.NoError(t, err)
		rows, err := db.History().Document(t.Context(), app.documentIndexOrEmpty(t.Context(), root).Key("docs/sub/guide.md"))
		require.NoError(t, err)
		return len(rows)
	}
	// A recorded edit gives the document a history to compare a read with.
	res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), printOps(t, app, "s/shop/store/g", "docs/sub/guide.md"), ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	before := recorded()
	require.Positive(t, before)

	// A person edits the file in their editor, then prints what ksed would do.
	guide := filepath.Join(root, "docs", "sub", "guide.md")
	require.NoError(t, os.WriteFile(guide, []byte("# Guide\n\nVisit the store tomorrow.\n"), 0o644))
	printed := printOps(t, app, "s/store/shop/g", "docs/sub/guide.md")
	require.Contains(t, printed, "replace_text")
	assert.Equal(t, before, recorded(), "the printing run recorded nothing")
}
