// Command kapi-asr is the out-of-process speech-recognition plugin. Like
// kapi-vision, the host discovers and spawns it ("kapi-asr serve") and drives it
// over a stdin/stdout protocol (asrproto). It is pure-Go and bundles a
// whisper.cpp `whisper-cli` (MIT) per platform — no cgo, so it builds for every
// target; the native code lives in the bundled binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/plugins/asr/asrproto"
	"github.com/neokapi/neokapi/plugins/asr/internal/whisper"
)

const version = "0.1.0"

func main() {
	sub := ""
	if len(os.Args) >= 2 {
		sub = os.Args[1]
	}
	switch sub {
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "kapi-asr:", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	case "doctor":
		// Standard self-check that `kapi plugins doctor` runs.
		os.Exit(runDoctor())
	default:
		fmt.Fprintf(os.Stderr, "kapi-asr %s\nusage: kapi-asr serve | kapi-asr version | kapi-asr doctor\n", version)
		os.Exit(2)
	}
}

// runDoctor reports the resolved whisper-cli and model. It fails only on a real
// defect — the bundled whisper-cli not resolving — since the model is acquired
// on demand and a missing one is a readiness note, not a broken install.
func runDoctor() int {
	bin := resolveBin()
	model := resolveModel()
	fmt.Printf("kapi-asr %s\n  whisper-cli: %s\n  model: %s\n", version, binStatus(bin), modelStatus(model))
	if !whisperResolvable(bin) {
		return 1
	}
	return 0
}

// whisperResolvable reports whether the resolved whisper-cli exists: a bundled
// path must be a real file; the bare "whisper-cli" PATH fallback is resolved via
// exec.LookPath.
func whisperResolvable(bin string) bool {
	if bin == "" {
		return false
	}
	if filepath.IsAbs(bin) || strings.ContainsRune(bin, filepath.Separator) {
		return fileExists(bin)
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

func binStatus(bin string) string {
	if whisperResolvable(bin) {
		return bin
	}
	return bin + " (not found)"
}

func serve() error {
	eng := &whisper.Engine{
		Bin:   resolveBin(),
		Model: resolveModel(),
	}
	return asrproto.Serve(os.Stdin, os.Stdout, func(req asrproto.Request) asrproto.Response {
		switch req.Op {
		case asrproto.OpPing:
			return asrproto.Response{OK: true, Version: version}
		case asrproto.OpInfo:
			return asrproto.Response{Version: version, Models: modelInfo(eng.Model)}
		case asrproto.OpTranscribe:
			if req.AudioPath == "" {
				return asrproto.Response{Error: "transcribe: empty audioPath"}
			}
			lang, segs, err := eng.Transcribe(context.Background(), req.AudioPath, req.Lang)
			if err != nil {
				return asrproto.Response{Error: err.Error()}
			}
			return asrproto.Response{Language: lang, Segments: segs}
		default:
			return asrproto.Response{Error: fmt.Sprintf("unknown op %q", req.Op)}
		}
	})
}

// resolveBin finds whisper-cli: $KAPI_ASR_WHISPER_BIN, else a binary bundled
// beside this plugin, else "whisper-cli" on PATH.
func resolveBin() string {
	if b := os.Getenv("KAPI_ASR_WHISPER_BIN"); b != "" {
		return b
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "whisper-cli")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return "whisper-cli"
}

// resolveModel finds the ggml model: $KAPI_ASR_MODEL, else the first ggml-*.bin
// bundled beside the plugin binary (the release layout), else "".
func resolveModel() string {
	if m := os.Getenv("KAPI_ASR_MODEL"); m != "" {
		return m
	}
	if exe, err := os.Executable(); err == nil {
		if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "ggml-*.bin")); len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

func modelStatus(m string) string {
	if m == "" {
		return "(none — set KAPI_ASR_MODEL or bundle a ggml-*.bin beside the binary)"
	}
	if fileExists(m) {
		return m
	}
	return m + " (missing)"
}

func modelInfo(model string) []asrproto.ModelInfo {
	if model == "" {
		return nil
	}
	return []asrproto.ModelInfo{{
		Name:    filepath.Base(model),
		Loaded:  fileExists(model),
		Default: true,
	}}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
