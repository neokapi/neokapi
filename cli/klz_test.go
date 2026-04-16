package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureArchive writes a synthetic .klz archive with the three
// example blocks and an annotation sidecar. Used to exercise the
// klz CLI subcommands end-to-end.
func fixtureArchive(t *testing.T, path string) {
	t.Helper()

	w := klz.NewWriter(klz.WriterOptions{
		Generator: klz.ManifestGenerator{ID: "@neokapi/kapi-format-examples", Version: "0.0.1"},
		Project:   klz.ManifestProject{ID: "neokapi-kapi-format-examples", SourceLocale: "en", TargetLocales: []string{"de"}},
		Created:   "2026-04-15T10:00:00Z",
	})
	doc := &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "@neokapi/kapi-format-examples", Version: "0.0.1"},
		Project:       klf.ProjectInfo{ID: "neokapi-kapi-format-examples", SourceLocale: "en"},
		Documents: []klf.Document{
			{
				ID:           "examples",
				DocumentType: klf.DocumentTypeJSX,
				Path:         "examples/all.tsx",
				Blocks: []klf.Block{
					{
						ID: "files-heading", Hash: "2xykvb", Translatable: true,
						Type: klf.BlockTypeJSXElement,
						Source: []klf.Run{
							{Text: &klf.TextRun{Text: "Files "}},
							{PcOpen: &klf.PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
								Data: `<span className="muted">`, Equiv: "muted"}},
							{Text: &klf.TextRun{Text: "("}},
							{Ph: &klf.PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
								Data: "{count}", Equiv: "count"}},
							{Text: &klf.TextRun{Text: " matched)"}},
							{PcClose: &klf.PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
								Data: "</span>", Equiv: "muted"}},
						},
						Placeholders: []klf.Placeholder{
							{Name: "muted", Kind: klf.PlaceholderElement,
								SourceExpr: `<span className="muted">...</span>`, JSType: "ReactNode"},
							{Name: "count", Kind: klf.PlaceholderVariable,
								SourceExpr: "count", JSType: "number"},
						},
						Properties: klf.BlockProperties{
							File: "src/FilesHeading.tsx", Line: 4,
							Component: "FilesHeading", JSXPath: "FilesHeading > h2", Element: "h2",
						},
					},
				},
			},
		},
	}
	require.NoError(t, w.AddDocument("documents/examples.klf", doc, nil))

	annFile := &klf.AnnotationFile{
		Header: klf.AnnotationFileHeader{
			Type: "header", AnnotationType: "@neokapi/example", AnnotationVersion: "1.0.0",
			Producer: klf.AnnotationProducer{ID: "@neokapi/kapi-format-examples", Version: "0.0.1"},
			Created:  "2026-04-15T12:00:00Z", TargetArchive: "sha256:deadbeef",
		},
		Annotations: []klf.Annotation{
			{Type: "annotation", ID: "review-1",
				Anchor: klf.AnnotationAnchor{Kind: klf.AnchorBlock, Block: "files-heading"},
				Data:   []byte(`{"kind":"review"}`)},
		},
	}
	require.NoError(t, w.AddAnnotationFile("annotations/neokapi-example.klfl", annFile, nil))

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	_, err = w.Write(f)
	require.NoError(t, err)
}

func TestKLZInspect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	app := &App{}
	cmd := app.NewKLZCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"inspect", path})
	require.NoError(t, cmd.Execute())
	out := buf.String()
	assert.Contains(t, out, "@neokapi/kapi-format-examples")
	assert.Contains(t, out, "documents/examples.klf")
	assert.Contains(t, out, "Documents:")
}

func TestKLZVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	app := &App{}
	cmd := app.NewKLZCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"verify", path})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "OK")
}

func TestKLZOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	app := &App{}
	cmd := app.NewKLZCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"orphans", path})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "checked 1 annotations, 0 orphans")
}

func TestKLZExtractPack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	dir := filepath.Join(t.TempDir(), "extracted")
	app := &App{}
	{
		cmd := app.NewKLZCmd()
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"extract", path, "--out", dir})
		require.NoError(t, cmd.Execute())
	}

	// Manifest + documents and annotations should exist on disk.
	for _, rel := range []string{"manifest.json", "documents/examples.klf", "annotations/neokapi-example.klfl"} {
		_, err := os.Stat(filepath.Join(dir, rel))
		require.NoError(t, err, rel)
	}

	repack := filepath.Join(t.TempDir(), "repack.klz")
	{
		cmd := app.NewKLZCmd()
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"pack", dir, "--out", repack})
		require.NoError(t, cmd.Execute())
	}
	r, err := klz.Open(repack)
	require.NoError(t, err)
	defer r.Close()
	assert.Empty(t, r.VerifyAll())
}

func TestKLZDiff(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.klz")
	b := filepath.Join(t.TempDir(), "b.klz")
	fixtureArchive(t, a)
	fixtureArchive(t, b)

	app := &App{}
	cmd := app.NewKLZCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"diff", a, b})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "+0 -0 ~0")
}

func TestKLZAnnotations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	app := &App{}
	cmd := app.NewKLZCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"annotations", path})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "@neokapi/example")
}

func TestCacheInfo(t *testing.T) {
	app := &App{}
	cmd := app.NewCacheCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"info"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "Cache root:")
}

func TestCachePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	app := &App{}
	cmd := app.NewCacheCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"path", path})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "neokapi")
	assert.Contains(t, buf.String(), "klz")
}

func TestCacheWarm(t *testing.T) {
	// Isolate the cache under a temp dir so the test doesn't touch
	// the user's real cache.
	t.Setenv("NEOKAPI_KLZ_CACHE", t.TempDir())

	path := filepath.Join(t.TempDir(), "ex.klz")
	fixtureArchive(t, path)

	app := &App{}
	cmd := app.NewCacheCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"warm", path})
	err := cmd.Execute()
	if err == nil {
		// klzcache tag is enabled: warm should have produced an
		// on-disk cache entry.
		return
	}
	// Without the klzcache tag, warm returns a deferred error that
	// mentions the tag.
	require.Contains(t, err.Error(), "klzcache")
}
