package spec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// NativeRunner runs every Example in a Spec through a neokapi
// DataFormatReader and asserts the results match the spec's
// declarations. It's the always-on counterpart to the parity bridge
// runner: the spec describes WHAT the format does, NativeRunner
// verifies the Go reader honors that.
type NativeRunner struct {
	Spec *Spec

	// NewReader builds the reader for a given variant. The variant is
	// the empty string for monolithic formats. For multi-variant
	// formats (openxml, odf), implementors typically dispatch on the
	// variant id to set the appropriate ParseType / sub-reader.
	NewReader func(variant string) format.DataFormatReader

	// SourceLocale / TargetLocale default to "en"/"fr" if empty.
	SourceLocale string
	TargetLocale string
}

// Run drives every Feature × Example as a subtest. Each example is
// independent: failures don't cascade, the report shows per-example
// pass/fail.
func (r *NativeRunner) Run(t *testing.T) {
	t.Helper()
	if r.Spec == nil {
		t.Fatal("NativeRunner: Spec is nil")
	}
	if r.NewReader == nil {
		t.Fatal("NativeRunner: NewReader is nil")
	}
	for _, feat := range r.Spec.Features {
		feat := feat
		t.Run(feat.ID, func(t *testing.T) {
			for _, ex := range feat.Examples {
				ex := ex
				t.Run(ex.Name, func(t *testing.T) {
					r.runExample(t, feat, ex)
				})
			}
		})
	}
}

func (r *NativeRunner) runExample(t *testing.T, feat Feature, ex Example) {
	t.Helper()
	if ex.BridgeOnly {
		t.Skip("bridge_only example — skipped by native runner")
		return
	}
	input, err := r.resolveInput(ex)
	if err != nil {
		// Skip cleanly when an upstream-testdata fixture isn't fetched
		// — the spec itself is fine, the corpus just isn't available
		// on this machine. Spec authors get the same skip the existing
		// native tests use.
		if strings.HasPrefix(ex.InputFile, "okapi:") {
			t.Skipf("input not available: %v", err)
			return
		}
		t.Fatalf("resolve input: %v", err)
	}
	reader := r.NewReader(ex.Variant)
	if reader == nil {
		t.Fatalf("NewReader returned nil for variant %q", ex.Variant)
	}

	cfg := mergeConfig(feat.Config, ex.Config)
	if len(cfg) > 0 {
		c := reader.Config()
		if c == nil {
			t.Fatalf("config provided but reader has no Config()")
		}
		if err := c.ApplyMap(cfg); err != nil {
			t.Fatalf("apply config %v: %v", cfg, err)
		}
	}

	parts, err := readAll(reader, input)
	if err != nil {
		if ex.ExpectedFail != "" {
			t.Logf("expected_fail (%s): read error %v", ex.ExpectedFail, err)
			return
		}
		t.Fatalf("read: %v", err)
	}
	if ex.ExpectedFail != "" {
		// Buffer assertion failures via a sub-T so an xfail doesn't
		// break the build. Log the divergence; warn loudly if it has
		// quietly started passing.
		failed := runAssertionsCapture(parts, ex.Assertions)
		if len(failed) == 0 {
			t.Logf("expected_fail (%s): assertions now pass — remove the expected_fail tag", ex.ExpectedFail)
			return
		}
		for _, msg := range failed {
			t.Logf("expected_fail (%s): %s", ex.ExpectedFail, msg)
		}
		return
	}
	checkAssertions(t, parts, ex.Assertions)
}

// runAssertionsCapture mirrors checkAssertions but returns the failure
// messages instead of writing them to t. Used by ExpectedFail handling
// so xfailed examples don't break the build.
func runAssertionsCapture(parts []*model.Part, a Assertions) []string {
	var msgs []string
	report := func(format string, args ...any) {
		msgs = append(msgs, fmt.Sprintf(format, args...))
	}
	texts := blockTexts(parts)
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

func (r *NativeRunner) resolveInput(ex Example) ([]byte, error) {
	switch {
	case ex.InputFile != "":
		path, err := r.resolveFilePath(ex.InputFile)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	case ex.InputXML != "":
		return []byte(ex.InputXML), nil
	case len(ex.InputBytes) > 0:
		return ex.InputBytes, nil
	}
	return nil, fmt.Errorf("example has no input")
}

// resolveFilePath turns a spec-relative input_file value into an
// absolute filesystem path. Supports two schemes:
//
//   - "testdata/foo.docx" — relative to the spec.yaml directory.
//     Use for fixtures committed alongside the format.
//   - "okapi:path/under/okapi-testdata.docx" — resolved against the
//     latest version dir under <repo>/okapi-testdata/. Use for
//     upstream fixtures not committed to neokapi. Skipped (not failed)
//     when okapi-testdata isn't fetched, with a clear message.
func (r *NativeRunner) resolveFilePath(rel string) (string, error) {
	if strings.HasPrefix(rel, "okapi:") {
		base, err := findOkapiTestdataRoot()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, strings.TrimPrefix(rel, "okapi:")), nil
	}
	if filepath.IsAbs(rel) {
		return rel, nil
	}
	return filepath.Join(r.Spec.dir, rel), nil
}

// findOkapiTestdataRoot walks up from cwd to find go.work, then
// returns the path to the latest version dir under okapi-testdata/
// containing the canonical filter resources tree.
func findOkapiTestdataRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("okapi-testdata: could not find repo root (go.work) walking up from %s", dir)
		}
		dir = parent
	}
	base := filepath.Join(dir, "okapi-testdata")
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

func mergeConfig(base, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func readAll(reader format.DataFormatReader, input []byte) ([]*model.Part, error) {
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

// blockTexts extracts the source text of every Block part, joining
// segment runs with empty string. Non-Block parts are skipped.
func blockTexts(parts []*model.Part) []string {
	out := []string{}
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock {
			continue
		}
		blk, ok := p.Resource.(*model.Block)
		if !ok || !blk.Translatable {
			continue
		}
		var sb strings.Builder
		for _, seg := range blk.Source {
			for _, run := range seg.Runs {
				if run.Text != nil {
					sb.WriteString(run.Text.Text)
				}
			}
		}
		text := sb.String()
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func checkAssertions(t *testing.T, parts []*model.Part, a Assertions) {
	t.Helper()
	texts := blockTexts(parts)
	if a.BlockCount != nil && len(texts) != *a.BlockCount {
		t.Errorf("block_count: want %d, got %d (texts=%v)", *a.BlockCount, len(texts), texts)
	}
	if a.BlockCountMin != nil && len(texts) < *a.BlockCountMin {
		t.Errorf("block_count_min: want >= %d, got %d (texts=%v)", *a.BlockCountMin, len(texts), texts)
	}
	if a.BlockCountMax != nil && len(texts) > *a.BlockCountMax {
		t.Errorf("block_count_max: want <= %d, got %d (texts=%v)", *a.BlockCountMax, len(texts), texts)
	}
	if a.FirstBlockText != nil {
		if len(texts) == 0 {
			t.Errorf("first_block_text: want %q, got no blocks", *a.FirstBlockText)
		} else if texts[0] != *a.FirstBlockText {
			t.Errorf("first_block_text: want %q, got %q", *a.FirstBlockText, texts[0])
		}
	}
	if len(a.BlockTexts) > 0 {
		if len(texts) != len(a.BlockTexts) {
			t.Errorf("block_texts: want %d blocks, got %d (got=%v)", len(a.BlockTexts), len(texts), texts)
		} else {
			for i, want := range a.BlockTexts {
				if texts[i] != want {
					t.Errorf("block_texts[%d]: want %q, got %q", i, want, texts[i])
				}
			}
		}
	}
	for _, want := range a.HasBlockWithText {
		if !containsText(texts, want) {
			t.Errorf("has_block_with_text: %q not found in %v", want, texts)
		}
	}
	for _, unwanted := range a.NoBlockWithText {
		if containsText(texts, unwanted) {
			t.Errorf("no_block_with_text: %q unexpectedly present in %v", unwanted, texts)
		}
	}
}

func containsText(texts []string, needle string) bool {
	for _, t := range texts {
		if strings.Contains(t, needle) {
			return true
		}
	}
	return false
}
