//go:build !js

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The Gemma provider drives the out-of-process kapi-llm plugin, which runs the
// Gemma 4 ONNX model in-process and speaks a line-delimited JSON protocol on
// stdin/stdout (llmproto-line-json-v1). Keeping the model out of the kapi binary
// is deliberate: its onnxruntime + tokenizer cgo stack ships only in the plugin.
// The host knows the wire protocol but does not import the plugin module, so the
// CLI stays free of the heavy native dependencies — exactly as cli/segment_sat.go
// drives kapi-sat.
//
// Registering here (rather than in the framework's providers/ai) keeps the
// framework platform-agnostic: the plugin-subprocess machinery lives in the cli
// module, which already depends on cli/pluginhost. The provider id is declared in
// providers/ai so it is a known local provider for flow side-effect analysis.
//
// This file is excluded from the js/wasm build (//go:build !js): the browser
// build cannot spawn a subprocess and gets a transformers.js-backed Gemma
// provider instead.
func init() {
	aiprovider.RegisterProvider(
		aiprovider.ProviderInfo{Name: aiprovider.Gemma, Label: "Gemma (local)"},
		func(cfg aiprovider.Config) aiprovider.LLMProvider { return newLLMProvider(cfg) },
	)
}

const llmPluginName = "llm"

// llmMaxLineBytes bounds a single protocol response line read from the plugin.
// It must match the plugin's own cap (plugins/llm/llmproto.MaxLineBytes = 64
// MiB). Duplicated here (not imported) so the CLI does not depend on the plugin
// module and inherit its native build requirements.
const llmMaxLineBytes = 64 << 20

const defaultGemmaModel = "gemma-4-e2b"

// llmWire* mirror the kapi-llm protocol (llmproto-line-json-v1, see
// plugins/llm/llmproto). Duplicated intentionally so the CLI does not depend on
// the plugin module.
type llmWireMedia struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	MIME string `json:"mime,omitempty"`
}

type llmWireMessage struct {
	Role  string         `json:"role"`
	Text  string         `json:"text,omitempty"`
	Media []llmWireMedia `json:"media,omitempty"`
}

type llmWireRequest struct {
	ID          int64            `json:"id,omitempty"`
	Op          string           `json:"op"`
	Messages    []llmWireMessage `json:"messages,omitempty"`
	Model       string           `json:"model,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	TopP        float64          `json:"top_p,omitempty"`
	Schema      json.RawMessage  `json:"schema,omitempty"`
}

type llmWireResponse struct {
	ID           int64  `json:"id,omitempty"`
	Text         string `json:"text,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	OK           bool   `json:"ok,omitempty"`
	Version      string `json:"version,omitempty"`
	Error        string `json:"error,omitempty"`
}

// llmTransport is the kapi-llm round trip, abstracted so the provider is testable
// without spawning a subprocess.
type llmTransport interface {
	generate(req llmWireRequest) (llmWireResponse, error)
	io.Closer
}

// llmProvider implements aiprovider.LLMProvider by delegating generation to the
// kapi-llm plugin over llmTransport. The transport (and its subprocess) is
// created once, on first use, and reused so the multi-GB model loads only once
// for the provider's lifetime.
type llmProvider struct {
	cfg aiprovider.Config

	once      sync.Once
	transport llmTransport
	initErr   error
	cancel    context.CancelFunc
}

func newLLMProvider(cfg aiprovider.Config) *llmProvider {
	if cfg.Model == "" {
		cfg.Model = defaultGemmaModel
	}
	return &llmProvider{cfg: cfg}
}

func (p *llmProvider) Name() aiprovider.ProviderID { return aiprovider.Gemma }

// InputModalities advertises image and audio: the engine runs Gemma 4's vision
// and audio encoders. (Video needs frame extraction and is not accepted here.)
func (p *llmProvider) InputModalities() []aiprovider.Modality {
	return []aiprovider.Modality{aiprovider.ModalityImage, aiprovider.ModalityAudio}
}

func (p *llmProvider) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	// Mirror aiprovider.standardTranslate (unexported): same prompt, so the
	// local model behaves like the cloud providers for translation.
	prompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLanguage, req.TargetLocale, req.Source,
	) + req.Directives()

	resp, err := p.Chat(ctx, []aiprovider.Message{aiprovider.TextMessage("user", prompt)})
	if err != nil {
		return nil, err
	}
	return &aiprovider.TranslateResponse{
		Translation: resp.Content,
		Confidence:  0.7,
		Model:       resp.Model,
		Usage:       resp.Usage,
	}, nil
}

func (p *llmProvider) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return p.generate(ctx, messages, nil)
}

func (p *llmProvider) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	var raw json.RawMessage
	if schema.Schema != nil {
		b, err := json.Marshal(schema.Schema)
		if err != nil {
			return nil, fmt.Errorf("gemma: marshal schema: %w", err)
		}
		raw = b
	}
	return p.generate(ctx, messages, raw)
}

// generate is the shared Chat/ChatStructured path.
func (p *llmProvider) generate(ctx context.Context, messages []aiprovider.Message, schema json.RawMessage) (*aiprovider.ChatResponse, error) {
	t, err := p.ensure()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wire, cleanup, err := toWireMessages(messages)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	resp, err := t.generate(llmWireRequest{
		Op:          "generate",
		Messages:    wire,
		Model:       p.cfg.Model,
		MaxTokens:   p.cfg.MaxTokens,
		Temperature: p.cfg.Temperature,
		Schema:      schema,
	})
	if err != nil {
		return nil, err
	}
	model := p.cfg.Model
	if model == "" {
		model = defaultGemmaModel
	}
	return &aiprovider.ChatResponse{
		Content: resp.Text,
		Model:   model,
		Usage: aiprovider.TokenUsage{
			InputTokens:  resp.InputTokens,
			OutputTokens: resp.OutputTokens,
		},
	}, nil
}

func (p *llmProvider) ensure() (llmTransport, error) {
	p.once.Do(func() {
		if p.transport == nil {
			p.transport, p.initErr = p.dial()
		}
	})
	return p.transport, p.initErr
}

// dial locates the kapi-llm plugin and starts its serve loop under a
// provider-scoped context, so the warm process survives across many Chat calls;
// Close tears it down.
func (p *llmProvider) dial() (llmTransport, error) {
	plugin, err := findLLMPlugin()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc, err := startLLMProcess(ctx, plugin.BinaryPath)
	if err != nil {
		cancel()
		return nil, err
	}
	p.cancel = cancel
	return proc, nil
}

func (p *llmProvider) Close() error {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.transport != nil {
		err := p.transport.Close()
		p.transport = nil
		return err
	}
	return nil
}

// findLLMPlugin discovers the installed kapi-llm plugin by name.
func findLLMPlugin() (*pluginhost.Plugin, error) {
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
	})
	for _, pl := range plugins {
		if pl.Name() == llmPluginName {
			return pl, nil
		}
	}
	return nil, fmt.Errorf(
		"the Gemma (local) provider requires the %q plugin; install it with `kapi plugins install llm` "+
			"or build it locally with `make build-llm-plugin-onnx` (see plugins/llm/README.md)",
		llmPluginName)
}

// toWireMessages converts provider messages to wire form, materializing any
// media to a local path the plugin can open. It returns a cleanup func that
// removes any temp files written for inline media.
func toWireMessages(messages []aiprovider.Message) ([]llmWireMessage, func(), error) {
	var temps []string
	cleanup := func() {
		for _, p := range temps {
			_ = os.Remove(p)
		}
	}
	out := make([]llmWireMessage, 0, len(messages))
	for _, m := range messages {
		wm := llmWireMessage{Role: m.Role, Text: m.Text()}
		for _, part := range m.Parts {
			if part.Kind == aiprovider.ContentText {
				continue // folded into Text via m.Text()
			}
			path, tmp, err := materializeMedia(part.Media)
			if err != nil {
				cleanup()
				return nil, func() {}, fmt.Errorf("gemma: %w", err)
			}
			if tmp != "" {
				temps = append(temps, tmp)
			}
			mime := ""
			if part.Media != nil {
				mime = part.Media.MimeType
			}
			wm.Media = append(wm.Media, llmWireMedia{Kind: string(part.Kind), Path: path, MIME: mime})
		}
		out = append(out, wm)
	}
	return out, cleanup, nil
}

// materializeMedia returns a local filesystem path for a media slice. A local
// URI is used in place; inline Data is written to a temp file (returned as tmp
// so the caller can remove it); blob-store-backed media must be materialized by
// the caller before the provider call.
func materializeMedia(m *model.Media) (path, tmp string, err error) {
	if m == nil {
		return "", "", errors.New("nil media part")
	}
	if isLocalFilePath(m.URI) {
		return strings.TrimPrefix(m.URI, "file://"), "", nil
	}
	if len(m.Data) > 0 {
		f, err := os.CreateTemp("", "kapi-llm-media-*")
		if err != nil {
			return "", "", fmt.Errorf("create temp for media: %w", err)
		}
		name := f.Name()
		if _, err := f.Write(m.Data); err != nil {
			_ = f.Close()
			_ = os.Remove(name)
			return "", "", fmt.Errorf("write media temp: %w", err)
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(name)
			return "", "", fmt.Errorf("close media temp: %w", err)
		}
		return name, name, nil
	}
	if m.BlobKey != "" {
		return "", "", fmt.Errorf("media is blob-store-backed (%q); materialize it before the Gemma provider call", m.BlobKey)
	}
	return "", "", errors.New("media has no inline data or local URI to send")
}

// isLocalFilePath reports whether s is a local filesystem path (not an
// http(s)/data URL).
func isLocalFilePath(s string) bool {
	if s == "" {
		return false
	}
	if filepath.IsAbs(s) || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return true
	}
	return u.Scheme == "" || u.Scheme == "file"
}

// llmProcess is the live kapi-llm subprocess and its line-delimited transport.
// Round trips are serialized so a single warm process serves all calls.
type llmProcess struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	enc    *json.Encoder
	sc     *bufio.Scanner
	id     int64
	stdin  io.Closer
	stdout io.Closer

	closeOnce sync.Once
}

func startLLMProcess(ctx context.Context, bin string) (*llmProcess, error) {
	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Stderr = os.Stderr // forward first-run model-download progress
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("kapi-llm stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("kapi-llm stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start kapi-llm %q: %w", bin, err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), llmMaxLineBytes)
	return &llmProcess{cmd: cmd, enc: json.NewEncoder(stdin), sc: sc, stdin: stdin, stdout: stdout}, nil
}

func (s *llmProcess) generate(req llmWireRequest) (llmWireResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id++
	req.ID = s.id
	if err := s.enc.Encode(req); err != nil {
		return llmWireResponse{}, fmt.Errorf("write request: %w", err)
	}
	if !s.sc.Scan() {
		if err := s.sc.Err(); err != nil {
			return llmWireResponse{}, fmt.Errorf("read response: %w", err)
		}
		return llmWireResponse{}, io.ErrUnexpectedEOF
	}
	var resp llmWireResponse
	if err := json.Unmarshal(s.sc.Bytes(), &resp); err != nil {
		return llmWireResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != "" {
		return llmWireResponse{}, errors.New(resp.Error)
	}
	return resp, nil
}

func (s *llmProcess) close() {
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

func (s *llmProcess) Close() error {
	s.close()
	return nil
}
