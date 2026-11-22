package host

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// TestConvSameFormatWritesTheDocument asserts that `kconv FILE --to <its own
// format>` writes the document through the format's writer. For every format
// with a reader and a writer, each sample file is converted to its own format
// and the output must equal what the format's writer produces from the same
// read. A conversion that falls back to another writer (the structural block
// listing, say) fails here.
//
// Samples are found as core/formats' TestWriterIsAFixedPoint finds them: the
// format's corpus manifest and the testdata of the package that implements
// its reader. A format whose reader refuses every document by design
// (format.WriteOnlyReader) must make kconv fail with an error.
//
// No plugin is discovered, so the run is the same on every machine. A sample
// the format's own reader cannot read without one (audio needs the asr
// plugin to transcribe) must make kconv fail with that reader's error.
func TestConvSameFormatWritesTheDocument(t *testing.T) {
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_ASR_PLUGIN", "")
	reg := newToolboxApp(t).FormatReg
	samples := sameFormatSamples(t, reg)
	for _, id := range reg.WriterNames() {
		if !reg.HasReader(id) {
			continue
		}
		t.Run(string(id), func(t *testing.T) {
			app := newToolboxApp(t)
			app.FormatFlag = string(id)
			to, err := app.ResolveTargetFormat(string(id), "")
			require.NoError(t, err)
			require.Equal(t, id, to)

			if convWriteOnly(reg, id) {
				in := filepath.Join(t.TempDir(), "sample"+app.formatExt(id))
				require.NoError(t, os.WriteFile(in, []byte("x"), 0o644))
				err := app.convertDocument(context.Background(), in, id, "", filepath.Join(t.TempDir(), "out"))
				require.ErrorContains(t, err, "does not read them")
				return
			}
			require.NotEmpty(t, samples[id], "%s has a writer and no sample file", id)
			for _, sample := range samples[id] {
				t.Run(filepath.Base(sample), func(t *testing.T) {
					src, err := os.ReadFile(sample)
					require.NoError(t, err)
					dir := t.TempDir()
					in := filepath.Join(dir, "in"+filepath.Ext(sample))
					require.NoError(t, os.WriteFile(in, src, 0o644))
					out := filepath.Join(dir, "out"+filepath.Ext(sample))
					// The image format reads a caption from "<image>.alt.txt"
					// and writes the caption beside its output under the same
					// convention, so each image sample gets one.
					sidecar := id == "image"
					if sidecar {
						require.NoError(t, os.WriteFile(in+".alt.txt", []byte("A colour swatch\n"), 0o644))
					}

					want, readErr := formatWriterOutput(t, reg, id, in, src)
					convErr := app.convertDocument(context.Background(), in, id, "", out)
					if readErr != nil {
						require.Error(t, convErr, "the %s reader cannot read this sample (%v), so kconv must fail", id, readErr)
						require.Equal(t, readErr.Error(), convErr.Error())
						return
					}
					require.NoError(t, convErr)
					got, err := os.ReadFile(out)
					require.NoError(t, err)
					if !bytes.Equal(want, got) {
						t.Fatalf("kconv --to %s did not write the document through the %s writer\nwant: %q\ngot:  %q",
							id, id, clipBytes(want), clipBytes(got))
					}
					if sidecar {
						alt, err := os.ReadFile(out + ".alt.txt")
						require.NoError(t, err, "the caption is written beside the output under its name")
						assert.Equal(t, "A colour swatch\n", string(alt))
					}
				})
			}
		})
	}
}

// TestConvToAReadOnlyFormatSaysSo asserts that converting to a format kapi
// reads and cannot write fails with an error that says so, whether the
// format is named by --to or by the -o extension.
func TestConvToAReadOnlyFormatSaysSo(t *testing.T) {
	app := newToolboxApp(t)
	var readOnly []registry.FormatID
	for _, id := range app.FormatReg.ReaderNames() {
		if !app.FormatReg.HasWriter(id) {
			readOnly = append(readOnly, id)
		}
	}
	require.NotEmpty(t, readOnly, "no read-only format is registered to test with")
	for _, id := range readOnly {
		t.Run(string(id), func(t *testing.T) {
			_, err := app.ResolveTargetFormat(string(id), "")
			require.ErrorContains(t, err, "has no writer")
			if ext := app.formatExt(id); ext != "" && app.readOnlyFormat("", ext) != "" {
				_, err := app.ResolveTargetFormat("", "out"+ext)
				require.ErrorContains(t, err, "has no writer")
			}
		})
	}
}

// formatWriterOutput reads the file at path with id's reader and writes it
// back unedited with id's writer, wired for a same-format write-back. The
// error is the reader's, when it cannot read the file.
func formatWriterOutput(t *testing.T, reg *registry.FormatRegistry, id registry.FormatID, path string, src []byte) ([]byte, error) {
	t.Helper()
	reader, err := reg.NewReader(id)
	require.NoError(t, err)
	writer, err := reg.NewWriter(id)
	require.NoError(t, err)
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	if store != nil {
		defer store.Close()
	}
	if sps, ok := writer.(format.SourcePathSetter); ok {
		sps.SetSourcePath(path)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(src)),
	}
	require.NoError(t, format.OpenDocument(ctx, reader, doc))
	defer reader.Close()
	var parts []*model.Part
	var readErr error
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			readErr = pr.Error
			continue
		}
		parts = append(parts, pr.Part)
	}
	if readErr != nil {
		return nil, readErr
	}
	writer.SetEncoding("UTF-8")
	out, err := spec.WriteParts(writer, parts, src)
	require.NoError(t, err)
	return out, nil
}

func convWriteOnly(reg *registry.FormatRegistry, id registry.FormatID) bool {
	r, err := reg.NewReader(id)
	if err != nil {
		return false
	}
	_, ok := r.(format.WriteOnlyReader)
	return ok
}

// repoRoot is the repository root relative to this package.
const repoRoot = ".."

// sameFormatSamples maps each format with a writer to its sample files.
func sameFormatSamples(t *testing.T, reg *registry.FormatRegistry) map[registry.FormatID][]string {
	t.Helper()
	out := map[registry.FormatID][]string{}
	type sample struct {
		id   registry.FormatID
		path string
	}
	seen := map[sample]bool{}
	add := func(id registry.FormatID, path string) {
		path = filepath.Clean(path)
		if !seen[sample{id, path}] {
			seen[sample{id, path}] = true
			out[id] = append(out[id], path)
		}
	}
	isSample := func(id registry.FormatID, path string) bool {
		if strings.Contains(filepath.ToSlash(path), "/testdata/fuzz/") {
			return false
		}
		return slices.ContainsFunc(sampleExtensions(reg, id), func(ext string) bool {
			return strings.EqualFold(filepath.Ext(path), ext)
		})
	}

	manifests, err := filepath.Glob(filepath.Join(repoRoot, "core", "formats", "*", "corpus.yaml"))
	require.NoError(t, err)
	for _, m := range manifests {
		data, err := os.ReadFile(m)
		require.NoError(t, err)
		var manifest struct {
			Format  string `yaml:"format"`
			Entries []struct {
				Path string `yaml:"path"`
			} `yaml:"entries"`
		}
		require.NoError(t, yaml.Unmarshal(data, &manifest), m)
		id := registry.FormatID(manifest.Format)
		for _, e := range manifest.Entries {
			if path := filepath.Join(repoRoot, filepath.FromSlash(e.Path)); isSample(id, path) {
				add(id, path)
			}
		}
	}

	for _, id := range reg.WriterNames() {
		r, err := reg.NewReader(id)
		if err != nil {
			continue
		}
		typ := reflect.TypeOf(r)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		rel, ok := strings.CutPrefix(typ.PkgPath(), "github.com/neokapi/neokapi/")
		if !ok {
			continue
		}
		testdata := filepath.Join(repoRoot, filepath.FromSlash(rel), "testdata")
		if _, err := os.Stat(testdata); err != nil {
			continue
		}
		require.NoError(t, filepath.WalkDir(testdata, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && isSample(id, path) {
				add(id, path)
			}
			return nil
		}))
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out
}

// sampleExtensions returns the extensions id declares for detection and the
// ones its reader names for itself.
func sampleExtensions(reg *registry.FormatRegistry, id registry.FormatID) []string {
	var exts []string
	if info := reg.FormatInfo(id); info != nil {
		exts = append(exts, info.Extensions...)
	}
	if r, err := reg.NewReader(id); err == nil {
		v := reflect.ValueOf(r)
		for v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.Kind() == reflect.Struct {
			if f := v.FieldByName("FormatExtensions"); f.IsValid() {
				if own, ok := reflect.TypeAssert[[]string](f); ok {
					exts = append(exts, own...)
				}
			}
		}
	}
	return exts
}

func clipBytes(b []byte) string {
	const limit = 400
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "..."
}
