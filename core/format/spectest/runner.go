// Package spectest provides test helpers for driving a format Spec
// through a neokapi DataFormatReader. It is a sibling to core/format/spec
// and lives in a separate package so that importing testing does not taint
// the core/format/spec package's normal (non-test) build graph.
//
// Callers import this package only from *_test.go files:
//
//	import "github.com/neokapi/neokapi/core/format/spectest"
package spectest

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// NativeRunner runs every Example in a Spec through a neokapi
// DataFormatReader and asserts the results match the spec's
// declarations. It's the always-on counterpart to the parity bridge
// runner: the spec describes WHAT the format does, NativeRunner
// verifies the Go reader honors that.
//
// It evaluates the multi-view expected model (format-spec-cases.md §5):
//   - expected.extracted (or a legacy example's inline Assertions) — the
//     source-text view, always checked;
//   - expected.blocks — the canonical block-event dump, compared structurally;
//   - expected.roundtrip — writer output (byte_exact | idempotent | normalized),
//     checked only when NewWriter is wired;
//   - class: invalid — asserts the reader surfaces a clean error (no panic).
//
// A view that is absent on a case is simply not checked, so existing specs
// (inline assertions only) behave exactly as before.
type NativeRunner struct {
	Spec *spec.Spec

	// NewReader builds the reader for a given variant. The variant is
	// the empty string for monolithic formats. For multi-variant
	// formats (openxml, odf), implementors typically dispatch on the
	// variant id to set the appropriate ParseType / sub-reader.
	NewReader func(variant string) format.DataFormatReader

	// NewWriter builds the writer for a given variant, mirroring
	// NewReader. Optional: only needed when a case declares an
	// expected.roundtrip view. The hook follows the same factory pattern
	// as cli/parity's FormatSpec.NewWriter.
	NewWriter func(variant string) format.DataFormatWriter
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
		t.Run(feat.ID, func(t *testing.T) {
			for _, ex := range feat.Examples {
				t.Run(ex.Name, func(t *testing.T) {
					r.runExample(t, feat, ex)
				})
			}
		})
	}
}

func (r *NativeRunner) runExample(t *testing.T, feat spec.Feature, ex spec.Example) {
	t.Helper()
	if ex.BridgeOnly {
		t.Skip("bridge_only example — skipped by native runner")
		return
	}
	input, err := spec.ResolveInput(r.Spec, ex)
	if err != nil {
		// Skip cleanly when an upstream-testdata fixture isn't fetched
		// — the spec itself is fine, the corpus just isn't available.
		if strings.HasPrefix(ex.InputFile, "okapi:") {
			t.Skipf("input not available: %v", err)
			return
		}
		t.Fatalf("resolve input: %v", err)
	}

	// class: invalid takes a distinct path — the reader must reject the
	// input cleanly. Never reaches the assertion/blocks/roundtrip views.
	if ex.CaseClass() == spec.ClassInvalid {
		r.runInvalid(t, ex, input)
		return
	}

	parts, err := r.read(feat, ex, input)
	if err != nil {
		if ex.ExpectedFail != "" {
			t.Logf("expected_fail (%s): read error %v", ex.ExpectedFail, err)
			return
		}
		t.Fatalf("read: %v", err)
	}

	// Collect failures across every applicable view, then apply the
	// expected_fail downgrade uniformly.
	var failed []string

	// extracted view — today's source-text assertions (inline or
	// expected.extracted). Always evaluated; empty assertions check nothing.
	failed = append(failed, spec.EvalAssertions(parts, ex.ExtractedAssertions())...)

	// blocks view — the canonical block-event dump.
	if ex.Expected != nil && ex.Expected.Blocks != "" {
		failed = append(failed, r.checkBlocks(t, ex, parts)...)
	}

	// roundtrip view — writer output.
	if ex.Expected != nil && ex.Expected.Roundtrip != nil {
		failed = append(failed, r.checkRoundtrip(t, feat, ex, input, parts)...)
	}

	if ex.ExpectedFail != "" {
		if len(failed) == 0 {
			t.Logf("expected_fail (%s): assertions now pass — remove the expected_fail tag", ex.ExpectedFail)
			return
		}
		for _, msg := range failed {
			t.Logf("expected_fail (%s): %s", ex.ExpectedFail, msg)
		}
		return
	}
	for _, msg := range failed {
		t.Errorf("%s", msg)
	}
}

// read builds the reader for the example's variant, applies the merged
// config, and streams the parts.
func (r *NativeRunner) read(feat spec.Feature, ex spec.Example, input []byte) ([]*model.Part, error) {
	reader := r.NewReader(ex.Variant)
	if reader == nil {
		return nil, fmt.Errorf("NewReader returned nil for variant %q", ex.Variant)
	}
	cfg := spec.MergeConfig(feat.Config, ex.Config)
	if len(cfg) > 0 {
		c := reader.Config()
		if c == nil {
			return nil, errors.New("config provided but reader has no Config()")
		}
		if err := c.ApplyMap(cfg); err != nil {
			return nil, fmt.Errorf("apply config %v: %w", cfg, err)
		}
	}
	return spec.ReadParts(reader, input)
}

// runInvalid asserts the reader rejects malformed input cleanly: no panic,
// and either Open fails or a PartResult carries an error. Accept-mode never
// rewrites these (the tree-sitter guard rail).
func (r *NativeRunner) runInvalid(t *testing.T, ex spec.Example, input []byte) {
	t.Helper()
	if AcceptMode() {
		// Guard rail: surface the refusal so a regressed reader can't
		// silently relax an error expectation under -u.
		if err := RefuseAcceptForCase(ex); err != nil {
			t.Logf("%v", err)
		}
	}

	// Guard against a reader panic on hostile input — convert it into a
	// clean failure rather than crashing the test binary.
	var (
		parts   []*model.Part
		readErr error
	)
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				readErr = fmt.Errorf("reader panicked on invalid input: %v", rec)
			}
		}()
		parts, readErr = r.read(spec.Feature{}, ex, input)
	}()

	if readErr == nil {
		msg := fmt.Sprintf("class: invalid case %q: reader accepted malformed input (%d parts), expected a clean error", ex.CaseID(), len(parts))
		if ex.ExpectedFail != "" {
			t.Logf("expected_fail (%s): %s", ex.ExpectedFail, msg)
			return
		}
		t.Errorf("%s", msg)
		return
	}
	if strings.Contains(readErr.Error(), "panicked") {
		t.Errorf("%s", readErr)
		return
	}

	// Clean rejection. Optionally assert the message substring.
	if ex.Expected != nil && ex.Expected.Error != nil && ex.Expected.Error.MessageContains != "" {
		if !strings.Contains(readErr.Error(), ex.Expected.Error.MessageContains) {
			msg := fmt.Sprintf("class: invalid case %q: error %q does not contain %q", ex.CaseID(), readErr.Error(), ex.Expected.Error.MessageContains)
			if ex.ExpectedFail != "" {
				t.Logf("expected_fail (%s): %s", ex.ExpectedFail, msg)
				return
			}
			t.Errorf("%s", msg)
			return
		}
	}
	t.Logf("rejected cleanly: %v", readErr)
}

// checkBlocks compares the live block-event dump to the case's expected.blocks
// fixture (inline JSONL or a sibling file). In accept-mode it regenerates a
// file-backed fixture instead of comparing. Returns assertion-failure
// messages (empty on match); infrastructure problems call t.Fatalf.
func (r *NativeRunner) checkBlocks(t *testing.T, ex spec.Example, parts []*model.Part) []string {
	t.Helper()
	got, err := spec.DumpBlockEvents(parts)
	if err != nil {
		t.Fatalf("dump block events: %v", err)
	}

	if AcceptMode() {
		path, uerr := UpdateBlocksFixture(r.Spec, ex, parts)
		if uerr != nil {
			t.Logf("accept-mode: %v (leaving fixture as-is)", uerr)
		} else {
			t.Logf("accept-mode: wrote %s", path)
		}
		return nil
	}

	blocks := ex.Expected.Blocks
	var want string
	if isInlineBlocks(blocks) {
		want = blocks
	} else {
		path, perr := spec.ResolveFilePath(r.Spec, blocks)
		if perr != nil {
			t.Fatalf("resolve blocks fixture: %v", perr)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return []string{fmt.Sprintf("blocks fixture not found: %s (run with KAPI_SPEC_UPDATE=1 to generate)", path)}
		}
		want = string(data)
	}

	if diff := spec.FirstDiffLine(want, string(got)); diff != "" {
		return []string{"blocks view mismatch — " + diff}
	}
	return nil
}

// checkRoundtrip drives read→write and asserts the writer output per the
// case's roundtrip mode. Returns assertion-failure messages (empty on
// success); a missing NewWriter or a write error calls t.Fatalf.
func (r *NativeRunner) checkRoundtrip(t *testing.T, feat spec.Feature, ex spec.Example, input []byte, parts []*model.Part) []string {
	t.Helper()
	rt := ex.Expected.Roundtrip
	if r.NewWriter == nil {
		t.Fatalf("case %q declares expected.roundtrip but the runner has no NewWriter", ex.CaseID())
	}
	writer := r.NewWriter(ex.Variant)
	if writer == nil {
		t.Fatalf("NewWriter returned nil for variant %q", ex.Variant)
	}
	output, err := spec.WriteParts(writer, parts, input)
	if err != nil {
		t.Fatalf("roundtrip write: %v", err)
	}

	var failed []string
	for _, sub := range rt.OutputContains {
		if !strings.Contains(string(output), sub) {
			failed = append(failed, fmt.Sprintf("roundtrip output_contains: %q not found in output", sub))
		}
	}

	switch rt.Mode {
	case spec.RoundtripByteExact:
		if !bytesEqual(output, input) {
			failed = append(failed, "roundtrip byte_exact mismatch — "+spec.FirstDiffLine(string(input), string(output)))
		}
	case spec.RoundtripIdempotent:
		reparts, rerr := r.read(feat, ex, output)
		if rerr != nil {
			t.Fatalf("roundtrip idempotent re-read: %v", rerr)
		}
		writer2 := r.NewWriter(ex.Variant)
		output2, werr := spec.WriteParts(writer2, reparts, output)
		if werr != nil {
			t.Fatalf("roundtrip idempotent re-write: %v", werr)
		}
		if !bytesEqual(output, output2) {
			failed = append(failed, "roundtrip idempotent fixpoint not reached — "+spec.FirstDiffLine(string(output), string(output2)))
		}
	case spec.RoundtripNormalized:
		if AcceptMode() {
			path, uerr := UpdateRoundtripFixture(r.Spec, ex, output)
			if uerr != nil {
				t.Logf("accept-mode: %v (leaving fixture as-is)", uerr)
			} else {
				t.Logf("accept-mode: wrote %s", path)
			}
			return failed
		}
		path, perr := spec.ResolveFilePath(r.Spec, rt.OutputFile)
		if perr != nil {
			t.Fatalf("resolve roundtrip output_file: %v", perr)
		}
		data, derr := os.ReadFile(path)
		if derr != nil {
			return append(failed, fmt.Sprintf("roundtrip output_file not found: %s (run with KAPI_SPEC_UPDATE=1 to generate)", path))
		}
		if !bytesEqual(output, data) {
			failed = append(failed, "roundtrip normalized mismatch — "+spec.FirstDiffLine(string(data), string(output)))
		}
	}
	return failed
}

func bytesEqual(a, b []byte) bool {
	return string(a) == string(b)
}
