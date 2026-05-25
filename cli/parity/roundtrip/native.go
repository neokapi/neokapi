//go:build parity

package roundtrip

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// sameLanguageAs mirrors okapi's LocaleId.sameLanguageAs: two locales
// match when their primary language subtag is identical, regardless of
// region/script (e.g. "fr" sameLanguageAs "fr-FR"). The pseudo gate
// uses this rather than strict equality because okapi's XLIFFFilter
// keeps trgLang at the request locale ("fr") whenever it's
// sameLanguageAs the file's target-language ("fr-FR"); the existing
// target then participates in TextModificationStep instead of being
// preserved verbatim. Without this, native skips pseudo on bilingual
// fixtures whose <file target-language="fr-FR"> mismatches our test
// target "fr" — see ImplementationPlan.docx.xlf and RB-12-Test02.xlf.
func sameLanguageAs(a, b model.LocaleID) bool {
	return primaryLang(a) == primaryLang(b)
}

// primaryLang returns the primary language subtag, lowercased — the
// part before the first "-" or "_". Empty input returns empty.
func primaryLang(l model.LocaleID) string {
	s := strings.ToLower(string(l))
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		s = s[:i]
	}
	return s
}

// NativeEngine drives a neokapi format reader → PseudoTranslate tool
// → writer pipeline in-process. The format ID is the registered
// neokapi name (e.g. "plaintext", "html", "po") rather than the
// upstream Okapi class name.
type NativeEngine struct {
	// FormatID is the registry key for the reader/writer pair.
	FormatID registry.FormatID

	// ReaderConfig is applied via reader.Config().ApplyMap when
	// non-nil. Pass the same semantic config the spec.yaml example
	// uses; defaults are taken otherwise.
	ReaderConfig map[string]any

	// WriterOverlay is a curated map applied to the writer's
	// WriterConfig().ApplyMap to align the native writer's output with
	// upstream Okapi's defaults for parity comparison. These are NOT
	// format defaults — they exist solely so the parity test can verify
	// "given the same semantic config, native produces the same bytes
	// as okapi". Document the intent inline at the call site so the
	// "why" survives.
	WriterOverlay map[string]any
}

// Name returns "native".
func (e *NativeEngine) Name() string { return "native" }

// Available always succeeds — the native pipeline runs in-process
// with no external dependencies.
func (e *NativeEngine) Available() error { return nil }

// RoundTrip extracts via the registered reader, applies the
// PseudoTranslate tool to every Block part, and writes through the
// registered writer. Returns the merged output bytes.
func (e *NativeEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	if e.FormatID == "" {
		t.Fatal("NativeEngine.RoundTrip: FormatID is required")
	}
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	reader, err := reg.NewReader(e.FormatID)
	if err != nil {
		t.Fatalf("NativeEngine: reader for %q: %v", e.FormatID, err)
	}
	if len(e.ReaderConfig) > 0 {
		cfg := reader.Config()
		if cfg == nil {
			t.Fatalf("NativeEngine: ReaderConfig set but reader %q has no Config()", e.FormatID)
		}
		if err := cfg.ApplyMap(e.ReaderConfig); err != nil {
			t.Fatalf("NativeEngine: ApplyMap: %v", err)
		}
	}
	writer, err := reg.NewWriter(e.FormatID)
	if err != nil {
		t.Fatalf("NativeEngine: writer for %q: %v", e.FormatID, err)
	}
	if len(e.WriterOverlay) > 0 {
		cfgable, ok := writer.(format.WriterConfigurable)
		if !ok {
			t.Fatalf("NativeEngine: WriterOverlay set but writer %q does not implement WriterConfigurable", e.FormatID)
		}
		cfg := cfgable.WriterConfig()
		if cfg == nil {
			t.Fatalf("NativeEngine: WriterOverlay set but writer %q returned nil WriterConfig", e.FormatID)
		}
		if err := cfg.ApplyMap(e.WriterOverlay); err != nil {
			t.Fatalf("NativeEngine: WriterConfig.ApplyMap: %v", err)
		}
	}

	tgt := model.LocaleID(spec.TgtLocale())

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, in.Filename)
	if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
		t.Fatalf("NativeEngine: write input: %v", err)
	}
	for name, data := range in.Companions {
		if err := os.WriteFile(filepath.Join(tmpDir, name), data, 0o644); err != nil {
			t.Fatalf("NativeEngine: write companion %q: %v", name, err)
		}
	}

	doc := &model.RawDocument{
		URI:          inputPath,
		SourceLocale: model.LocaleID(spec.SrcLocale()),
		TargetLocale: tgt,
		Reader:       io.NopCloser(bytes.NewReader(in.Bytes)),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := reader.Open(ctx, doc); err != nil {
		t.Fatalf("NativeEngine: reader.Open: %v", err)
	}
	defer reader.Close()

	// Wire the skeleton store before reader.Open so the reader can
	// stream entries while it parses. Writers that consume it produce
	// byte-stable output by replaying skeleton text + filling block
	// refs with translated content, instead of falling back to a
	// best-effort no-skeleton path.
	store, err := format.NewSkeletonStore()
	if err != nil {
		t.Fatalf("NativeEngine: skeleton store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// xliff2 has both a legacy SkeletonStore round-trip path and a newer
	// DOM-patching round-trip path. The DOM path is strictly better
	// (handles trgLang overrides, segment-id rewrites, etc.) but the
	// reader still routes through the streaming/skeleton implementation
	// when a SkeletonStore is wired. Skip wiring the store for xliff2 so
	// the parity harness gets the DOM path; other formats keep their
	// existing wiring.
	if e.FormatID != "xliff2" {
		if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
			emitter.SetSkeletonStore(store)
		}
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			consumer.SetSkeletonStore(store)
		}
	}

	// Set skeleton from input bytes for writers that reuse it.
	if setter, ok := writer.(format.OriginalContentSetter); ok {
		setter.SetOriginalContent(in.Bytes)
	}
	if setter, ok := writer.(format.SourcePathSetter); ok {
		setter.SetSourcePath(inputPath)
	}
	if setter, ok := writer.(format.SourceLocaleSetter); ok {
		setter.SetSourceLocale(model.LocaleID(spec.SrcLocale()))
	}
	writer.SetLocale(tgt)

	var outBuf bytes.Buffer
	if err := writer.SetOutputWriter(&outBuf); err != nil {
		t.Fatalf("NativeEngine: SetOutputWriter: %v", err)
	}

	// Drain the reader fully into a part slice (applying the inline
	// pseudo transform on Block parts) before touching the writer.
	// Sequencing read-then-write lets us detect a stub
	// SkeletonStoreEmitter (one that registers but emits zero entries)
	// and unwire the writer's skeleton consumer before it runs — without
	// the unwiring, a writer that branches on skeletonStore != nil would
	// silently produce empty output. Streaming concurrency isn't worth
	// preserving for parity fixtures (small, in-process).
	//
	// The pseudo transform is inline instead of the registered
	// PseudoTranslate tool — that tool always applies an accent map,
	// which would diverge from bridge/tikal outputs. We want a
	// deterministic wrap that all three engines produce identically.
	var parts []*model.Part
	// fileTargetLang tracks the source's declared target-language (e.g.
	// xliff <file target-language="es">, ts <TS language="af">). When
	// it's set and differs from the test target, okapi's pipeline
	// preserves the existing target instead of pseudo-translating it
	// (TextModificationStep gates on the file's target-language). Mirror
	// that here so bilingual fixtures with non-matching targets stay
	// byte-equal with the okapi reference.
	var fileTargetLang model.LocaleID
	// XLIFF 2 is documented-lossy on round-trip: okapi unconditionally
	// overwrites the file's `trgLang` with the requested target locale and
	// (re-)pseudo-translates every translatable unit, ignoring whatever
	// target was authored on disk. Mirror that here so native + okapi
	// converge — for xliff2 we always apply pseudo and we rewrite the
	// layer's target-language property so the writer emits the requested
	// trgLang on the root <xliff> element.
	forcePseudoIgnoreFileTarget := e.FormatID == "xliff2"
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			if !errors.Is(res.Error, io.EOF) {
				t.Fatalf("NativeEngine: reader stream: %v", res.Error)
			}
			continue
		}
		if res.Part == nil {
			continue
		}
		if res.Part.Type == model.PartLayerStart {
			if layer, ok := res.Part.Resource.(*model.Layer); ok {
				if tl, ok := layer.Properties["target-language"]; ok {
					fileTargetLang = model.LocaleID(tl)
				}
				if forcePseudoIgnoreFileTarget {
					if layer.Properties == nil {
						layer.Properties = map[string]string{}
					}
					layer.Properties["target-language"] = string(tgt)
					// Okapi's xliff2 pipeline collapses the file's `srcLang`
					// to the requested --src-lang only when the requested
					// value is the primary subtag of the file's value (e.g.
					// requested "en" + file "en-US" → emits "en"). When the
					// primaries differ (file "es-ES", requested "en"), okapi
					// preserves the file's srcLang verbatim. Mirror that
					// narrow rule here so fixtures like Project Playground,
					// code_id_mismatch, and test02 close while notes.xlf
					// (srcLang="es-ES") stays canon-equal.
					existing := string(layer.Locale)
					requested := spec.SrcLocale()
					if requested != "" && existing != requested && strings.HasPrefix(existing, requested+"-") {
						layer.Locale = model.LocaleID(requested)
					}
				}
			}
		}
		if res.Part.Type == model.PartBlock {
			if b, ok := res.Part.Resource.(*model.Block); ok {
				// okapi's TextModificationStep applies whenever the
				// request locale is sameLanguageAs the file's
				// target-language: XLIFFFilter keeps trgLang at the
				// request locale in that case, so the existing target
				// participates and gets pseudo-translated. Only when
				// the languages truly differ (e.g. file=es, request=fr)
				// does okapi preserve the existing target verbatim and
				// skip transformation. xliff2 is the exception — okapi
				// pseudo-translates unconditionally there, see the
				// comment above.
				//
				// For xliff2, the source is the pseudo base only when
				// the file's existing trgLang differs from the
				// requested target (the existing target is in the
				// "wrong" language and gets discarded). When trgLang
				// matches, the existing target is the right base.
				if forcePseudoIgnoreFileTarget || fileTargetLang.IsEmpty() || sameLanguageAs(fileTargetLang, tgt) {
					forceSrc := forcePseudoIgnoreFileTarget && !fileTargetLang.IsEmpty() && !sameLanguageAs(fileTargetLang, tgt)
					// xliff2: okapi's X2ToOkpConverter copies source
					// to target for any ignorable lacking one, when
					// at least one sibling segment has a target (line
					// 200: "apply the source ignorable content to
					// target unless there exists target ignorable
					// content, but only if we had a target segment").
					// Without this pre-pseudo seeding, pickPseudoBase
					// returns just the existing-target subset and our
					// ignorables come out with empty targets while
					// okapi's are pseudo-translated. Do this BEFORE
					// pseudo so the new targets get translated too.
					if forcePseudoIgnoreFileTarget {
						seedIgnorableTargetsFromSource(b, tgt)
					}
					applyPseudoToBlockOpts(b, spec, forceSrc)
				}
			}
		}
		parts = append(parts, res.Part)
	}

	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		if store.EntriesWritten() == 0 {
			// Reader registered as a SkeletonStoreEmitter but never
			// actually emitted (stubbed). Unwire the writer so it
			// takes its no-skeleton path.
			consumer.SetSkeletonStore(nil)
		}
	}

	writerIn := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		writerIn <- p
	}
	close(writerIn)

	var writeErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeErr = writer.Write(ctx, writerIn)
	}()
	wg.Wait()
	if err := writer.Close(); err != nil && writeErr == nil {
		writeErr = err
	}
	if writeErr != nil {
		t.Fatalf("NativeEngine: writer: %v", writeErr)
	}

	out := outBuf.Bytes()
	if len(out) == 0 {
		t.Fatalf("NativeEngine: writer produced empty output")
	}
	return out
}

// seedIgnorableTargetsFromSource ensures every source segment in b has a
// matching target segment for tgt, by cloning the source segment's runs
// into the target run sequence (and adding an id-aligned target
// segmentation span) when no target segment exists for that id. Used by
// the xliff2 native engine to mirror okapi's X2ToOkpConverter line 200
// source-to-target copy for ignorables that lack a target — the reader
// doesn't synthesize these (a faithful parse keeps the source
// asymmetry), but pseudo only operates on existing targets, so without
// the seed our ignorables end up with empty targets while okapi's are
// pseudo-translated.
//
// Operates over the stand-off segmentation overlay (AD-002): the source
// segments come from the source overlay (nil → a single anonymous
// segment), the existing target segments from the target overlay. For
// each ignorable source segment missing from the target we append its
// cloned runs to the target run slice and a matching span (same id +
// props) to the target overlay, keeping the target index/id-aligned with
// the source.
//
// Only seeds when at least one target segment ALREADY exists — for
// unit-only-source blocks (translate="no"-style flows) we leave the
// model untouched.
func seedIgnorableTargetsFromSource(b *model.Block, tgt model.LocaleID) {
	if b == nil || len(b.Source) == 0 {
		return
	}
	target := b.Target(tgt)
	if target == nil || len(target.Runs) == 0 {
		return
	}

	srcSeg := b.SourceSegmentation()
	srcCount := b.SourceSegmentCount()

	key := model.Variant(tgt)
	tgtSeg := b.SegmentationFor(&key)

	// Existing target span ids and the target run slice. With no target
	// overlay the whole target is one anonymous segment; treat its id as
	// the lone source segment's id so we don't double-seed it.
	have := make(map[string]bool)
	var tgtSpans []model.Span
	if tgtSeg != nil {
		tgtSpans = make([]model.Span, len(tgtSeg.Spans))
		copy(tgtSpans, tgtSeg.Spans)
		for _, sp := range tgtSpans {
			have[sp.ID] = true
		}
	} else if srcSeg != nil && len(srcSeg.Spans) > 0 {
		// Single flat target, source is segmented: the existing target
		// runs map to the first source segment id (the reader emits a
		// target only when it has non-empty inline content). Seed a span
		// covering the existing runs so the rebuilt target overlay keeps
		// describing that pre-existing segment.
		firstID := srcSeg.Spans[0].ID
		have[firstID] = true
		tgtSpans = append(tgtSpans, model.Span{
			ID:    firstID,
			Range: model.RunRange{StartRun: 0, EndRun: len(target.Runs)},
		})
	}

	tgtRuns := target.Runs
	seeded := false
	for i := 0; i < srcCount; i++ {
		var (
			id    string
			props map[string]string
		)
		if srcSeg != nil && i < len(srcSeg.Spans) {
			id = srcSeg.Spans[i].ID
			props = srcSeg.Spans[i].Props
		}
		if have[id] {
			continue
		}
		// Only seed for ignorables (mirrors X2ToOkpConverter's check
		// `!part.isSegment()` at line 200). Plain <segment> elements
		// without a target stay target-less in okapi's output too.
		if props["xliff2:kind"] != "ignorable" {
			continue
		}
		segRuns := cloneRuns(b.SourceSegmentRuns(i))
		start := len(tgtRuns)
		tgtRuns = append(tgtRuns, segRuns...)
		end := len(tgtRuns)
		sp := model.Span{ID: id, Range: model.RunRange{StartRun: start, EndRun: end}}
		if len(props) > 0 {
			clonedProps := make(map[string]string, len(props))
			for k, v := range props {
				clonedProps[k] = v
			}
			sp.Props = clonedProps
		}
		tgtSpans = append(tgtSpans, sp)
		have[id] = true
		seeded = true
	}
	if !seeded {
		return
	}
	b.SetTargetRuns(tgt, tgtRuns)
	b.SetSegmentation(&key, tgtSpans)
}

// cloneRuns returns a deep copy of the given run slice.
func cloneRuns(runs []model.Run) []model.Run {
	out := make([]model.Run, 0, len(runs))
	for _, r := range runs {
		nr := r
		if r.Text != nil {
			t := *r.Text
			nr.Text = &t
		}
		out = append(out, nr)
	}
	return out
}

// Compile-time interface check.
var _ Engine = (*NativeEngine)(nil)
