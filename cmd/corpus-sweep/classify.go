package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
)

// Classification is the corpus-sweep failure/success taxonomy
// (docs/internals/research/format-ops/followup-format-parser-security-ops.md §c,
// format-maturity.md §2.5). A file is classified into exactly one bucket.
//
// The worker (one subprocess per file) can only ever observe the four
// in-process outcomes — OK, OK_ROUNDTRIP, EXPECTED_REJECT, ROUNDTRIP_DRIFT —
// plus a recovered-panic CRASH. The driver assigns the three process-fatal
// outcomes CRASH (unrecovered panic / non-zero exit), HANG (wall-clock kill),
// and OOM (RSS-cap kill) from the subprocess result. The split is the Tika
// ForkParser doctrine: a permahang or evil-OOM cannot be caught in-process, so
// the orchestrator never parses a corpus file itself.
type Classification string

const (
	// OK — the reader parsed the input within limits (no round-trip claim;
	// e.g. a read-only format with no writer, or a writer that refused).
	OK Classification = "OK"
	// OKRoundTrip — read→write→read is stable (block-event dumps identical).
	OKRoundTrip Classification = "OK_ROUNDTRIP"
	// ExpectedReject — the reader returned a clean error on the channel
	// (graceful refusal), no panic.
	ExpectedReject Classification = "EXPECTED_REJECT"
	// RoundtripDrift — parsed, but the re-read of the written output differs
	// (or fails) — the faithfulness-instability class.
	RoundtripDrift Classification = "ROUNDTRIP_DRIFT"
	// Crash — a Go panic or unexpected non-zero exit.
	Crash Classification = "CRASH"
	// Hang — killed by the wall-clock timeout (permahang).
	Hang Classification = "HANG"
	// OOM — killed by the RSS cap (evil OOM).
	OOM Classification = "OOM"
)

// allClasses is the canonical ordering used to seed per-format count maps so
// every bucket is present (zero-valued) in the report and the ledger.
var allClasses = []Classification{OK, OKRoundTrip, ExpectedReject, RoundtripDrift, Crash, Hang, OOM}

// sweepSourceLocale / sweepTargetLocale are the fixed locales the sweep reads
// and writes under. They are mirrored on both the reader's RawDocument and the
// writer so a bilingual format's target variant survives read→write→read.
const (
	sweepSourceLocale = model.LocaleID("en")
	sweepTargetLocale = model.LocaleID("fr")
)

// classifyTimeout bounds each in-process read/write so a single pathological
// reader cannot stall the worker past the driver's wall-clock kill. It is a
// belt-and-suspenders bound under the driver's per-subprocess timeout.
const classifyTimeout = 60 * time.Second

// classify runs one file through read→write→read and returns its taxonomy
// bucket. It is the pure, in-process unit the worker invokes and the unit test
// exercises directly. Panics are recovered and reported as CRASH so a
// recoverable panic is attributed to the file rather than only surfacing as a
// non-zero subprocess exit; truly fatal failures (stack-overflow, OOM, hang)
// are left to the driver's subprocess monitoring.
//
// budget bounds the read and write byte streams with the shared core/safeio
// limits.
func classify(reg *registry.FormatRegistry, formatID string, data []byte, budget safeio.Budget) (cls Classification, detail string) {
	defer func() {
		if r := recover(); r != nil {
			cls = Crash
			detail = fmt.Sprintf("panic: %v", r)
		}
	}()

	reader, err := reg.NewReader(registry.FormatID(formatID))
	if err != nil {
		// An unknown format id is a harness/config error, not a corpus
		// property — surface it as CRASH so it is visible rather than
		// miscounted as a graceful reject.
		return Crash, fmt.Sprintf("new reader: %v", err)
	}
	parts1, err := readBounded(reader, data, budget)
	if err != nil {
		return ExpectedReject, err.Error()
	}
	dump1, err := spec.DumpBlockEvents(parts1)
	if err != nil {
		return ExpectedReject, fmt.Sprintf("dump1: %v", err)
	}

	writer, werr := reg.NewWriter(registry.FormatID(formatID))
	if werr != nil || writer == nil {
		// Read-only format (e.g. pdf): the read succeeded within limits.
		return OK, "read-only (no writer); read ok"
	}
	out, err := writeBounded(writer, parts1, data, budget)
	if err != nil {
		// The writer refused cleanly; the read still worked within limits.
		return OK, fmt.Sprintf("read ok; write returned error: %v", err)
	}

	reader2, err := reg.NewReader(registry.FormatID(formatID))
	if err != nil {
		return OK, fmt.Sprintf("read ok; re-read reader unavailable: %v", err)
	}
	parts2, err := readBounded(reader2, out, budget)
	if err != nil {
		// The written document is not re-readable → faithfulness drift.
		return RoundtripDrift, fmt.Sprintf("re-read failed: %v", err)
	}
	dump2, err := spec.DumpBlockEvents(parts2)
	if err != nil {
		return RoundtripDrift, fmt.Sprintf("dump2: %v", err)
	}
	if !bytes.Equal(dump1, dump2) {
		return RoundtripDrift, spec.FirstDiffLine(string(dump1), string(dump2))
	}
	return OKRoundTrip, ""
}

// Round-trip fidelity note: the harness uses the same writer configuration as
// the spec framework's roundtrip view (core/format/spec.WriteParts) —
// OriginalContent-if-supported plus the read locales — and deliberately does
// NOT wire the byte-exact SkeletonStore (core/format.SkeletonStoreEmitter /
// SkeletonStoreConsumer). Most writers re-serialize the Part stream
// faithfully; one (html) round-trips via OriginalContent. A format whose
// byte-exact round-trip depends on a shared reader→writer SkeletonStore (xml)
// is therefore reported as ROUNDTRIP_DRIFT through the generic pipeline. That
// is an honest, advisory observation — read→write→read via the standard Part
// stream is not idempotent for it — surfaced for adjudication, not auto-fixed:
// universally forcing the skeleton path degrades the round-trip of every
// other format. (Verified empirically: skeleton-wiring fixes xml but breaks
// json/po/properties/xliff/yaml/html.)

// readBounded drives reader Open→Read→Close over data, with the input stream
// wrapped by the byte budget. A reader error on the channel is returned (the
// caller classifies it EXPECTED_REJECT); a panic propagates to classify's
// recover.
func readBounded(reader format.DataFormatReader, data []byte, budget safeio.Budget) ([]*model.Part, error) {
	ctx, cancel := context.WithTimeout(context.Background(), classifyTimeout)
	defer cancel()
	doc := &model.RawDocument{
		SourceLocale: sweepSourceLocale,
		TargetLocale: sweepTargetLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(budget.Reader(bytes.NewReader(data))),
	}
	if err := reader.Open(ctx, doc); err != nil {
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
	return parts, nil
}

// writeBounded drives writer SetOutputWriter→Write→Close over parts and returns
// the produced bytes, with the output stream wrapped by the byte budget. When
// the writer needs the original skeleton (html, json, …) it is supplied.
func writeBounded(writer format.DataFormatWriter, parts []*model.Part, original []byte, budget safeio.Budget) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), classifyTimeout)
	defer cancel()
	// Mirror the read locales so the writer renders the same target the reader
	// produced; otherwise a bilingual writer (po, xliff, …) cannot find the
	// target variant and emits a spurious round-trip difference.
	writer.SetLocale(sweepTargetLocale)
	writer.SetEncoding("UTF-8")
	if sl, ok := writer.(format.SourceLocaleSetter); ok {
		sl.SetSourceLocale(sweepSourceLocale)
	}
	if oc, ok := writer.(format.OriginalContentSetter); ok && len(original) > 0 {
		oc.SetOriginalContent(original)
	}
	var buf bytes.Buffer
	if err := writer.SetOutputWriter(budget.Writer(&buf)); err != nil {
		return nil, fmt.Errorf("set output: %w", err)
	}
	ch := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	if err := writer.Write(ctx, ch); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}
	return buf.Bytes(), nil
}

// isSafetyFailure reports the hard, unambiguous safety failures: a panic /
// unexpected exit (CRASH), a permahang (HANG), or an evil-OOM. These break the
// run (non-zero exit) and are promoted to the fuzz seed corpus regardless of
// tier — even a committed exemplar that crashes is a real bug worth a
// regression seed.
func isSafetyFailure(cls string) bool {
	switch Classification(cls) {
	case Crash, Hang, OOM:
		return true
	}
	return false
}

// isRoundtripDrift reports the faithfulness signal. It is advisory: through the
// generic Part-stream pipeline some formats (xml and other skeleton-/
// structure-dependent writers) do not round-trip idempotently, so a drift on a
// committed Tier A exemplar is a known harness limitation rather than a fresh
// finding. Drift is therefore promoted only for Tier B wild files and never
// forces a non-zero exit; the ritual surfaces real drift regressions by diffing
// per-format counts against the previous sweep.
func isRoundtripDrift(cls string) bool {
	return Classification(cls) == RoundtripDrift
}
