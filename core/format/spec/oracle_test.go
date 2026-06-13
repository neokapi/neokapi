package spec

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// fakeReader is a minimal DataFormatReader that replays a fixed part stream
// (or surfaces a fixed error), used to exercise RunNativeCase without pulling
// a concrete format package into the spec package's test build.
type fakeReader struct {
	format.BaseFormatReader
	parts []*model.Part
	err   error
}

func (f *fakeReader) Signature() format.FormatSignature                      { return format.FormatSignature{} }
func (f *fakeReader) Open(ctx context.Context, doc *model.RawDocument) error { return nil }
func (f *fakeReader) Close() error                                           { return nil }

func (f *fakeReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, len(f.parts)+1)
	go func() {
		defer close(ch)
		if f.err != nil {
			ch <- model.PartResult{Error: f.err}
			return
		}
		for _, p := range f.parts {
			ch <- model.PartResult{Part: p}
		}
	}()
	return ch
}

func oracleParts() []*model.Part {
	return []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc", Format: "plaintext"}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID: "b1", Translatable: true,
			Source: []model.Run{{Text: &model.TextRun{Text: "hello"}}},
		}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc"}},
	}
}

// TestRunNativeCase_Valid proves the differential-oracle hook returns the
// canonical dump + extracted view for a parsing input.
func TestRunNativeCase_Valid(t *testing.T) {
	parts := oracleParts()
	s := &Spec{Format: "okf_x", Features: []Feature{{ID: "f", Examples: []Example{{Name: "a", InputXML: "x"}}}}}
	ex := s.Features[0].Examples[0]

	res := RunNativeCase(s, ex, nil, func(string) format.DataFormatReader {
		return &fakeReader{parts: parts}
	})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	want, _ := DumpBlockEvents(parts)
	if string(res.BlockEvents) != string(want) {
		t.Errorf("BlockEvents mismatch\n got: %s\nwant: %s", res.BlockEvents, want)
	}
	if len(res.Extracted) != 1 || res.Extracted[0] != "hello" {
		t.Errorf("Extracted: got %v, want [hello]", res.Extracted)
	}
	if len(res.Parts) != 3 {
		t.Errorf("Parts: got %d, want 3", len(res.Parts))
	}
}

// TestRunNativeCase_Rejected proves a rejecting reader surfaces the §3
// invalid-class signal through CaseResult.Err with no dump.
func TestRunNativeCase_Rejected(t *testing.T) {
	s := &Spec{Format: "okf_x", Features: []Feature{{ID: "f", Examples: []Example{{Name: "a", InputXML: "x"}}}}}
	ex := s.Features[0].Examples[0]

	res := RunNativeCase(s, ex, nil, func(string) format.DataFormatReader {
		return &fakeReader{err: errors.New("syntax: boom")}
	})
	if res.Err == nil {
		t.Fatal("expected an error from a rejecting reader")
	}
	if res.BlockEvents != nil {
		t.Errorf("BlockEvents should be empty on rejection, got %s", res.BlockEvents)
	}
}
