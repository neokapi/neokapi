package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverFixtures walks okapiTestdataRoot using the ParityCorpus()
// per-format spec and returns one TestFile per discovered file.
// Mirrors the harness's discoverFiles() so the bench measures the
// same corpus the parity suite exercises end-to-end.
//
// Files are returned sorted by (format, basename) for stable ordering.
func DiscoverFixtures(okapiTestdataRoot string) ([]TestFile, error) {
	if okapiTestdataRoot == "" {
		return nil, fmt.Errorf("okapi-testdata root is required")
	}
	if info, err := os.Stat(okapiTestdataRoot); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("okapi-testdata root %q not a directory: %v", okapiTestdataRoot, err)
	}

	var out []TestFile
	for _, cf := range ParityCorpus() {
		paths, err := discoverFormat(okapiTestdataRoot, cf)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			info, err := os.Stat(p)
			if err != nil {
				continue
			}
			out = append(out, TestFile{
				Name:        filepath.Base(p),
				Format:      cf.FormatID,
				Category:    sizeCategory(info.Size()),
				SourcePath:  p,
				SizeBytes:   info.Size(),
				FilterClass: cf.FilterClass,
				OkapiFprm:   cf.OkapiParamConfig,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Format != out[j].Format {
			return out[i].Format < out[j].Format
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func discoverFormat(root string, cf CorpusFormat) ([]string, error) {
	if len(cf.ExplicitFiles) > 0 {
		out := make([]string, 0, len(cf.ExplicitFiles))
		for _, rel := range cf.ExplicitFiles {
			abs := filepath.Join(root, rel)
			if _, err := os.Stat(abs); err == nil {
				out = append(out, abs)
			}
		}
		sort.Strings(out)
		return out, nil
	}

	extSet := map[string]bool{}
	for _, e := range cf.Extensions {
		extSet[strings.ToLower(e)] = true
	}

	var out []string
	for _, src := range cf.Sources {
		srcAbs := filepath.Join(root, src)
		info, err := os.Stat(srcAbs)
		if err != nil || !info.IsDir() {
			// Source dir missing in this tarball — skip rather than fatal.
			// Some upstream layouts have shifted; the bench shouldn't fail
			// on coverage gaps.
			continue
		}
		err = filepath.WalkDir(srcAbs, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if !cf.Recurse && path != srcAbs {
					return filepath.SkipDir
				}
				return nil
			}
			if extSet[strings.ToLower(filepath.Ext(path))] {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// sizeCategory buckets a file by byte size — keeps the existing
// tiny/small/medium/large vocabulary the report HTML already groups by.
func sizeCategory(size int64) string {
	switch {
	case size < 5*1024:
		return "tiny"
	case size < 50*1024:
		return "small"
	case size < 500*1024:
		return "medium"
	default:
		return "large"
	}
}

// Sample returns a deterministic subset of fixtures spread across
// formats and size categories. fraction is clamped to (0,1]; pass 1.0
// for the full set. Each format gets at least one sample so no format
// disappears from the report.
//
// Within each format the sample is taken at evenly-spaced indices in
// size-sorted order, so the run covers the small-to-large spread for
// every format rather than (e.g.) all-tiny or all-large.
func Sample(fixtures []TestFile, fraction float64) []TestFile {
	if fraction >= 1.0 || len(fixtures) == 0 {
		return fixtures
	}
	if fraction <= 0 {
		fraction = 0.1
	}

	// Group by format.
	byFormat := map[string][]TestFile{}
	var formatOrder []string
	seen := map[string]bool{}
	for _, f := range fixtures {
		if !seen[f.Format] {
			formatOrder = append(formatOrder, f.Format)
			seen[f.Format] = true
		}
		byFormat[f.Format] = append(byFormat[f.Format], f)
	}

	out := make([]TestFile, 0, int(float64(len(fixtures))*fraction)+len(formatOrder))
	for _, format := range formatOrder {
		group := byFormat[format]
		// Sort by size so even-spaced indices cover the size range.
		sort.Slice(group, func(i, j int) bool { return group[i].SizeBytes < group[j].SizeBytes })

		want := int(float64(len(group)) * fraction)
		if want < 1 {
			want = 1
		}
		if want > len(group) {
			want = len(group)
		}

		// Even-spaced indices: 0, n/want, 2n/want, …, (want-1)*n/want.
		// This always picks the smallest and (when want>1) reaches near
		// the largest, hitting the size spread.
		for i := 0; i < want; i++ {
			idx := i * len(group) / want
			out = append(out, group[idx])
		}
	}

	// Stable order independent of map iteration so reports diff cleanly
	// between runs of the same sample.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Format != out[j].Format {
			return out[i].Format < out[j].Format
		}
		return out[i].Name < out[j].Name
	})
	return out
}
