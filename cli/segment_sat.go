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
	"runtime"
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

// satMaxLineBytes bounds a single protocol response line read from the plugin.
// It must match the plugin's own cap (plugins/sat/satproto.MaxLineBytes = 64
// MiB): a boundary list for a very large block can produce a response longer
// than bufio's default 64 KiB cap, and any host scanner buffer smaller than the
// plugin's write cap would silently truncate a valid response into a decode
// error. The constant is duplicated here (rather than imported) for the same
// reason the protocol structs are: the CLI must not depend on the plugin module
// and inherit its native (onnxruntime/cgo) build requirements.
const satMaxLineBytes = 64 << 20

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
// the kapi-sat plugin over satTransport. The transport (and its subprocess) is
// created once, on first use, and reused so the model loads only once.
//
// The subprocess lifetime is deliberately decoupled from any per-call context.
// A single engine (and therefore a single warm plugin process) is reused across
// many blocks and, in the desktop runner, across many files — each with its own
// cancellable per-run context. Binding the child to the first block's context
// would let that run's completion cancel kill the process, leaving every later
// call reading a dead pipe. Instead the child is started under an engine-scoped
// context cancelled only by [satEngine.Close], and a finalizer reaps it if
// Close is never called (the segment tool does not own the engine's lifecycle).
type satEngine struct {
	cfg       segment.Config
	once      sync.Once
	transport satTransport
	initErr   error

	cancel context.CancelFunc // cancels engCtx; tears the subprocess down
	closer io.Closer          // the live process, if dialed; nil for fakes
}

func newSatEngine(cfg segment.Config) (segment.Segmenter, error) {
	return &satEngine{cfg: cfg}, nil
}

func (e *satEngine) Layer() string { return segment.LayerSentence }

func (e *satEngine) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	e.once.Do(func() {
		if e.transport == nil {
			e.transport, e.initErr = e.dial()
		}
	})
	if e.initErr != nil {
		return nil, e.initErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
// starts its serve loop under an engine-scoped context. The process lifetime is
// bound to the engine — not to any single run's pipeline context — so a warm
// plugin survives across files; [satEngine.Close] (or, as a backstop, the
// finalizer) tears it down. A runtime cleanup reaps the child even when nothing
// calls Close, since the segment tool that holds the engine has no Close hook.
func (e *satEngine) dial() (satTransport, error) {
	bin := e.cfg.PluginPath
	if bin == "" {
		p, err := findSatPlugin()
		if err != nil {
			return nil, err
		}
		bin = p.BinaryPath
	}
	engCtx, cancel := context.WithCancel(context.Background())
	proc, err := startSatProcess(engCtx, bin)
	if err != nil {
		cancel()
		return nil, err
	}
	e.cancel = cancel
	e.closer = proc
	// Backstop: if the engine is dropped without Close (the segment tool owns
	// no Close hook), reap the child so it is not orphaned. AddCleanup avoids
	// the resurrection pitfalls of SetFinalizer and must not capture e.
	runtime.AddCleanup(e, func(p *satProcess) { p.close() }, proc)
	return proc, nil
}

// Close tears down the warm kapi-sat subprocess: it cancels the engine context
// and Kills + Waits the child, mirroring the kapi-check plugin's lifecycle. It
// is safe to call more than once and on an engine that never dialed (e.g. one
// backed by a fake transport in tests). Close satisfies io.Closer.
func (e *satEngine) Close() error {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if e.closer != nil {
		err := e.closer.Close()
		e.closer = nil
		return err
	}
	return nil
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
	cmd    *exec.Cmd
	mu     sync.Mutex
	enc    *json.Encoder
	sc     *bufio.Scanner
	id     int64
	stdin  io.Closer
	stdout io.Closer

	closeOnce sync.Once
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
	// Boundary lists for large blocks can exceed bufio's default 64 KiB line
	// cap. Size the buffer to the plugin's own write cap (satMaxLineBytes) so a
	// large but valid response is never truncated into a decode error.
	sc.Buffer(make([]byte, 0, 64*1024), satMaxLineBytes)
	return &satProcess{cmd: cmd, enc: json.NewEncoder(stdin), sc: sc, stdin: stdin, stdout: stdout}, nil
}

// close tears the subprocess down: Kill + Wait reaps the child (so it is never
// left as a zombie), then the pipes are closed. It mirrors the kapi-check
// plugin's checkProcess.close() and is idempotent — Close, the engine
// finalizer, and a context-cancel-driven exit may all race to call it.
func (s *satProcess) close() {
	s.closeOnce.Do(func() {
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
			_ = s.cmd.Wait()
		}
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.stdout != nil {
			_ = s.stdout.Close()
		}
	})
}

// Close satisfies io.Closer so the engine can reap the child uniformly.
func (s *satProcess) Close() error {
	s.close()
	return nil
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
