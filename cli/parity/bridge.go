//go:build parity

package parity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	// ConfigId names a built-in Okapi filter configuration to apply before
	// opening (e.g. "okf_xmlstream-dita"). The bridge loads that config's
	// parameters; FilterParams override on top. Empty = filter defaults.
	ConfigId string

	// SubscribeParts narrows which PartType values flow back over the
	// RPC. Empty (the default) streams every event.
	SubscribeParts []int32

	// Transform is an optional hook that mutates each Block before
	// it's echoed back to the daemon during a round-trip. Only used
	// by RunBridgeRoundTrip; ignored by RunBridge (read-only). When
	// nil the round-trip echoes parts unchanged. Non-Block parts
	// (Layer, Data, etc.) always echo through untouched.
	Transform func(b *model.Block)
}

// RunBridge drives a read-only Process RPC against the okapi-bridge
// daemon and returns the streamed parts. Failures are fatal — the
// caller is asserting parity. Use TryRunBridge when failures should
// surface as errors instead (Informational fixtures).
func RunBridge(t *testing.T, req BridgeRequest) []*model.Part {
	t.Helper()
	parts, err := TryRunBridge(t, req)
	if err != nil {
		t.Fatalf("RunBridge: %v", err)
	}
	return parts
}

// TryRunBridge is the non-fatal variant of RunBridge. The daemon must
// still be acquireable (set-up failures stay fatal — every parity
// test needs the daemon) but per-call gRPC errors come back as
// `error` so the caller can record an outcome instead of failing the
// test. Used by Informational fixtures so a daemon-side error on one
// fixture doesn't break the whole CI run.
func TryRunBridge(t *testing.T, req BridgeRequest) ([]*model.Part, error) {
	t.Helper()
	if req.FilterClass == "" {
		return nil, fmt.Errorf("FilterClass is required")
	}
	if len(req.InputBytes) == 0 && req.InputPath == "" {
		return nil, fmt.Errorf("InputBytes or InputPath must be set")
	}
	client := AcquireBridgeDaemon(t)
	bridgeClient := pb.NewBridgeServiceClient(client.Conn)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := bridgeClient.Process(ctx)
	if err != nil {
		return nil, fmt.Errorf("open Process stream: %w", err)
	}

	header := &pb.ProcessHeader{
		FilterClass:    req.FilterClass,
		SourceLocale:   defaultStr(req.SourceLocale, "en"),
		TargetLocale:   defaultStr(req.TargetLocale, "fr"),
		Encoding:       defaultStr(req.Encoding, "UTF-8"),
		MimeType:       req.MimeType,
		FilterParams:   req.FilterParams,
		ConfigId:       req.ConfigId,
		SubscribeParts: req.SubscribeParts,
	}
	if req.InputPath != "" {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Path{Path: req.InputPath}}
	} else {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: req.InputBytes}}
	}
	if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
		return nil, fmt.Errorf("send header: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return nil, fmt.Errorf("CloseSend: %w", err)
	}

	parts, err := drainProcessResponses(stream)
	if err != nil {
		return nil, fmt.Errorf("drain stream: %w", err)
	}
	return parts, nil
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
	// The daemon's GenericFilterWriter expects a real path; an empty
	// string trips a FileNotFoundException. Use a per-test temp file
	// and read the bytes back when ProcessComplete arrives.
	outPath := filepath.Join(t.TempDir(), "bridge-roundtrip.out")
	target := defaultStr(req.TargetLocale, "fr")
	header := &pb.ProcessHeader{
		FilterClass:    req.FilterClass,
		SourceLocale:   defaultStr(req.SourceLocale, "en"),
		TargetLocale:   target,
		OutputLocale:   target,
		Encoding:       defaultStr(req.Encoding, "UTF-8"),
		MimeType:       req.MimeType,
		FilterParams:   req.FilterParams,
		ConfigId:       req.ConfigId,
		SubscribeParts: req.SubscribeParts,
		Output:         &pb.OutputRef{Destination: &pb.OutputRef_Path{Path: outPath}},
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
		// EOF from Send means the daemon already half-closed the stream
		// (it has a Complete{Error} or ReadDone in flight that we will
		// pick up on the next Recv). Stop trying to send echoes — keep
		// receiving so the trailing message reaches the Complete handler
		// and surfaces the real diagnostic instead of a generic EOF.
		sendEcho := func(req *pb.ProcessRequest) {
			if err := stream.Send(req); err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("RunBridgeRoundTrip: echo: %v", err)
			}
		}
		switch m := resp.Response.(type) {
		case *pb.ProcessResponse_Part:
			parts = append(parts, protoconvert.ProtoToPart(m.Part))
			outPart := maybeTransformPart(m.Part, req.Transform)
			sendEcho(&pb.ProcessRequest{Request: &pb.ProcessRequest_Part{Part: outPart}})
		case *pb.ProcessResponse_PartBatch:
			for _, p := range m.PartBatch.Parts {
				parts = append(parts, protoconvert.ProtoToPart(p))
				outPart := maybeTransformPart(p, req.Transform)
				sendEcho(&pb.ProcessRequest{Request: &pb.ProcessRequest_Part{Part: outPart}})
			}
		case *pb.ProcessResponse_ContentBatch:
			for _, cb := range m.ContentBatch.Blocks {
				parts = append(parts, protoconvert.ContentBlockToPart(cb))
				outCb := maybeTransformContentBlock(cb, req.Transform)
				sendEcho(&pb.ProcessRequest{Request: &pb.ProcessRequest_ContentBlock{ContentBlock: outCb}})
			}
		case *pb.ProcessResponse_ReadDone:
			if err := stream.CloseSend(); err != nil {
				t.Fatalf("RunBridgeRoundTrip: CloseSend after ReadDone: %v", err)
			}
		case *pb.ProcessResponse_Complete:
			if m.Complete.Error != "" {
				t.Fatalf("RunBridgeRoundTrip: daemon error: %s", m.Complete.Error)
			}
			// Inline output is empty when OutputRef is used (per
			// neokapi_bridge.proto). Fall back to reading the on-disk
			// artifact the daemon wrote at outPath.
			output = m.Complete.Output
			if len(output) == 0 {
				bytes, err := os.ReadFile(outPath)
				if err != nil {
					t.Fatalf("RunBridgeRoundTrip: read output %s: %v", outPath, err)
				}
				output = bytes
			}
		}
	}
	return BridgeRoundTripResult{Parts: parts, Output: output}
}

// maybeTransformPart applies the BridgeRequest.Transform hook to a
// PartMessage's Block payload (if present) and returns a fresh
// PartMessage to echo back. When transform is nil the original
// proto is returned unchanged — the daemon-side writer thread sees
// exactly what its reader emitted.
func maybeTransformPart(in *pb.PartMessage, transform func(*model.Block)) *pb.PartMessage {
	if transform == nil || in == nil || in.Block == nil {
		return in
	}
	part := protoconvert.ProtoToPart(in)
	if part == nil {
		return in
	}
	block, ok := part.Resource.(*model.Block)
	if !ok || block == nil {
		return in
	}
	transform(block)
	return protoconvert.PartToProto(part)
}

// maybeTransformContentBlock is the lightweight-encoding sibling of
// maybeTransformPart for the bridge's ContentBlock fast path.
func maybeTransformContentBlock(in *pb.ContentBlock, transform func(*model.Block)) *pb.ContentBlock {
	if transform == nil || in == nil {
		return in
	}
	part := protoconvert.ContentBlockToPart(in)
	if part == nil {
		return in
	}
	block, ok := part.Resource.(*model.Block)
	if !ok || block == nil {
		return in
	}
	transform(block)
	return protoconvert.PartToContentBlock(part)
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
