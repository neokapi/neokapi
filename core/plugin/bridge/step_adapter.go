package bridge

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/shared"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// BridgeStepTool implements tool.Tool by delegating to an Okapi pipeline step
// running in the Java bridge via the ProcessStep gRPC RPC.
type BridgeStepTool struct {
	tool.BaseTool
	registry           *BridgeRegistry
	bridgeCfg          BridgeConfig
	stepClass          string
	stepParams         map[string]any
	toolSchema         *schema.ComponentSchema
	sourceLocale       string
	targetLocale       string
	rootDirectory      string
	inputRootDirectory string
}

var _ tool.Tool = (*BridgeStepTool)(nil)

// NewBridgeStepTool creates a step tool that delegates to a Java bridge step.
func NewBridgeStepTool(
	registry *BridgeRegistry,
	cfg BridgeConfig,
	stepClass string,
	name string,
	description string,
	toolSchema *schema.ComponentSchema,
) *BridgeStepTool {
	t := &BridgeStepTool{
		registry:   registry,
		bridgeCfg:  cfg,
		stepClass:  stepClass,
		toolSchema: toolSchema,
	}
	t.ToolName = name
	t.ToolDescription = description
	if toolSchema != nil {
		t.SchemaFn = func() *schema.ComponentSchema { return toolSchema }
	}
	return t
}

// SetStepParams sets the step-specific parameters.
func (t *BridgeStepTool) SetStepParams(params map[string]any) {
	t.stepParams = params
}

// SetLocales sets the source and target locales for the step.
func (t *BridgeStepTool) SetLocales(source, target string) {
	t.sourceLocale = source
	t.targetLocale = target
}

// SetRootDirectory sets the project root directory for ${rootDir} resolution.
func (t *BridgeStepTool) SetRootDirectory(dir string) {
	t.rootDirectory = dir
}

// SetInputRootDirectory sets the input root directory for ${inputRootDir} resolution.
func (t *BridgeStepTool) SetInputRootDirectory(dir string) {
	t.inputRootDirectory = dir
}

// Process reads Parts from the input channel, sends them through the Java bridge
// step via ProcessStep gRPC, and writes processed parts to the output channel.
func (t *BridgeStepTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	bridge, release, err := t.registry.Acquire(t.bridgeCfg)
	if err != nil {
		return fmt.Errorf("acquiring bridge for step %s: %w", t.stepClass, err)
	}
	defer release()

	return t.processWithBridge(ctx, bridge, in, out)
}

func (t *BridgeStepTool) processWithBridge(
	ctx context.Context,
	b *JavaBridge,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	b.mu.RLock()
	if !b.running {
		b.mu.RUnlock()
		return errors.New("bridge not running")
	}
	client := b.client
	b.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, b.cfg.streamTimeout())
	defer cancel()

	stream, err := client.ProcessStep(ctx)
	if err != nil {
		return fmt.Errorf("process step: %w", err)
	}

	// 1. Send header.
	header := &pb.StepHeader{
		StepClass:          t.stepClass,
		StepParams:         encodeFilterParams(t.stepParams),
		ParamTypes:         extractParamTypes(t.toolSchema, t.stepParams),
		SourceLocale:       t.sourceLocale,
		TargetLocale:       t.targetLocale,
		RootDirectory:      t.rootDirectory,
		InputRootDirectory: t.inputRootDirectory,
	}
	if err := stream.Send(&pb.StepRequest{
		Request: &pb.StepRequest_Header{Header: header},
	}); err != nil {
		return fmt.Errorf("step send header: %w", err)
	}

	// 2. Start receiving goroutine.
	recvDone := make(chan error, 1)
	recvParts := make(chan *model.Part, 4096)
	go func() {
		defer close(recvParts)
		for {
			resp, err := stream.Recv()
			if err != nil {
				recvDone <- fmt.Errorf("step recv: %w", err)
				return
			}
			switch r := resp.Response.(type) {
			case *pb.StepResponse_Part:
				part := shared.ProtoToPart(r.Part)
				select {
				case recvParts <- part:
				case <-ctx.Done():
					recvDone <- ctx.Err()
					return
				}
			case *pb.StepResponse_Complete:
				recvDone <- nil
				return
			}
		}
	}()

	// 3. Forward received parts to output in a goroutine.
	outputDone := make(chan error, 1)
	go func() {
		for part := range recvParts {
			select {
			case out <- part:
			case <-ctx.Done():
				outputDone <- ctx.Err()
				return
			}
		}
		outputDone <- nil
	}()

	// 4. Send input parts to the step.
	for {
		select {
		case part, ok := <-in:
			if !ok {
				// Input exhausted — close the send side.
				if err := stream.CloseSend(); err != nil {
					return fmt.Errorf("step close send: %w", err)
				}
				goto waitDone
			}
			msg := shared.PartToProto(part)
			if err := stream.Send(&pb.StepRequest{
				Request: &pb.StepRequest_Part{Part: msg},
			}); err != nil {
				return fmt.Errorf("step send part: %w", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

waitDone:
	// 5. Wait for recv goroutine to finish (StepComplete received).
	if err := <-recvDone; err != nil {
		return err
	}
	// 6. Wait for output goroutine to finish forwarding.
	if err := <-outputDone; err != nil {
		return err
	}
	return nil
}
