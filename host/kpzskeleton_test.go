package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/kpz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A .kpz's skeleton is what `kapi merge` writes the translation back THROUGH. The
// capture used to be gated on `serr == nil && len(skel) > 0`, which folded a
// capture FAILURE into the legitimate "this format has no skeleton" case that
// captureSkeletonBytes already reports as (nil, nil). The package then shipped
// without a skeleton, merge re-serialized the document from the content model —
// losing the source's exact byte formatting — and both commands reported success.

// skelFailReader reads one translatable block, but fails the read as soon as a
// SkeletonStore is wired. That is exactly the divergence the old guard hid:
// ReadSourceBlocks (no skeleton) succeeds while captureSkeletonBytes (skeleton
// wired) does not.
type skelFailReader struct {
	format.BaseFormatReader
	skel *format.SkeletonStore
}

func newSkelFailReader() format.DataFormatReader {
	r := &skelFailReader{}
	r.FormatName = skelFailFormat
	r.FormatDisplayName = "Skeleton-capture failure fixture"
	return r
}

const skelFailFormat = "skelfail-fixture"

func (r *skelFailReader) Signature() format.FormatSignature {
	return format.FormatSignature{Extensions: []string{".skelfail"}}
}
func (r *skelFailReader) SetSkeletonStore(s *format.SkeletonStore) { r.skel = s }
func (r *skelFailReader) Open(context.Context, *model.RawDocument) error {
	return nil
}
func (r *skelFailReader) Close() error { return nil }

func (r *skelFailReader) Read(context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 2)
	if r.skel != nil {
		ch <- model.PartResult{Error: errors.New("skeleton emitter: structure could not be captured")}
		close(ch)
		return ch
	}
	b := mkBlock("greeting", "Hello world")
	b.Translatable = true
	ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: b}}
	close(ch)
	return ch
}

// newSkelFixtureApp returns an app whose registry knows the fixture format, plus a
// project context rooted at a temp dir holding one fixture source file.
func newSkelFixtureApp(t *testing.T) (*App, *project.ProjectContext, project.ResolvedFile, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	src := filepath.Join(dir, "en.skelfail")
	require.NoError(t, os.WriteFile(src, []byte("greeting=Hello world\n"), 0o644))

	a := &App{}
	a.InitRegistries()
	a.FormatReg.RegisterReader(skelFailFormat, newSkelFailReader,
		format.FormatSignature{Extensions: []string{".skelfail"}}, "Skeleton-capture failure fixture")
	a.SourceLang = "en"

	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "SkelFixture",
		Defaults: project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
	}
	pctx := project.NewProjectContext(proj, filepath.Join(dir, project.RecipeFileName))
	rf := project.ResolvedFile{Path: src, Relative: "en.skelfail", Format: skelFailFormat, Collection: "app"}
	return a, pctx, rf, dir
}

// TestExtractOneKpz_SkeletonCaptureFailureFailsTheExtract is the regression: the
// extract must refuse rather than ship a package that merge can only write back
// re-serialized.
func TestExtractOneKpz_SkeletonCaptureFailureFailsTheExtract(t *testing.T) {
	a, pctx, rf, dir := newSkelFixtureApp(t)
	out := filepath.Join(dir, "out.kpz")

	err := a.extractOneKpz(context.Background(), kpzInterchangeTask{
		ctx: pctx, source: rf, targetLocale: model.LocaleID("fr"), outputPath: out,
	})
	require.Error(t, err, "a skeleton that exists but could not be captured must fail the extract")
	assert.Contains(t, err.Error(), "capture round-trip skeleton")
	assert.Contains(t, err.Error(), "en.skelfail", "the message names the source it could not capture")

	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr),
		"no skeleton-less package is left behind for merge to degrade on")
}

// skelNoneReader has NO SetSkeletonStore method at all — the shape of a
// generative format that legitimately carries no round-trip skeleton.
type skelNoneReader struct {
	format.BaseFormatReader
}

const skelNoneFormat = "skelnone-fixture"

func newSkelNoneReader() format.DataFormatReader {
	r := &skelNoneReader{}
	r.FormatName = skelNoneFormat
	r.FormatDisplayName = "Skeleton-less fixture"
	return r
}

func (r *skelNoneReader) Signature() format.FormatSignature {
	return format.FormatSignature{Extensions: []string{".skelnone"}}
}
func (r *skelNoneReader) Open(context.Context, *model.RawDocument) error { return nil }
func (r *skelNoneReader) Close() error                                   { return nil }

func (r *skelNoneReader) Read(context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 1)
	b := mkBlock("greeting", "Hello world")
	b.Translatable = true
	ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: b}}
	close(ch)
	return ch
}

// TestCaptureSkeletonBytes_NoEmitterIsNotAnError is the discriminating control:
// genuine absence stays (nil, nil), so a format that legitimately has no skeleton
// still extracts. Without it, "fail on any capture miss" would break every
// skeleton-less format.
func TestCaptureSkeletonBytes_NoEmitterIsNotAnError(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	a.FormatReg.RegisterReader(skelNoneFormat, newSkelNoneReader,
		format.FormatSignature{Extensions: []string{".skelnone"}}, "Skeleton-less fixture")

	dir := t.TempDir()
	src := filepath.Join(dir, "en.skelnone")
	body := []byte("greeting=Hello world\n")
	require.NoError(t, os.WriteFile(src, body, 0o644))

	skel, err := captureSkeletonBytes(context.Background(), a.FormatReg, registry.FormatID(skelNoneFormat),
		src, body, model.LocaleID("en"))
	require.NoError(t, err, "a format with no skeleton emitter is absence, not failure")
	assert.Empty(t, skel)
}

// TestExtractOneKpz_SkeletonlessFormatStillExtracts is the end-to-end control for
// the same distinction: a format that records no skeleton still produces a package.
func TestExtractOneKpz_SkeletonlessFormatStillExtracts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	src := filepath.Join(dir, "en.skelnone")
	require.NoError(t, os.WriteFile(src, []byte("greeting=Hello world\n"), 0o644))

	a := &App{}
	a.InitRegistries()
	a.FormatReg.RegisterReader(skelNoneFormat, newSkelNoneReader,
		format.FormatSignature{Extensions: []string{".skelnone"}}, "Skeleton-less fixture")
	a.SourceLang = "en"
	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "SkelFixture",
		Defaults: project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
	}
	pctx := project.NewProjectContext(proj, filepath.Join(dir, project.RecipeFileName))
	rf := project.ResolvedFile{Path: src, Relative: "en.skelnone", Format: skelNoneFormat, Collection: "app"}
	out := filepath.Join(dir, "out.kpz")

	require.NoError(t, a.extractOneKpz(context.Background(), kpzInterchangeTask{
		ctx: pctx, source: rf, targetLocale: model.LocaleID("fr"), outputPath: out,
	}))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	pkg, err := kpz.Unmarshal(data)
	require.NoError(t, err)
	assert.Empty(t, pkg.Skeletons, "the format genuinely records none — and that is fine")
}
