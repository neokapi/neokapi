package cli

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// The vision engine drives the out-of-process kapi-vision plugin, which runs the
// RapidOCR / PP-OCRv4 ONNX models in-process and speaks the binary-framed
// visionproto protocol on stdin/stdout. Keeping the models + onnxruntime out of
// the kapi binary is deliberate (same rationale as the SaT plugin). The host
// knows the wire format but does not import the plugin module, so the CLI stays
// free of the native build requirements — the small framing + structs below
// mirror plugins/vision/visionproto (visionproto-framed-v1).
func init() {
	vision.RegisterEngine("vision", newVisionEngine)
}

const visionPluginName = "vision"

// visionTransport is the kapi-vision round trip, abstracted so the engine is
// testable without spawning a subprocess. OCR is by path: only the path crosses
// to the plugin, never the image bytes.
type visionTransport interface {
	ocr(imagePath, lang, modelName string) (*vision.OCRResult, error)
	io.Closer
}

// visionEngine implements vision.Engine by delegating OCR to the plugin over
// visionTransport. The transport (and its subprocess) is created once, on first
// OCR, and reused so the models load only once.
type visionEngine struct {
	once      sync.Once
	transport visionTransport
	initErr   error
}

func newVisionEngine() (vision.Engine, error) { return &visionEngine{}, nil }

func (e *visionEngine) OCR(ctx context.Context, imagePath string, opts vision.OCROptions) (*vision.OCRResult, error) {
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
	return e.transport.ocr(imagePath, opts.Lang, "")
}

func (e *visionEngine) Close() error {
	if e.transport != nil {
		err := e.transport.Close()
		e.transport = nil
		return err
	}
	return nil
}

// dial locates the kapi-vision plugin and starts its serve loop.
func (e *visionEngine) dial() (visionTransport, error) {
	p, err := findVisionPlugin()
	if err != nil {
		return nil, err
	}
	return startVisionProcess(p.BinaryPath)
}

func findVisionPlugin() (*pluginhost.Plugin, error) {
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
	})
	for _, p := range plugins {
		if p.Name() == visionPluginName {
			return p, nil
		}
	}
	return nil, fmt.Errorf(
		"vision (OCR) requires the %q plugin; install it with `kapi plugins install vision` "+
			"or build it locally (see plugins/vision/README.md)",
		visionPluginName)
}

// --- wire format (mirrors plugins/vision/visionproto, intentionally duplicated) ---

type visionRequest struct {
	Op    string `json:"op"`
	Path  string `json:"path,omitempty"`
	Lang  string `json:"lang,omitempty"`
	Model string `json:"model,omitempty"`
}

type visionResponse struct {
	Version string            `json:"version,omitempty"`
	Error   string            `json:"error,omitempty"`
	OCR     *visionOCRResult  `json:"ocr,omitempty"`
	Models  []json.RawMessage `json:"models,omitempty"`
}

type visionOCRResult struct {
	Width  int             `json:"width"`
	Height int             `json:"height"`
	Lines  []visionOCRLine `json:"lines"`
}

type visionOCRLine struct {
	Text       string  `json:"text"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Confidence float64 `json:"confidence"`
}

const visionMaxFrame = 256 << 20

func visionWriteMessage(w io.Writer, header any, payload []byte) error {
	hb, err := json.Marshal(header)
	if err != nil {
		return err
	}
	if err := visionWriteFrame(w, hb); err != nil {
		return err
	}
	return visionWriteFrame(w, payload)
}

func visionWriteFrame(w io.Writer, b []byte) error {
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(b)))
	if _, err := w.Write(l[:]); err != nil {
		return err
	}
	if len(b) > 0 {
		_, err := w.Write(b)
		return err
	}
	return nil
}

func visionReadFrame(r io.Reader) ([]byte, error) {
	var l [4]byte
	if _, err := io.ReadFull(r, l[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(l[:])
	if n > visionMaxFrame {
		return nil, fmt.Errorf("vision: frame too large (%d)", n)
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func visionReadResponse(r io.Reader) (visionResponse, error) {
	hb, err := visionReadFrame(r)
	if err != nil {
		return visionResponse{}, err
	}
	var resp visionResponse
	if err := json.Unmarshal(hb, &resp); err != nil {
		return visionResponse{}, err
	}
	if _, err := visionReadFrame(r); err != nil { // trailing payload frame
		return visionResponse{}, err
	}
	return resp, nil
}

// visionProcess is the live kapi-vision subprocess and its framed transport.
type visionProcess struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	stdin  io.WriteCloser
	stdout io.ReadCloser
	once   sync.Once
}

func startVisionProcess(bin string) (*visionProcess, error) {
	cmd := exec.Command(bin, "serve")
	cmd.Stderr = os.Stderr // forward first-run model-download progress
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("vision plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("vision plugin stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start vision plugin %q: %w", bin, err)
	}
	return &visionProcess{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

func (p *visionProcess) ocr(imagePath, lang, modelName string) (*vision.OCRResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := visionWriteMessage(p.stdin, visionRequest{Op: "ocr", Path: imagePath, Lang: lang, Model: modelName}, nil); err != nil {
		return nil, err
	}
	resp, err := visionReadResponse(p.stdout)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("vision: %s", resp.Error)
	}
	if resp.OCR == nil {
		return &vision.OCRResult{}, nil
	}
	out := &vision.OCRResult{Width: resp.OCR.Width, Height: resp.OCR.Height}
	for _, ln := range resp.OCR.Lines {
		out.Lines = append(out.Lines, vision.OCRLine{
			Text:       ln.Text,
			BBox:       model.Rect{X: ln.X, Y: ln.Y, W: ln.W, H: ln.H},
			Confidence: ln.Confidence,
		})
	}
	return out, nil
}

func (p *visionProcess) Close() error {
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
