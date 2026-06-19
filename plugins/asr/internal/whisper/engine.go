// Package whisper is the kapi-asr plugin's transcription engine. It drives a
// bundled whisper.cpp `whisper-cli` executable (MIT) — the same separate-binary
// pattern kapi-av uses for ffmpeg — so the plugin stays pure-Go and builds
// trivially on every platform; only the bundled whisper-cli differs per target.
package whisper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/plugins/asr/asrproto"
)

// Engine transcribes audio by invoking whisper-cli with JSON output.
type Engine struct {
	// Bin is the whisper-cli path; "" resolves "whisper-cli" on PATH (the
	// bundled binary is added to the plugin's PATH/working dir at install).
	Bin string
	// Model is the ggml model file path (required).
	Model string
}

// whisperJSON is the subset of whisper-cli's -oj output we consume.
type whisperJSON struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"` // ms from start
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

// parseWhisperJSON maps whisper-cli JSON to asrproto segments. Confidence is left
// 0 (unknown): whisper-cli's default JSON carries no per-segment confidence — a
// later refinement step treats 0 as "no signal". Empty segments are dropped.
func parseWhisperJSON(b []byte) (language string, segs []asrproto.Segment, err error) {
	var w whisperJSON
	if err := json.Unmarshal(b, &w); err != nil {
		return "", nil, fmt.Errorf("whisper: parse json: %w", err)
	}
	for _, t := range w.Transcription {
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		segs = append(segs, asrproto.Segment{
			Text:    text,
			StartMS: t.Offsets.From,
			EndMS:   t.Offsets.To,
		})
	}
	return w.Result.Language, segs, nil
}

// Transcribe runs whisper-cli over audioPath and returns the detected language
// and segments. lang is an optional hint ("" = auto/model default).
func (e *Engine) Transcribe(ctx context.Context, audioPath, lang string) (language string, segs []asrproto.Segment, err error) {
	bin := e.Bin
	if bin == "" {
		bin = "whisper-cli"
	}
	if e.Model == "" {
		return "", nil, errors.New("whisper: no model configured (set the model path)")
	}
	dir, err := os.MkdirTemp("", "kapi-asr")
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	base := filepath.Join(dir, "out")
	args := []string{"-m", e.Model, "-f", audioPath, "-oj", "-of", base, "-nt"}
	if lang != "" {
		args = append(args, "-l", lang)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, rerr := cmd.CombinedOutput(); rerr != nil {
		return "", nil, fmt.Errorf("whisper: run %s: %w: %s", bin, rerr, lastLine(out))
	}
	data, err := os.ReadFile(base + ".json")
	if err != nil {
		return "", nil, fmt.Errorf("whisper: read json output: %w", err)
	}
	return parseWhisperJSON(data)
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
