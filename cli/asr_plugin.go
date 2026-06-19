package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/asr"
)

// The asr engine drives the out-of-process kapi-asr plugin, which runs a
// whisper.cpp model and speaks the line-delimited JSON asrproto protocol on
// stdin/stdout. Keeping whisper + models out of the kapi binary is deliberate
// (same rationale as kapi-vision/kapi-sat). The host knows the wire format but
// does not import the plugin module, so the CLI stays free of the plugin's build
// requirements — the small structs below mirror plugins/asr/asrproto
// (asrproto-line-json-v1). Audio crosses to the plugin by path, never as bytes.
func init() {
	asr.RegisterEngine("asr", newASREngine)
}

const asrPluginName = "asr"

func newASREngine() (asr.Engine, error) { return &asrEngine{}, nil }

// asrEngine implements asr.Engine by delegating to the plugin over a warm
// subprocess created on first use and reused (so the model loads once).
type asrEngine struct {
	once    sync.Once
	proc    *asrProcess
	initErr error
	cancel  context.CancelFunc
}

func (e *asrEngine) Transcribe(ctx context.Context, audioPath string, opts asr.Options) (*asr.Result, error) {
	e.once.Do(func() { e.proc, e.initErr = e.dial() })
	if e.initErr != nil {
		return nil, e.initErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e.proc.transcribe(audioPath, opts.Lang)
}

func (e *asrEngine) Close() error {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if e.proc != nil {
		err := e.proc.Close()
		e.proc = nil
		return err
	}
	return nil
}

func (e *asrEngine) dial() (*asrProcess, error) {
	bin, err := findASRPlugin()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc, err := startASRProcess(ctx, bin)
	if err != nil {
		cancel()
		return nil, err
	}
	e.cancel = cancel
	return proc, nil
}

// findASRPlugin resolves the kapi-asr binary: an explicit KAPI_ASR_PLUGIN path
// (dev/local override) wins; otherwise the unified plugin discovery locates it
// by manifest name, like kapi-vision.
func findASRPlugin() (string, error) {
	if p := os.Getenv("KAPI_ASR_PLUGIN"); p != "" {
		return p, nil
	}
	for _, p := range pluginhost.Discover(pluginhost.DiscoverOptions{EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR")}) {
		if p.Name() == asrPluginName {
			return p.BinaryPath, nil
		}
	}
	return "", fmt.Errorf(
		"audio transcription requires the %q plugin; install it with `kapi plugins install asr` "+
			"or set KAPI_ASR_PLUGIN to a built kapi-asr binary", asrPluginName)
}

// --- wire format (mirrors plugins/asr/asrproto, intentionally duplicated) ---

type asrRequest struct {
	ID        int64  `json:"id,omitempty"`
	Op        string `json:"op"`
	AudioPath string `json:"audioPath,omitempty"`
	Lang      string `json:"lang,omitempty"`
}

type asrSegment struct {
	Text       string  `json:"text"`
	StartMS    int64   `json:"startMs"`
	EndMS      int64   `json:"endMs,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type asrResponse struct {
	ID       int64        `json:"id,omitempty"`
	Segments []asrSegment `json:"segments,omitempty"`
	Language string       `json:"language,omitempty"`
	Error    string       `json:"error,omitempty"`
}

// asrProcess is the live kapi-asr subprocess and its line-JSON transport.
type asrProcess struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	stdin  io.WriteCloser
	sc     *bufio.Scanner
	stdout io.ReadCloser
	id     int64
	once   sync.Once
}

func startASRProcess(ctx context.Context, bin string) (*asrProcess, error) {
	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Stderr = os.Stderr // forward first-run model-download / whisper progress
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("asr plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("asr plugin stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start asr plugin %q: %w", bin, err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 64<<20)
	return &asrProcess{cmd: cmd, stdin: stdin, stdout: stdout, sc: sc}, nil
}

func (p *asrProcess) transcribe(audioPath, lang string) (*asr.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.id++
	req := asrRequest{ID: p.id, Op: "transcribe", AudioPath: audioPath, Lang: lang}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := p.stdin.Write(append(b, '\n')); err != nil {
		return nil, fmt.Errorf("asr: write request: %w", err)
	}
	if !p.sc.Scan() {
		if err := p.sc.Err(); err != nil {
			return nil, fmt.Errorf("asr: read response: %w", err)
		}
		return nil, fmt.Errorf("asr: plugin closed stdout: %w", io.EOF)
	}
	var resp asrResponse
	if err := json.Unmarshal(p.sc.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("asr: decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("asr: %s", resp.Error)
	}
	out := &asr.Result{Language: resp.Language}
	for _, s := range resp.Segments {
		out.Segments = append(out.Segments, asr.Segment{
			Text:       s.Text,
			StartMS:    s.StartMS,
			EndMS:      s.EndMS,
			Confidence: s.Confidence,
		})
	}
	return out, nil
}

func (p *asrProcess) Close() error {
	p.once.Do(func() {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
		}
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		if p.stdout != nil {
			_ = p.stdout.Close()
		}
	})
	return nil
}
