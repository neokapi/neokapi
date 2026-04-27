package importer_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/importer"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
)

func TestImportDirect_Basic(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	// Seed one source block so ImportDirect has a block to land on.
	sess, _ := store.Begin(ctx)
	_ = sess.PutBlock("default", &blockstore.Block{
		ID:           "hello",
		Hash:         "hash-hello",
		Translatable: true,
		Source:       []klf.Run{{Text: &klf.TextRun{Text: "Hello"}}},
	})
	_ = sess.Commit()

	pairs := pairSeq(
		importer.ImportPair{BlockHash: "hash-hello", Locale: "fr", Text: "Bonjour"},
		importer.ImportPair{BlockHash: "hash-hello", Locale: "de", Text: "Hallo"},
	)
	report, err := importer.ImportDirect(ctx, store, pairs, importer.Options{
		Provider: "webhook:test",
	})
	if err != nil {
		t.Fatalf("ImportDirect: %v", err)
	}
	if report.TotalPairs != 2 || report.Written != 2 {
		t.Fatalf("report mismatch: %+v", report)
	}

	sess2, _ := store.Begin(ctx)
	defer sess2.Close()
	got, err := sess2.GetOverlay("targets/fr", "hash-hello")
	if err != nil {
		t.Fatalf("read fr overlay: %v", err)
	}
	if got.Kind != "targets/fr" || got.BlockHash != "hash-hello" {
		t.Fatalf("overlay key mismatch: %+v", got)
	}
	// Payload includes both text + provider.
	if !containsJSONField(got.Payload, `"text":"Bonjour"`) {
		t.Fatalf("expected text in payload, got %s", got.Payload)
	}
	if !containsJSONField(got.Payload, `"provider":"webhook:test"`) {
		t.Fatalf("expected provider in payload, got %s", got.Payload)
	}
}

func TestImportDirect_MissingHashReportsUnmatched(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	pairs := pairSeq(
		importer.ImportPair{Locale: "fr", Text: "Bonjour"}, // no BlockHash
	)
	r, err := importer.ImportDirect(ctx, store, pairs, importer.Options{})
	if err != nil {
		t.Fatalf("ImportDirect: %v", err)
	}
	if r.Unmatched != 1 || r.Written != 0 {
		t.Fatalf("expected 1 unmatched / 0 written, got %+v", r)
	}
}

func TestImportDirect_ConflictSkipExisting(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	sess, _ := store.Begin(ctx)
	_ = sess.PutBlock("default", &blockstore.Block{
		ID: "x", Hash: "h-x", Translatable: true,
		Source: []klf.Run{{Text: &klf.TextRun{Text: "x"}}},
	})
	// Pre-existing target.
	_ = sess.PutOverlay(blockstore.Overlay{
		Kind: "targets/fr", BlockHash: "h-x", Payload: []byte(`{"text":"existing"}`),
	})
	_ = sess.Commit()

	pairs := pairSeq(
		importer.ImportPair{BlockHash: "h-x", Locale: "fr", Text: "imported"},
	)
	r, err := importer.ImportDirect(ctx, store, pairs, importer.Options{
		OnConflict: importer.SkipExisting,
	})
	if err != nil {
		t.Fatalf("ImportDirect: %v", err)
	}
	if r.SkippedByPolicy != 1 || r.Written != 0 {
		t.Fatalf("expected 1 skip / 0 write, got %+v", r)
	}

	// Verify the pre-existing overlay wasn't overwritten.
	sess2, _ := store.Begin(ctx)
	defer sess2.Close()
	got, _ := sess2.GetOverlay("targets/fr", "h-x")
	if !containsJSONField(got.Payload, `"text":"existing"`) {
		t.Fatalf("skip-existing didn't preserve original: %s", got.Payload)
	}
}

func TestImportDirect_ConflictReplaceExisting(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	sess, _ := store.Begin(ctx)
	_ = sess.PutOverlay(blockstore.Overlay{
		Kind: "targets/fr", BlockHash: "h-x", Payload: []byte(`{"text":"old"}`),
	})
	_ = sess.Commit()

	pairs := pairSeq(
		importer.ImportPair{BlockHash: "h-x", Locale: "fr", Text: "new"},
	)
	r, _ := importer.ImportDirect(ctx, store, pairs, importer.Options{})
	if r.Written != 1 {
		t.Fatalf("expected 1 write, got %+v", r)
	}

	sess2, _ := store.Begin(ctx)
	defer sess2.Close()
	got, _ := sess2.GetOverlay("targets/fr", "h-x")
	if !containsJSONField(got.Payload, `"text":"new"`) {
		t.Fatalf("replace didn't stick: %s", got.Payload)
	}
}

func TestImportFromFormat_MatchesBySourceHash(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	// Seed three blocks. Two will match incoming sources.
	sess, _ := store.Begin(ctx)
	for _, b := range []*blockstore.Block{
		{ID: "a", Hash: "ha", Translatable: true, Source: []klf.Run{{Text: &klf.TextRun{Text: "Hello"}}}},
		{ID: "b", Hash: "hb", Translatable: true, Source: []klf.Run{{Text: &klf.TextRun{Text: "Save"}}}},
		{ID: "c", Hash: "hc", Translatable: true, Source: []klf.Run{{Text: &klf.TextRun{Text: "Unmatched"}}}},
	} {
		_ = sess.PutBlock("", b)
	}
	_ = sess.Commit()

	// Fake reader emits two model.Blocks whose source matches "Hello"
	// and "Save", each with a French target. The third source is
	// missing from the incoming — the existing block "Unmatched"
	// doesn't get an overlay.
	reader := &fakeReader{blocks: []*model.Block{
		srcTargetBlock("Hello", "fr", "Bonjour"),
		srcTargetBlock("Save", "fr", "Enregistrer"),
		srcTargetBlock("NotInStore", "fr", "should be Unmatched"),
	}}

	report, err := importer.ImportFromFormat(ctx, store, reader, &model.RawDocument{},
		importer.Options{Provider: "tmx:legacy"})
	if err != nil {
		t.Fatalf("ImportFromFormat: %v", err)
	}
	if report.TotalPairs != 3 {
		t.Fatalf("want 3 total pairs, got %d: %+v", report.TotalPairs, report)
	}
	if report.Matched != 2 || report.Unmatched != 1 {
		t.Fatalf("want 2 matched / 1 unmatched, got %+v", report)
	}
	if report.Written != 2 {
		t.Fatalf("want 2 written, got %+v", report)
	}

	// Verify the overlays landed on the matching block hashes.
	sess2, _ := store.Begin(ctx)
	defer sess2.Close()

	if got, err := sess2.GetOverlay("targets/fr", "ha"); err != nil {
		t.Fatalf("expected overlay on ha: %v", err)
	} else if !containsJSONField(got.Payload, `"text":"Bonjour"`) {
		t.Fatalf("ha overlay payload wrong: %s", got.Payload)
	}
	if got, err := sess2.GetOverlay("targets/fr", "hb"); err != nil {
		t.Fatalf("expected overlay on hb: %v", err)
	} else if !containsJSONField(got.Payload, `"text":"Enregistrer"`) {
		t.Fatalf("hb overlay payload wrong: %s", got.Payload)
	}
	if _, err := sess2.GetOverlay("targets/fr", "hc"); !errors.Is(err, blockstore.ErrNotFound) {
		t.Fatalf("expected no overlay on hc, got %v", err)
	}
}

// ─── helpers ────────────────────────────────────────────────────

func pairSeq(pairs ...importer.ImportPair) iter.Seq2[importer.ImportPair, error] {
	return func(yield func(importer.ImportPair, error) bool) {
		for _, p := range pairs {
			if !yield(p, nil) {
				return
			}
		}
	}
}

func containsJSONField(payload []byte, needle string) bool {
	s := string(payload)
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// srcTargetBlock builds a model.Block with one source segment and one
// target segment in the given locale.
func srcTargetBlock(src, locale, tgt string) *model.Block {
	b := model.NewRunsBlock("", []model.Run{{Text: &model.TextRun{Text: src}}})
	b.SetTargetRuns(model.LocaleID(locale), []model.Run{{Text: &model.TextRun{Text: tgt}}})
	return b
}

// fakeReader implements format.DataFormatReader by emitting a fixed
// slice of model.Blocks as PartResult channel messages. Enough of the
// interface to drive ImportFromFormat end-to-end in a unit test.
type fakeReader struct {
	blocks []*model.Block
}

func (r *fakeReader) Name() string                                       { return "fake" }
func (r *fakeReader) DisplayName() string                                { return "Fake" }
func (r *fakeReader) Signature() format.FormatSignature                  { return format.FormatSignature{} }
func (r *fakeReader) Open(_ context.Context, _ *model.RawDocument) error { return nil }
func (r *fakeReader) Close() error                                       { return nil }
func (r *fakeReader) Config() format.DataFormatConfig                    { return nil }
func (r *fakeReader) SetConfig(_ format.DataFormatConfig) error          { return nil }

func (r *fakeReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)
		for _, b := range r.blocks {
			select {
			case ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: b}}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
