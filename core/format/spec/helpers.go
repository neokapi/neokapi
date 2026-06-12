package spec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// ResolveInput returns the input bytes for an Example by reading
// InputFile (relative to the Spec dir, under okapi-testdata when
// prefixed with "okapi:", or under the fetched Tier B corpus when
// prefixed with "corpus:") or by returning InputXML / InputBytes
// directly. Used by both the native and parity runners.
func ResolveInput(s *Spec, ex Example) ([]byte, error) {
	switch {
	case ex.InputFile != "":
		path, err := ResolveFilePath(s, ex.InputFile)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	case ex.InputXML != "":
		return []byte(ex.InputXML), nil
	case len(ex.InputBytes) > 0:
		return ex.InputBytes, nil
	}
	return nil, errors.New("example has no input")
}

// ResolveFilePath turns a spec-relative input_file value into an
// absolute filesystem path. Supported schemes:
//
//   - "okapi:<relpath>"  — under the fetched okapi-testdata root
//     (FindOkapiTestdataRoot)
//   - "corpus:<relpath>" — under the fetched Tier B corpus root
//     (FindCorpusRoot)
//   - absolute paths     — returned as-is
//   - anything else      — relative to the spec.yaml directory
func ResolveFilePath(s *Spec, rel string) (string, error) {
	if strings.HasPrefix(rel, "okapi:") {
		base, err := FindOkapiTestdataRoot()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, strings.TrimPrefix(rel, "okapi:")), nil
	}
	if strings.HasPrefix(rel, "corpus:") {
		base, err := FindCorpusRoot()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, strings.TrimPrefix(rel, "corpus:")), nil
	}
	if filepath.IsAbs(rel) {
		return rel, nil
	}
	return filepath.Join(s.dir, rel), nil
}

// findRepoRoot walks up from cwd to the directory containing go.work
// (the repository root coordinating the multi-module workspace).
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (go.work) walking up from %s", dir)
		}
		dir = parent
	}
}

// FindOkapiTestdataRoot walks up from cwd to find go.work, then
// returns the path to the latest version dir under okapi-testdata/.
// Returns an error when not found so callers can decide between skip
// and fail.
func FindOkapiTestdataRoot() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", fmt.Errorf("okapi-testdata: %w", err)
	}
	base := filepath.Join(root, "okapi-testdata")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("okapi-testdata not found at %s — run scripts/fetch-okapi-testdata.sh", base)
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", fmt.Errorf("okapi-testdata: no version directories under %s", base)
	}
	return filepath.Join(base, latest), nil
}

// corpusTagPrefix is the release-tag prefix of the Tier B corpus store
// (release format-corpus-vN, fetched into corpus/<tag>/<id>/ by
// scripts/fetch-corpus.sh).
const corpusTagPrefix = "format-corpus-v"

// FindCorpusRoot walks up from cwd to find go.work, then returns the
// path to the lexically-latest format-corpus-v* version dir under
// corpus/. Returns an error when not found so callers can decide
// between skip and fail; the error names `make fetch-corpus` so a
// test skip message tells the reader how to stage the corpus.
func FindCorpusRoot() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", fmt.Errorf("corpus: %w", err)
	}
	base := filepath.Join(root, "corpus")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("corpus not found at %s — run `make fetch-corpus`", base)
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), corpusTagPrefix) && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", fmt.Errorf("corpus: no %s* version directories under %s — run `make fetch-corpus`", corpusTagPrefix, base)
	}
	return filepath.Join(base, latest), nil
}

// MergeConfig returns base overlaid with overlay; nil if both empty.
func MergeConfig(base, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(overlay))
	maps.Copy(out, base)
	maps.Copy(out, overlay)
	return out
}

// ReadParts drives a reader through Open → Read → Close and returns
// the streamed parts. Used by the native spec runner; the parity
// runner reuses it for its native side.
func ReadParts(reader format.DataFormatReader, input []byte) ([]*model.Part, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	doc := &model.RawDocument{
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer reader.Close()
	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read part: %w", pr.Error)
		}
		parts = append(parts, pr.Part)
	}
	return parts, nil
}

// BlockTexts extracts the joined source text of every translatable
// Block in parts, skipping non-Block parts and untranslatable blocks.
func BlockTexts(parts []*model.Part) []string {
	out := []string{}
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock {
			continue
		}
		blk, ok := p.Resource.(*model.Block)
		if !ok || !blk.Translatable {
			continue
		}
		text := model.RunsText(blk.Source)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

// EvalAssertions evaluates Assertions against parts and returns a list
// of failure messages (empty when all pass). Used by the runners to
// decide whether to fail the test or record an expected_fail outcome.
func EvalAssertions(parts []*model.Part, a Assertions) []string {
	var msgs []string
	report := func(format string, args ...any) {
		msgs = append(msgs, fmt.Sprintf(format, args...))
	}
	texts := BlockTexts(parts)
	if a.BlockCount != nil && len(texts) != *a.BlockCount {
		report("block_count: want %d, got %d", *a.BlockCount, len(texts))
	}
	if a.BlockCountMin != nil && len(texts) < *a.BlockCountMin {
		report("block_count_min: want >= %d, got %d", *a.BlockCountMin, len(texts))
	}
	if a.BlockCountMax != nil && len(texts) > *a.BlockCountMax {
		report("block_count_max: want <= %d, got %d", *a.BlockCountMax, len(texts))
	}
	if a.FirstBlockText != nil {
		switch {
		case len(texts) == 0:
			report("first_block_text: want %q, got no blocks", *a.FirstBlockText)
		case texts[0] != *a.FirstBlockText:
			report("first_block_text: want %q, got %q", *a.FirstBlockText, texts[0])
		}
	}
	if len(a.BlockTexts) > 0 {
		if len(texts) != len(a.BlockTexts) {
			report("block_texts: want %d blocks, got %d", len(a.BlockTexts), len(texts))
		} else {
			for i, want := range a.BlockTexts {
				if texts[i] != want {
					report("block_texts[%d]: want %q, got %q", i, want, texts[i])
				}
			}
		}
	}
	for _, want := range a.HasBlockWithText {
		if !containsText(texts, want) {
			report("has_block_with_text: %q not found", want)
		}
	}
	for _, unwanted := range a.NoBlockWithText {
		if containsText(texts, unwanted) {
			report("no_block_with_text: %q unexpectedly present", unwanted)
		}
	}
	return msgs
}

func containsText(texts []string, needle string) bool {
	for _, t := range texts {
		if strings.Contains(t, needle) {
			return true
		}
	}
	return false
}
