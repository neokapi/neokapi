// Package exec implements kapi's `exec` format — a declarative
// extractor wrapper around any subprocess that accepts NUL-separated
// file paths on stdin and emits NDJSON block records on stdout.
//
// Projects opt in via a FormatSpec in their .kapi:
//
//	format:
//	  name: exec
//	  config:
//	    command: "vp kapi-react extract --stream"
//
// A flow runs the declared command once per collection with every
// matched file path on stdin and streams the emitted blocks through
// the rest of the pipeline. The protocol is transparent to debug
// (plain JSON), trivial to write in any language, and carries no
// kapi-specific schema beyond the klf.Block shape the subprocess
// emits.
//
// Records look like:
//
//	{"type":"block","document":"<path>","block":{ ...klf.Block... }}
//
// Per-line. Other lines on stdout are ignored. Non-zero exit status
// surfaces as an error with captured stderr attached.
package exec

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	gexec "os/exec"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/klf"
)

// FormatName is the name under which this format is declared in a
// .kapi project (`format: { name: exec, ... }`). Exposed as a
// constant so the CLI can dispatch without stringly-typing it.
const FormatName = "exec"

// Spec declares how to launch the extractor subprocess. Populated
// from the FormatSpec.Config.command field in a .kapi content
// declaration.
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
	Block    klf.Block `json:"block,omitzero"`
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

	cmd := gexec.CommandContext(runCtx, spec.Exec[0], spec.Exec[1:]...) // #nosec G204 — argv supplied by trusted project config
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

// DecodeLine parses one raw NDJSON line into a Record. Blank lines
// and lines that don't start with `{` are treated as noise and
// yield ok=false with nil error so callers can scan uniformly.
// Malformed JSON on an otherwise record-shaped line returns an
// error. Shared with `kapi pack` for stdin-mode ingestion.
func DecodeLine(line []byte) (Record, bool, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Record{}, false, nil
	}
	var rec Record
	if err := json.Unmarshal(trimmed, &rec); err != nil {
		return Record{}, false, fmt.Errorf("malformed NDJSON: %q: %w", string(trimmed), err)
	}
	return rec, true, nil
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
