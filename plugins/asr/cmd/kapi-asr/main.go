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
	"path/filepath"

	"github.com/neokapi/neokapi/plugins/asr/asrproto"
	"github.com/neokapi/neokapi/plugins/asr/internal/whisper"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "kapi-asr %s\nusage: kapi-asr serve\n", version)
		// A bare invocation prints version and exits 0 so discovery probes work.
		return
	}
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, "kapi-asr:", err)
		os.Exit(1)
	}
}

func serve() error {
	eng := &whisper.Engine{
		Bin:   resolveBin(),
		Model: os.Getenv("KAPI_ASR_MODEL"),
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
