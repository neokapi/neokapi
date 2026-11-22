package formats

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// TestWriterIsAFixedPoint asserts that writing a document the writer itself
// produced changes nothing. Every format with a reader and a writer is read
// and written three times over each of its sample files; the second and third
// outputs must be byte-identical.
//
// The first pass may normalize. A writer whose output still moves on the next
// pass rewrites the file on every run over an unchanged project.
//
// A sample is a file that carries one of the format's extensions, from its
// corpus manifest (corpus.yaml) or from the testdata directory of the package
// that implements its reader. Every format with a writer must have one, and
// every sample must read. Fuzz seeds are not documents. A format whose reader
// refuses every document by design (format.WriteOnlyReader) has nothing to
// read back; the test checks that its reader does refuse to open one.
func TestWriterIsAFixedPoint(t *testing.T) {
	t.Parallel()
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	samples := fixedPointSamples(t, reg)
	for _, id := range reg.WriterNames() {
		if !reg.HasReader(id) {
			continue
		}
		if writeOnly(reg, id) {
			// The marker is checked, not trusted: a reader that declares it
			// must refuse to open a document.
			t.Run(string(id), func(t *testing.T) {
				r, err := reg.NewReader(id)
				if err != nil {
					t.Fatal(err)
				}
				doc := &model.RawDocument{URI: "sample", Reader: io.NopCloser(bytes.NewReader([]byte("x")))}
				if err := r.Open(t.Context(), doc); err == nil {
					_ = r.Close()
					t.Fatalf("%s declares format.WriteOnlyReader but its reader opened a document", id)
				}
			})
			continue
		}
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			if len(samples[string(id)]) == 0 {
				t.Fatalf("%s has a writer and no sample file; add a small real document to the testdata of the package that reads it", id)
			}
			for _, path := range samples[string(id)] {
				t.Run(path, func(t *testing.T) {
					src, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("read sample: %v", err)
					}
					ext := filepath.Ext(path)
					out1, err := writePass(t, reg, id, ext, src)
					if err != nil {
						t.Fatalf("first pass: %v", err)
					}
					out2, err := writePass(t, reg, id, ext, out1)
					if err != nil {
						t.Fatalf("second pass over the writer's own output: %v\noutput: %q", err, clip(out1))
					}
					out3, err := writePass(t, reg, id, ext, out2)
					if err != nil {
						t.Fatalf("third pass: %v", err)
					}
					if !bytes.Equal(out2, out3) {
						t.Fatalf("%s: the writer's output moves on every pass\nsecond: %q\nthird:  %q\nfirst difference at byte %d",
							id, clip(out2), clip(out3), firstDifference(out2, out3))
					}
				})
			}
		})
	}
}

// writeOnly reports whether id's reader declares that it reads nothing.
func writeOnly(reg *registry.FormatRegistry, id registry.FormatID) bool {
	r, err := reg.NewReader(id)
	if err != nil {
		return false
	}
	_, ok := r.(format.WriteOnlyReader)
	return ok
}

// moduleRoot is the repository root relative to this package.
const moduleRoot = "../.."

// readerPackageDir returns the directory, relative to this package, of the
// package that implements id's reader.
func readerPackageDir(reg *registry.FormatRegistry, id registry.FormatID) (string, bool) {
	r, err := reg.NewReader(id)
	if err != nil {
		return "", false
	}
	typ := reflect.TypeOf(r)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	rel, ok := strings.CutPrefix(typ.PkgPath(), "github.com/neokapi/neokapi/")
	if !ok {
		return "", false
	}
	return filepath.Join(moduleRoot, filepath.FromSlash(rel)), true
}

// formatExtensions returns the extensions id declares: the ones detection
// uses and the ones its reader names for itself (a format can decline
// detection by extension, as Qt TS does to stay clear of TypeScript).
func formatExtensions(reg *registry.FormatRegistry, id registry.FormatID) []string {
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

// fixedPointSamples maps a format id to its sample files, as paths relative
// to this package.
func fixedPointSamples(t *testing.T, reg *registry.FormatRegistry) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	seen := map[string]bool{}
	add := func(id, path string) {
		path = filepath.Clean(path)
		if seen[id+"\x00"+path] {
			return
		}
		seen[id+"\x00"+path] = true
		out[id] = append(out[id], path)
	}
	isSample := func(id registry.FormatID, path string) bool {
		if strings.Contains(filepath.ToSlash(path), "/testdata/fuzz/") {
			return false
		}
		return slices.ContainsFunc(formatExtensions(reg, id), func(ext string) bool {
			return strings.EqualFold(filepath.Ext(path), ext)
		})
	}

	manifests, err := filepath.Glob(filepath.Join("*", "corpus.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Format  string `yaml:"format"`
			Entries []struct {
				Path string `yaml:"path"`
			} `yaml:"entries"`
		}
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		id := registry.FormatID(manifest.Format)
		for _, e := range manifest.Entries {
			// Manifest paths are repository-relative.
			if path := filepath.Join(moduleRoot, filepath.FromSlash(e.Path)); isSample(id, path) {
				add(manifest.Format, path)
			}
		}
	}

	for _, id := range reg.WriterNames() {
		dir, ok := readerPackageDir(reg, id)
		if !ok {
			continue
		}
		testdata := filepath.Join(dir, "testdata")
		if _, err := os.Stat(testdata); err != nil {
			continue
		}
		err := filepath.WalkDir(testdata, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && isSample(id, path) {
				add(string(id), path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out
}

// writePass reads src with a fresh reader and writes it back untouched with a
// fresh writer, wired the way a same-format write-back is. The document is a
// file named like the sample, as a run reads one: a media format's reader
// refers to the file it read and its writer copies from it, so the reader
// stays open until the writer is done.
func writePass(t *testing.T, reg *registry.FormatRegistry, id registry.FormatID, ext string, src []byte) ([]byte, error) {
	t.Helper()
	reader, err := reg.NewReader(id)
	if err != nil {
		return nil, fmt.Errorf("new reader: %w", err)
	}
	writer, err := reg.NewWriter(id)
	if err != nil {
		return nil, fmt.Errorf("new writer: %w", err)
	}
	store, err := format.NewWiredSkeleton(reader, writer)
	if err != nil {
		return nil, fmt.Errorf("wire skeleton: %w", err)
	}
	if store != nil {
		defer store.Close()
	}
	path := filepath.Join(t.TempDir(), "sample"+ext)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(src)),
	}
	if err := format.OpenDocument(ctx, reader, doc); err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer reader.Close()
	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read: %w", pr.Error)
		}
		parts = append(parts, pr.Part)
	}
	return spec.WriteParts(writer, parts, src)
}

func firstDifference(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func clip(b []byte) string {
	const limit = 400
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "..."
}
