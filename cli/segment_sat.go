package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

// The sat segmenter engine drives the out-of-process kapi-sat plugin, which
// runs the wtpsplit "Segment any Text" ONNX model in-process and speaks a
// line-delimited JSON protocol on stdin/stdout. Keeping the model out of the
// kapi binary is deliberate: its onnxruntime + tokenizer cgo stack ships only
// in the plugin. The host knows the wire protocol but does not import the
// plugin module, so the CLI stays free of the heavy native dependencies.
func init() {
	segment.RegisterEngine("sat", newSatEngine)
}

const satPluginName = "sat"

// satRequest / satResponse mirror the kapi-sat protocol (satproto-line-json-v1,
// see plugins/sat/satproto). Duplicated intentionally so the CLI does not
// depend on the plugin module.
type satRequest struct {
	ID        int64   `json:"id,omitempty"`
	Op        string  `json:"op"`
	Text      string  `json:"text,omitempty"`
	Lang      string  `json:"lang,omitempty"`
	Model     string  `json:"model,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

type satResponse struct {
	ID         int64  `json:"id,omitempty"`
	Boundaries []int  `json:"boundaries,omitempty"`
	OK         bool   `json:"ok,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

// satTransport is the kapi-sat round trip, abstracted so the engine is
// testable without spawning a subprocess.
type satTransport interface {
	segment(text, modelName, lang string, threshold float64) ([]int, error)
}

// satEngine implements segment.Segmenter by delegating boundary detection to
// the kapi-sat plugin over satTransport. The transport (and its subprocess)
// is created once, on first use, and reused so the model loads only once.
type satEngine struct {
	cfg       segment.Config
	once      sync.Once
	transport satTransport
	initErr   error
}

func newSatEngine(cfg segment.Config) (segment.Segmenter, error) {
	return &satEngine{cfg: cfg}, nil
}

func (e *satEngine) Layer() string { return segment.LayerSentence }

func (e *satEngine) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	e.once.Do(func() {
		if e.transport == nil {
			e.transport, e.initErr = e.dial(ctx)
		}
	})
	if e.initErr != nil {
		return nil, e.initErr
	}
	fl := segment.Flatten(runs, e.cfg.Mask)
	text := fl.Text()
	if text == "" {
		return nil, nil
	}
	boundaries, err := e.transport.segment(text, e.cfg.SatModel, string(loc), e.cfg.Threshold)
	if err != nil {
		return nil, fmt.Errorf("sat: %w", err)
	}
	return fl.Spans(boundaries), nil
}

// dial locates the kapi-sat plugin (unless an explicit PluginPath is set) and
// starts its serve loop. The process is bound to ctx — the pipeline context of
// the first block — so cancelling the run tears the subprocess down.
func (e *satEngine) dial(ctx context.Context) (satTransport, error) {
	bin := e.cfg.PluginPath
	if bin == "" {
		p, err := findSatPlugin()
		if err != nil {
			return nil, err
		}
		bin = p.BinaryPath
	}
	return startSatProcess(ctx, bin)
}

// findSatPlugin discovers the installed kapi-sat plugin by name.
func findSatPlugin() (*pluginhost.Plugin, error) {
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
	})
	for _, p := range plugins {
		if p.Name() == satPluginName {
			return p, nil
		}
	}
	return nil, fmt.Errorf(
		"sat segmenter requires the %q plugin; install it with `kapi plugins install sat` "+
			"or build it locally with `make build-sat-plugin-onnx` (see plugins/sat/README.md)",
		satPluginName)
}

// satProcess is the live kapi-sat subprocess and its line-delimited transport.
// Round trips are serialized so a single warm process serves all blocks.
type satProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	enc *json.Encoder
	sc  *bufio.Scanner
	id  int64
}

func startSatProcess(ctx context.Context, bin string) (*satProcess, error) {
	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Stderr = os.Stderr // forward first-run model-download progress
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sat plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sat plugin stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sat plugin %q: %w", bin, err)
	}
	sc := bufio.NewScanner(stdout)
	// Boundary lists for large blocks can exceed bufio's default line cap.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return &satProcess{cmd: cmd, enc: json.NewEncoder(stdin), sc: sc}, nil
}

func (s *satProcess) segment(text, modelName, lang string, threshold float64) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id++
	if err := s.enc.Encode(satRequest{
		ID: s.id, Op: "segment", Text: text, Lang: lang, Model: modelName, Threshold: threshold,
	}); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	if !s.sc.Scan() {
		if err := s.sc.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, io.ErrUnexpectedEOF
	}
	var resp satResponse
	if err := json.Unmarshal(s.sc.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Boundaries, nil
}
