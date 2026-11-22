package formats

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/registry"
)

// TestWriterIsAFixedPoint asserts that writing a document the writer itself
// produced changes nothing. Every format with a reader and a writer is read
// and written three times over each of its sample files; the second and third
// outputs must be byte-identical.
//
// The first pass may normalize (the round-trip and fidelity tests judge that
// against the source). A writer whose output still moves on the next pass has
// no stable form, so every run of the loop over an unchanged project rewrites
// the file, and the diff never settles.
//
// The samples are of two kinds. A corpus manifest entry (corpus.yaml) that
// carries one of the format's registered extensions must read. A file under
// the format's testdata directory may not: one the reader rejects is a
// malformed-input fixture or another format's file, with no output to hold
// fixed. Fuzz seeds are not documents.
func TestWriterIsAFixedPoint(t *testing.T) {
	t.Parallel()
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	samples := fixedPointSamples(t, reg)
	tested := 0
	var noSamples []string
	for _, id := range reg.WriterNames() {
		if !reg.HasReader(id) {
			continue
		}
		if len(samples[string(id)]) == 0 {
			noSamples = append(noSamples, string(id))
			continue
		}
		tested++
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			for _, s := range samples[string(id)] {
				t.Run(s.path, func(t *testing.T) {
					src, err := os.ReadFile(s.path)
					if err != nil {
						t.Fatalf("read sample: %v", err)
					}
					out1, err := writePass(reg, id, src)
					if err != nil {
						if !s.mustRead {
							t.Skipf("not a readable %s document: %v", id, err)
						}
						t.Fatalf("first pass: %v", err)
					}
					out2, err := writePass(reg, id, out1)
					if err != nil {
						t.Fatalf("second pass over the writer's own output: %v\noutput: %q", err, clip(out1))
					}
					out3, err := writePass(reg, id, out2)
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
	if tested == 0 {
		t.Fatal("no format with a writer has a sample file")
	}
	t.Logf("%d formats checked; no sample file for: %s", tested, strings.Join(noSamples, ", "))
}

type fixedPointSample struct {
	path     string // relative to this package
	mustRead bool
}

// fixedPointSamples maps a format id to its sample files.
func fixedPointSamples(t *testing.T, reg *registry.FormatRegistry) map[string][]fixedPointSample {
	t.Helper()
	out := map[string][]fixedPointSample{}
	seen := map[string]bool{}
	add := func(id, path string, mustRead bool) {
		if seen[id+"\x00"+path] {
			return
		}
		seen[id+"\x00"+path] = true
		out[id] = append(out[id], fixedPointSample{path: path, mustRead: mustRead})
	}

	isFuzzSeed := func(path string) bool {
		return strings.Contains(filepath.ToSlash(path), "/testdata/fuzz/")
	}
	hasExtension := func(id, path string) bool {
		info := reg.FormatInfo(registry.FormatID(id))
		if info == nil || isFuzzSeed(path) {
			return false
		}
		return slices.ContainsFunc(info.Extensions, func(ext string) bool {
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
		for _, e := range manifest.Entries {
			// Manifest paths are repository-relative; this package is core/formats.
			if path := strings.TrimPrefix(e.Path, "core/formats/"); hasExtension(manifest.Format, path) {
				add(manifest.Format, path, true)
			}
		}
	}

	for _, info := range reg.FormatInfos() {
		if !info.HasWriter {
			continue
		}
		dir := filepath.Join(string(info.Name), "testdata")
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && !isFuzzSeed(path) {
				add(string(info.Name), path, false)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for id := range out {
		slices.SortFunc(out[id], func(a, b fixedPointSample) int { return strings.Compare(a.path, b.path) })
	}
	return out
}

// writePass reads src with a fresh reader and writes it back untouched with a
// fresh writer, wired the way a same-format write-back is.
func writePass(reg *registry.FormatRegistry, id registry.FormatID, src []byte) ([]byte, error) {
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
	parts, err := spec.ReadParts(reader, src)
	if err != nil {
		return nil, err
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
