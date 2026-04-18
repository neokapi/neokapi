// Package extractor runs exec-based source extractors — subprocesses
// that accept NUL-separated file paths on stdin and emit NDJSON
// block records on stdout. The protocol is the lightweight
// counterpart to the gRPC DataFormatReader plugin: one-shot, batch,
// transparent to debug, trivial to write in any language.
//
// The NDJSON payload format:
//
//	{"type":"block","document":"<path>","block":{ ...klf.Block... }}
//
// Per-line records. Any other lines on stdout are ignored.
// Non-zero exit status surfaces to the caller as an error with the
// captured stderr attached.
package extractor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/klf"
)

// Spec declares how to launch an exec extractor. Populated from a
// kapi-plugin.json descriptor or an explicit extractor: stanza in a
// .kapi content collection.
type Spec struct {
	// Exec is the argv for the subprocess. First element is the
	// binary (looked up on PATH unless absolute); remaining elements
	// are forwarded verbatim.
	Exec []string
	// WorkDir is the directory to launch the subprocess from.
	// Defaults to the current working directory when empty. The
	// project directory is the usual choice so the subprocess picks
	// up the user's tsconfig / package.json / vite config naturally.
	WorkDir string
	// Timeout bounds the subprocess's runtime. Zero means no bound.
	Timeout time.Duration
}

// Record is one NDJSON line emitted by an extractor. Only "block"
// records are payload-bearing today; the type field is kept open for
// future control messages (progress, warnings, telemetry).
type Record struct {
	Type     string    `json:"type"`
	Document string    `json:"document,omitempty"`
	Block    klf.Block `json:"block,omitempty"`
}

// Run invokes an extractor subprocess, feeds it `paths` as NUL-
// separated input, and returns every block record it emits.
//
// Errors fall into three buckets: subprocess spawn failure, non-zero
// exit (stderr is attached), and malformed NDJSON on stdout.
func Run(ctx context.Context, spec Spec, paths []string) ([]Record, error) {
	if len(spec.Exec) == 0 {
		return nil, errors.New("extractor: empty Exec")
	}

	runCtx := ctx
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, spec.Exec[0], spec.Exec[1:]...) // #nosec G204 — argv supplied by trusted project config
	cmd.Dir = spec.WorkDir
	cmd.Stdin = bytes.NewReader(joinNUL(paths))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("extractor: stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("extractor: start %q: %w", spec.Exec[0], err)
	}

	records, parseErr := parseNDJSON(stdout)

	if err := cmd.Wait(); err != nil {
		return records, fmt.Errorf("extractor: %q exited with error: %w\nstderr: %s",
			strings.Join(spec.Exec, " "), err, strings.TrimSpace(stderr.String()))
	}
	if parseErr != nil {
		return records, fmt.Errorf("extractor: parse stdout: %w", parseErr)
	}
	return records, nil
}

// parseNDJSON reads line-delimited JSON records from r. A blank line
// or a line that doesn't start with `{` is treated as noise (progress
// text, log lines) and skipped. Records with an unknown `type` are
// passed through so callers can ignore them uniformly.
func parseNDJSON(r io.Reader) ([]Record, error) {
	var out []Record
	scanner := bufio.NewScanner(r)
	// Allow large individual JSON lines — a block with many runs +
	// placeholders can run to tens of KB.
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}
		var rec Record
		if err := json.Unmarshal(trimmed, &rec); err != nil {
			return out, fmt.Errorf("malformed NDJSON on stdout: %q: %w", string(trimmed), err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read stdout: %w", err)
	}
	return out, nil
}

func joinNUL(paths []string) []byte {
	if len(paths) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for i, p := range paths {
		if i > 0 {
			buf.WriteByte(0)
		}
		buf.WriteString(p)
	}
	buf.WriteByte(0)
	return buf.Bytes()
}
