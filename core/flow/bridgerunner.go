package flow

import (
	"context"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/tool"
)

// BridgeRunnerConfig configures a BridgeRunner.
type BridgeRunnerConfig struct {
	SourceLocale string
	TargetLocale string
	Encoding     string
}

// BridgeRunner runs a single-pass bridge pipeline where the Java process
// controls the read/write loop and Go processes parts inline. This is the
// bridge counterpart to FileRunner: Java reads, sends parts to Go, Go
// processes and returns them, Java writes — all in one filter iteration.
//
// Shared by CLI, desktop, and MCP when processing bridge format files.
type BridgeRunner struct {
	cfg BridgeRunnerConfig
}

// NewBridgeRunner creates a BridgeRunner with the given configuration.
func NewBridgeRunner(cfg BridgeRunnerConfig) *BridgeRunner {
	if cfg.Encoding == "" {
		cfg.Encoding = "UTF-8"
	}
	return &BridgeRunner{cfg: cfg}
}

// RunFile processes a single file through the bridge pipeline. The
// bridgeReader must be a BridgeFormatReader obtained from the format registry.
// Tools should already be configured (tracing wrappers, parallel blocks, etc.).
func (r *BridgeRunner) RunFile(ctx context.Context, flowName string, tools []tool.Tool, bridgeReader *bridge.BridgeFormatReader, inputPath, outputPath string) error {
	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	processor := bridgeReader.NewProcessor()
	_, err := processor.Execute(ctx, bridge.ProcessExecuteParams{
		InputPath:      inputPath,
		SourceLocale:   r.cfg.SourceLocale,
		TargetLocale:   r.cfg.TargetLocale,
		OutputPath:     outputPath,
		OutputLocale:   r.cfg.TargetLocale,
		Encoding:       r.cfg.Encoding,
		SubscribeParts: []int32{int32(model.PartBlock)},
	}, func(parts <-chan *model.Part) <-chan *model.Part {
		fb := NewFlow(flowName)
		for _, t := range tools {
			fb.AddTool(t)
		}
		f, ferr := fb.Build()
		if ferr != nil {
			out := make(chan *model.Part)
			close(out)
			return out
		}

		executor := NewExecutor()
		inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

		go func() {
			for p := range parts {
				inCh <- p
			}
			close(inCh)
		}()

		// Wait for flow completion in a goroutine so outCh can drain.
		go func() {
			_ = wait()
		}()

		return outCh
	})
	return err
}
