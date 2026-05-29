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
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// The voice-similarity check drives the out-of-process kapi-check plugin, which
// runs a small multilingual sentence-embedding ONNX model in-process and speaks
// a line-delimited JSON protocol on stdin/stdout. The model and its native
// stack live only in the plugin; the host knows the wire protocol but does not
// import the plugin module, so the CLI stays free of the heavy build.
//
// It is a proxy, not a verdict: low cosine similarity to a profile's on-voice
// examples is reported as a MINOR, advisory finding ("reads off-voice"), never
// a hard gate.

const checkPluginName = "check"

// voiceCheckRequest / voiceCheckResponse mirror the kapi-check protocol
// (checkproto-line-json-v1, see plugins/check/checkproto). Duplicated
// intentionally so the CLI does not depend on the plugin module.
type voiceCheckRequest struct {
	ID    int64    `json:"id,omitempty"`
	Op    string   `json:"op"`
	Text  string   `json:"text,omitempty"`
	Refs  []string `json:"refs,omitempty"`
	Model string   `json:"model,omitempty"`
}

type voiceCheckResponse struct {
	ID     int64     `json:"id,omitempty"`
	Scores []float64 `json:"scores,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// voiceTransport is the kapi-check similarity round trip, abstracted so the
// check logic is testable without spawning a subprocess.
type voiceTransport interface {
	similarity(text string, refs []string) ([]float64, error)
}

// DefaultVoiceSimilarity is the cosine cutoff below which a block's source text
// is flagged as reading off-voice relative to its closest profile example.
const DefaultVoiceSimilarity = 0.80

// voiceExamples returns the on-voice reference texts from a profile (its
// examples' "after" strings — the desired voice). Empty when the profile has no
// examples, in which case the voice check has nothing to compare against.
func voiceExamples(p *brand.VoiceProfile) []string {
	if p == nil {
		return nil
	}
	refs := make([]string, 0, len(p.Examples))
	for _, ex := range p.Examples {
		if ex.After != "" {
			refs = append(refs, ex.After)
		}
	}
	return refs
}

// voiceSimilarityFindings flags blocks whose source reads off-voice: it asks the
// transport for the cosine similarity of each block to the on-voice references
// and, when the best match is below threshold, emits a minor advisory finding.
func voiceSimilarityFindings(blocks []*model.Block, refs []string, t voiceTransport, threshold float64) ([]check.Finding, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if threshold <= 0 {
		threshold = DefaultVoiceSimilarity
	}
	var findings []check.Finding
	for _, b := range blocks {
		text := b.SourceText()
		if text == "" {
			continue
		}
		scores, err := t.similarity(text, refs)
		if err != nil {
			return nil, err
		}
		best := 0.0
		for _, s := range scores {
			if s > best {
				best = s
			}
		}
		if best < threshold {
			findings = append(findings, check.Finding{
				Category:     "voice",
				Severity:     check.SeverityMinor,
				Message:      fmt.Sprintf("Reads off-voice — closest brand example similarity %.2f (below %.2f)", best, threshold),
				Suggestion:   "Rephrase toward the brand voice examples, or widen the examples if this is acceptable",
				OriginalText: text,
				Metadata:     map[string]string{"similarity": fmt.Sprintf("%.3f", best)},
			})
		}
	}
	return findings, nil
}

// dialVoicePlugin discovers and starts the kapi-check plugin, returning a
// transport. It fails closed with guidance when the plugin is not installed —
// no silent download, the deterministic checks still ran.
func dialVoicePlugin(ctx context.Context) (voiceTransport, func(), error) {
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
	})
	var bin string
	for _, p := range plugins {
		if p.Name() == checkPluginName {
			bin = p.BinaryPath
			break
		}
	}
	if bin == "" {
		return nil, nil, fmt.Errorf(
			"voice check requires the %q plugin; install it with `kapi plugins install check` "+
				"(then `kapi-check pull` downloads the model), or skip it by omitting --voice",
			checkPluginName)
	}
	proc, err := startCheckProcess(ctx, bin)
	if err != nil {
		return nil, nil, err
	}
	return proc, func() { proc.close() }, nil
}

// checkProcess is the live kapi-check subprocess and its transport.
type checkProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	enc *json.Encoder
	sc  *bufio.Scanner
	id  int64
}

func startCheckProcess(ctx context.Context, bin string) (*checkProcess, error) {
	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("check plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("check plugin stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start check plugin %q: %w", bin, err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return &checkProcess{cmd: cmd, enc: json.NewEncoder(stdin), sc: sc}, nil
}

func (s *checkProcess) similarity(text string, refs []string) ([]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id++
	if err := s.enc.Encode(voiceCheckRequest{ID: s.id, Op: "similarity", Text: text, Refs: refs}); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	if !s.sc.Scan() {
		if err := s.sc.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, io.ErrUnexpectedEOF
	}
	var resp voiceCheckResponse
	if err := json.Unmarshal(s.sc.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Scores, nil
}

func (s *checkProcess) close() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
}
