//go:build parity

package parity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
)

// BridgeRequest configures one Process RPC against the okapi-bridge
// daemon. Either InputBytes or InputPath must be set.
type BridgeRequest struct {
	// FilterClass is the Java filter class name (e.g. okf_html, okf_json).
	FilterClass string

	// InputBytes is the inline document content. Mutually exclusive with
	// InputPath.
	InputBytes []byte

	// InputPath is an absolute path the daemon can read directly. Mutually
	// exclusive with InputBytes.
	InputPath string

	// SourceLocale and TargetLocale default to "en" / "fr" when empty.
	SourceLocale string
	TargetLocale string

	// Encoding defaults to "UTF-8" when empty.
	Encoding string

	// MimeType is optional.
	MimeType string

	// FilterParams maps the filter's flat parameters (Okapi convention)
	// to string values; the bridge applies suffixes (.b, .i) based on
	// schema metadata.
	FilterParams map[string]string

	// SubscribeParts narrows which PartType values flow back over the
	// RPC. Empty (the default) streams every event.
	SubscribeParts []int32
}

// RunBridge drives a read-only Process RPC against the okapi-bridge
// daemon and returns the streamed parts.
func RunBridge(t *testing.T, req BridgeRequest) []*model.Part {
	t.Helper()
	if req.FilterClass == "" {
		t.Fatal("RunBridge: FilterClass is required")
	}
	if len(req.InputBytes) == 0 && req.InputPath == "" {
		t.Fatal("RunBridge: InputBytes or InputPath must be set")
	}
	client := AcquireBridgeDaemon(t)
	bridgeClient := pb.NewBridgeServiceClient(client.Conn)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := bridgeClient.Process(ctx)
	if err != nil {
		t.Fatalf("RunBridge: open Process stream: %v", err)
	}

	header := &pb.ProcessHeader{
		FilterClass:    req.FilterClass,
		SourceLocale:   defaultStr(req.SourceLocale, "en"),
		TargetLocale:   defaultStr(req.TargetLocale, "fr"),
		Encoding:       defaultStr(req.Encoding, "UTF-8"),
		MimeType:       req.MimeType,
		FilterParams:   req.FilterParams,
		SubscribeParts: req.SubscribeParts,
	}
	if req.InputPath != "" {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Path{Path: req.InputPath}}
	} else {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: req.InputBytes}}
	}
	if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
		t.Fatalf("RunBridge: send header: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("RunBridge: CloseSend: %v", err)
	}

	parts, err := drainProcessResponses(stream)
	if err != nil {
		t.Fatalf("RunBridge: drain stream: %v", err)
	}
	return parts
}

// BridgeRoundTripResult holds both the events streamed during the
// round-trip and the final output bytes the daemon wrote.
type BridgeRoundTripResult struct {
	Parts  []*model.Part
	Output []byte
}

// RunBridgeRoundTrip drives a read+write Process RPC: feeds the source
// into the daemon, echoes every part back unchanged so the writer
// applies them, and returns both the part stream and the rewritten
// document bytes.
func RunBridgeRoundTrip(t *testing.T, req BridgeRequest) BridgeRoundTripResult {
	t.Helper()
	if req.FilterClass == "" {
		t.Fatal("RunBridgeRoundTrip: FilterClass is required")
	}
	if len(req.InputBytes) == 0 && req.InputPath == "" {
		t.Fatal("RunBridgeRoundTrip: InputBytes or InputPath must be set")
	}
	client := AcquireBridgeDaemon(t)
	bridgeClient := pb.NewBridgeServiceClient(client.Conn)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	stream, err := bridgeClient.Process(ctx)
	if err != nil {
		t.Fatalf("RunBridgeRoundTrip: open Process stream: %v", err)
	}
	target := defaultStr(req.TargetLocale, "fr")
	header := &pb.ProcessHeader{
		FilterClass:    req.FilterClass,
		SourceLocale:   defaultStr(req.SourceLocale, "en"),
		TargetLocale:   target,
		OutputLocale:   target,
		Encoding:       defaultStr(req.Encoding, "UTF-8"),
		MimeType:       req.MimeType,
		FilterParams:   req.FilterParams,
		SubscribeParts: req.SubscribeParts,
		// Empty path triggers inline-bytes return in ProcessComplete.
		Output: &pb.OutputRef{Destination: &pb.OutputRef_Path{Path: ""}},
	}
	if req.InputPath != "" {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Path{Path: req.InputPath}}
	} else {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: req.InputBytes}}
	}
	if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
		t.Fatalf("RunBridgeRoundTrip: send header: %v", err)
	}

	// Echo every read part back so the daemon's writer thread has
	// something to apply. We capture each read part for caller
	// inspection.
	var parts []*model.Part
	var output []byte
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("RunBridgeRoundTrip: recv: %v", err)
		}
		switch m := resp.Response.(type) {
		case *pb.ProcessResponse_Part:
			parts = append(parts, protoconvert.ProtoToPart(m.Part))
			if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Part{Part: m.Part}}); err != nil {
				t.Fatalf("RunBridgeRoundTrip: echo part: %v", err)
			}
		case *pb.ProcessResponse_PartBatch:
			for _, p := range m.PartBatch.Parts {
				parts = append(parts, protoconvert.ProtoToPart(p))
				if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Part{Part: p}}); err != nil {
					t.Fatalf("RunBridgeRoundTrip: echo batched part: %v", err)
				}
			}
		case *pb.ProcessResponse_ContentBatch:
			for _, cb := range m.ContentBatch.Blocks {
				parts = append(parts, protoconvert.ContentBlockToPart(cb))
				if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_ContentBlock{ContentBlock: cb}}); err != nil {
					t.Fatalf("RunBridgeRoundTrip: echo content block: %v", err)
				}
			}
		case *pb.ProcessResponse_ReadDone:
			if err := stream.CloseSend(); err != nil {
				t.Fatalf("RunBridgeRoundTrip: CloseSend after ReadDone: %v", err)
			}
		case *pb.ProcessResponse_Complete:
			if m.Complete.Error != "" {
				t.Fatalf("RunBridgeRoundTrip: daemon error: %s", m.Complete.Error)
			}
			output = m.Complete.Output
		}
	}
	return BridgeRoundTripResult{Parts: parts, Output: output}
}

// StepRequest configures one ProcessStep RPC.
type StepRequest struct {
	StepClass    string
	StepParams   map[string]string
	ParamTypes   map[string]string
	SourceLocale string
	TargetLocale string
	// Parts is the input stream to feed into the step. The harness
	// sends them after the StepHeader.
	Parts []*model.Part
}

// RunBridgeStep drives a ProcessStep RPC, feeding `Parts` into the
// daemon's pipeline step and returning whatever the step emits.
func RunBridgeStep(t *testing.T, req StepRequest) []*model.Part {
	t.Helper()
	if req.StepClass == "" {
		t.Fatal("RunBridgeStep: StepClass is required")
	}
	client := AcquireBridgeDaemon(t)
	bridgeClient := pb.NewBridgeServiceClient(client.Conn)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := bridgeClient.ProcessStep(ctx)
	if err != nil {
		t.Fatalf("RunBridgeStep: open ProcessStep stream: %v", err)
	}

	header := &pb.StepHeader{
		StepClass:    req.StepClass,
		StepParams:   req.StepParams,
		SourceLocale: defaultStr(req.SourceLocale, "en"),
		TargetLocale: defaultStr(req.TargetLocale, "fr"),
		ParamTypes:   req.ParamTypes,
	}
	if err := stream.Send(&pb.StepRequest{Request: &pb.StepRequest_Header{Header: header}}); err != nil {
		t.Fatalf("RunBridgeStep: send header: %v", err)
	}

	// Fan parts in concurrently so the daemon can pipeline.
	sendErr := make(chan error, 1)
	go func() {
		defer func() { _ = stream.CloseSend() }()
		for _, p := range req.Parts {
			if err := stream.Send(&pb.StepRequest{Request: &pb.StepRequest_Part{Part: protoconvert.PartToProto(p)}}); err != nil {
				sendErr <- fmt.Errorf("send part: %w", err)
				return
			}
		}
		sendErr <- nil
	}()

	var out []*model.Part
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("RunBridgeStep: recv: %v", err)
		}
		switch m := resp.Response.(type) {
		case *pb.StepResponse_Part:
			out = append(out, protoconvert.ProtoToPart(m.Part))
		case *pb.StepResponse_Complete:
			// Continue draining until the stream ends.
			_ = m
		}
	}
	if err := <-sendErr; err != nil {
		t.Fatalf("RunBridgeStep: %v", err)
	}
	return out
}

// drainProcessResponses pulls every part off a Process stream until the
// daemon emits ProcessComplete or the stream ends.
func drainProcessResponses(stream pb.BridgeService_ProcessClient) ([]*model.Part, error) {
	var out []*model.Part
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		switch m := resp.Response.(type) {
		case *pb.ProcessResponse_Part:
			out = append(out, protoconvert.ProtoToPart(m.Part))
		case *pb.ProcessResponse_PartBatch:
			for _, p := range m.PartBatch.Parts {
				out = append(out, protoconvert.ProtoToPart(p))
			}
		case *pb.ProcessResponse_ContentBatch:
			for _, cb := range m.ContentBatch.Blocks {
				out = append(out, protoconvert.ContentBlockToPart(cb))
			}
		case *pb.ProcessResponse_ReadDone:
			// Read-only mode closes after the next ProcessComplete.
		case *pb.ProcessResponse_Complete:
			if m.Complete.Error != "" {
				return out, fmt.Errorf("daemon: %s", m.Complete.Error)
			}
			return out, nil
		}
	}
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
