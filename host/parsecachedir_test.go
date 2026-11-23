package host

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// parseCacheProject writes a project with one source file at docs/x.json and
// returns its root.
func parseCacheProject(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: pc\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.json"), []byte(content), 0o644))
	return root
}

// recordBlock records src under key with one block and closes the recording.
func recordBlock(t *testing.T, c *docCache, src, key, text string) {
	t.Helper()
	rec := c.RecordDocument(src, key, "json")
	require.NotNil(t, rec)
	require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: mkBlock("a", text)}))
	require.NoError(t, rec.Commit())
}

func TestParseCacheDir_UnsetIsUnderTheStateDirectory(t *testing.T) {
	t.Setenv(EnvParseCacheDir, "")
	layout := project.LayoutAt(t.TempDir())
	dir, err := ParseCacheDir(layout)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(layout.CacheDir(), "docs"), dir)
}

func TestParseCacheDir_RefusesARelativePath(t *testing.T) {
	t.Setenv(EnvParseCacheDir, "kapi-parse")
	_, err := ParseCacheDir(project.LayoutAt(t.TempDir()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an absolute path")
	assert.NoDirExists(t, "kapi-parse", "nothing is created against the working directory")
}

func TestParseCacheDir_RefusesALink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "parse")
	require.NoError(t, os.Symlink(target, link))
	t.Setenv(EnvParseCacheDir, link)
	_, err := ParseCacheDir(project.LayoutAt(t.TempDir()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a symbolic link")
}

// A directory inside the checkout is refused whether or not git ignores it:
// kapi reads nothing inside a checkout as its own state.
func TestParseCacheDir_RefusesADirectoryInsideTheCheckout(t *testing.T) {
	root := parseCacheProject(t, `{}`)
	t.Setenv(EnvParseCacheDir, filepath.Join(root, ".cache", "parse"))
	_, err := ParseCacheDir(project.LayoutAt(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside the checkout")
	assert.NoDirExists(t, filepath.Join(root, ".cache"))
}

func TestParseCacheDir_CreatesTheDirectoryPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "parse")
	t.Setenv(EnvParseCacheDir, dir)
	got, err := ParseCacheDir(project.LayoutAt(t.TempDir()))
	require.NoError(t, err)
	assert.Equal(t, dir, got)
	st, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), st.Mode().Perm())
}

// In a test binary the variable cannot place the cache outside the temporary
// directory, so a test that sets it writes nowhere else.
func TestParseCacheDir_StaysInTheTemporaryDirectoryUnderTest(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	outside := filepath.Join(wd, "testdata-parse-cache-never-created")
	t.Setenv(EnvParseCacheDir, outside)
	_, err = ParseCacheDir(project.LayoutAt(t.TempDir()))
	require.Error(t, err)
	assert.NoDirExists(t, outside)
}

// Two checkouts of one project, in different folders, share one directory: an
// entry is filed under the path relative to the project root, so the second
// finds what the first recorded. A file at the same relative path with other
// content, or under another parse configuration, does not.
func TestDocCache_TwoProjectsShareOneDirectory(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "parse")
	t.Setenv(EnvParseCacheDir, shared)
	a := parseCacheProject(t, `{"a":"Apple"}`)
	b := parseCacheProject(t, `{"a":"Apple"}`)
	other := parseCacheProject(t, `{"a":"Pearl"}`)

	ca, err := openProjectDocCache(project.LayoutAt(a))
	require.NoError(t, err)
	defer ca.Close()
	recordBlock(t, ca, filepath.Join(a, "docs", "x.json"), "k", "Apple")
	assert.FileExists(t, filepath.Join(shared, "index.db"), "the cache lives in the named directory")

	cb, err := openProjectDocCache(project.LayoutAt(b))
	require.NoError(t, err)
	defer cb.Close()
	assert.NotNil(t, cb.OpenDocument(filepath.Join(b, "docs", "x.json"), "k"), "the same file in another clone is a hit")
	assert.Nil(t, cb.OpenDocument(filepath.Join(b, "docs", "x.json"), "other-config"), "another parse configuration is a miss")

	co, err := openProjectDocCache(project.LayoutAt(other))
	require.NoError(t, err)
	defer co.Close()
	assert.Nil(t, co.OpenDocument(filepath.Join(other, "docs", "x.json"), "k"), "other content at the same path is a miss")
}

// The size and modification time stand for a file's content only in the
// checkout that recorded them: another checkout's file of the same size and
// time, with other bytes, is a miss.
func TestDocCache_AnotherCheckoutIsComparedByContent(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "parse")
	t.Setenv(EnvParseCacheDir, shared)
	a := parseCacheProject(t, `{"a":"Apple"}`)
	b := parseCacheProject(t, `{"a":"Pearl"}`)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(a, "docs", "x.json"), stamp, stamp))
	require.NoError(t, os.Chtimes(filepath.Join(b, "docs", "x.json"), stamp, stamp))

	ca, err := openProjectDocCache(project.LayoutAt(a))
	require.NoError(t, err)
	defer ca.Close()
	recordBlock(t, ca, filepath.Join(a, "docs", "x.json"), "k", "Apple")

	cb, err := openProjectDocCache(project.LayoutAt(b))
	require.NoError(t, err)
	defer cb.Close()
	assert.Nil(t, cb.OpenDocument(filepath.Join(b, "docs", "x.json"), "k"))
}

// A restored entry whose log was truncated, replaced by another file or by a
// link, or filed under a name the inputs do not give, is dropped and the
// document parsed again; none of it is an error.
func TestDocCache_DiscardsATruncatedOrForeignEntry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(t *testing.T, c *docCache, log string)
	}{
		{"truncated", func(t *testing.T, _ *docCache, log string) {
			data, err := os.ReadFile(log)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(log, data[:len(data)/2], 0o600))
		}},
		{"foreign", func(t *testing.T, _ *docCache, log string) {
			require.NoError(t, os.WriteFile(log, []byte(`{"t":"block","b":{"ID":"z"}}`+"\n"), 0o600))
		}},
		{"a link", func(t *testing.T, _ *docCache, log string) {
			copied := log + ".copy"
			require.NoError(t, os.Rename(log, copied))
			require.NoError(t, os.Symlink(copied, log))
		}},
		{"renamed", func(t *testing.T, c *docCache, log string) {
			other := filepath.Join(filepath.Dir(log), "parts-elsewhere.log")
			require.NoError(t, os.Rename(log, other))
			_, err := c.DB.Exec(`UPDATE documents SET parts_ref = ?`, filepath.Base(other))
			require.NoError(t, err)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvParseCacheDir, filepath.Join(t.TempDir(), "parse"))
			root := parseCacheProject(t, `{"a":"Apple"}`)
			src := filepath.Join(root, "docs", "x.json")
			c, err := openProjectDocCache(project.LayoutAt(root))
			require.NoError(t, err)
			defer c.Close()
			recordBlock(t, c, src, "k", "Apple")
			logs, err := filepath.Glob(filepath.Join(c.dir, "parts-*.log"))
			require.NoError(t, err)
			require.Len(t, logs, 1)

			tc.spoil(t, c, logs[0])
			assert.Nil(t, c.OpenDocument(src, "k"), "a spoiled entry is a miss")
			var n int
			require.NoError(t, c.DB.QueryRow(`SELECT COUNT(*) FROM documents`).Scan(&n))
			assert.Zero(t, n, "and its row is gone")

			recordBlock(t, c, src, "k", "Apple")
			assert.NotNil(t, c.OpenDocument(src, "k"), "the document is recorded again")
		})
	}
}

// Every reader and writer of the parse cache asks ParseCacheDir: no other Go
// source builds the directory's path by hand.
func TestParseCacheDir_IsTheOneResolver(t *testing.T) {
	built := regexp.MustCompile(`CacheDir\(\)[^\n]*"docs"|"cache",\s*"docs"|"[^"\n]*cache/docs[^"\n]*"`)
	repo := ".."
	var found []string
	err := filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "testdata", "bin", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if built.Match(src) {
			found = append(found, p)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, found, "these files build the parse cache path; ask ParseCacheDir instead")
}
