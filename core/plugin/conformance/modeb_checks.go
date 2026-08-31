package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/clip"
)

// mcpProtocolVersion is the MCP revision the suite advertises in its
// initialize request. A server is free to answer with a different revision —
// the check only requires that it answers with one.
const mcpProtocolVersion = "2025-06-18"

func modeBChecks() []registryEntry {
	return []registryEntry{
		{
			ID:       "modeB.initialize",
			Title:    "`<binary> mcp-server` completes the MCP initialize handshake on stdio",
			Mode:     ModeB,
			Required: true,
			Why:      "kapi proxies its own MCP session to this subprocess; a server that never initializes contributes no tools.",
			run:      checkModeBInitialize,
		},
		{
			ID:       "modeB.tools-list",
			Title:    "tools/list returns every MCP tool the manifest declares",
			Mode:     ModeB,
			Required: true,
			Why:      "the manifest is what kapi builds its dispatch table from; a declared tool the server does not serve is a dead entry.",
			run:      checkModeBToolsList,
		},
		{
			ID:       "modeB.stdout-is-jsonrpc-only",
			Title:    "the MCP server writes nothing but JSON-RPC to stdout",
			Mode:     ModeB,
			Required: true,
			Why:      "stdout is the transport; a stray log line corrupts the stream and the session dies mid-call.",
			run:      checkModeBStdoutClean,
		},
		{
			ID:       "modeB.stdin-close-exits",
			Title:    "closing stdin terminates the MCP server",
			Mode:     ModeB,
			Required: true,
			Why:      "kapi ends a session by closing the pipe; a server that ignores EOF leaks a process per session.",
			run:      checkModeBStdinClose,
		},
	}
}

func (r *runner) requireModeB() (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	if !r.man.IsModeB() {
		return Skip, "manifest declares no MCP tools", nil
	}
	return "", "", nil
}

// mcpSession is one `<binary> mcp-server` subprocess plus its stdio framing.
// The MCP stdio transport is newline-delimited JSON-RPC 2.0, so the framing is
// a json.Encoder on the child's stdin and a json.Decoder on its stdout.
type mcpSession struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	dec *json.Decoder
	enc *json.Encoder

	// Pipes this session owns. Using exec's *Pipe helpers would let cmd.Wait
	// close them underneath the reader goroutine.
	stdin  *os.File
	stdout *os.File
	stderr *os.File

	errMu  sync.Mutex
	errBuf strings.Builder

	// exited is closed by the single reaper goroutine once cmd.Wait returns.
	exited chan struct{}

	nextID int
}

// hasExited reports, without blocking, whether the process has been reaped.
func (s *mcpSession) hasExited() bool {
	select {
	case <-s.exited:
		return true
	default:
		return false
	}
}

// jsonrpcResponse is the subset of a JSON-RPC 2.0 response the suite reads.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Method string `json:"method"`
}

// startMCP spawns the MCP server subprocess. The caller must call close.
func (r *runner) startMCP(ctx context.Context) (*mcpSession, error) {
	cmd := exec.CommandContext(ctx, r.binPath, "mcp-server")
	cmd.Env = r.pluginEnv()

	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		closeAll(inR, inW)
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		closeAll(inR, inW, outR, outW)
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW

	if err := cmd.Start(); err != nil {
		closeAll(inR, inW, outR, outW, errR, errW)
		return nil, fmt.Errorf("start `mcp-server`: %w", err)
	}
	// Drop the ends the child now owns: our copy of its stdin read end (so
	// closing inW reaches the child as EOF) and of its output write ends (so
	// the readers below see EOF when it exits).
	closeAll(inR, outW, errW)

	s := &mcpSession{
		cmd:    cmd,
		in:     inW,
		dec:    json.NewDecoder(outR),
		enc:    json.NewEncoder(inW),
		stdin:  inW,
		stdout: outR,
		stderr: errR,
		exited: make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()
	go func() {
		buf, _ := io.ReadAll(errR)
		s.errMu.Lock()
		s.errBuf.Write(buf)
		s.errMu.Unlock()
	}()
	return s, nil
}

func closeAll(files ...*os.File) {
	for _, f := range files {
		_ = f.Close()
	}
}

func (s *mcpSession) stderrText() string {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return strings.TrimSpace(s.errBuf.String())
}

// call sends a request and reads responses until it sees the matching id,
// skipping any server-initiated notifications along the way.
func (s *mcpSession) call(method string, params any) (*jsonrpcResponse, error) {
	s.nextID++
	id := s.nextID
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := s.enc.Encode(req); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}
	for {
		var resp jsonrpcResponse
		if err := s.dec.Decode(&resp); err != nil {
			if stderr := s.stderrText(); stderr != "" {
				return nil, fmt.Errorf("read %s response: %w (plugin stderr: %s)",
					method, err, clip.Runes(stderr, 300))
			}
			return nil, fmt.Errorf("read %s response: %w", method, err)
		}
		// A notification has no id; keep reading for the answer.
		if len(resp.ID) == 0 || string(resp.ID) == "null" {
			continue
		}
		var gotID int
		if err := json.Unmarshal(resp.ID, &gotID); err != nil || gotID != id {
			continue
		}
		if resp.Error != nil {
			return &resp, fmt.Errorf("%s: server error %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		return &resp, nil
	}
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (s *mcpSession) notify(method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return s.enc.Encode(msg)
}

// initialize performs the MCP initialize / notifications/initialized exchange.
func (s *mcpSession) initialize() (*jsonrpcResponse, error) {
	resp, err := s.call("initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "kapi-plugin-conformance",
			"version": ProtocolVersion,
		},
	})
	if err != nil {
		return resp, err
	}
	if err := s.notify("notifications/initialized", nil); err != nil {
		return resp, fmt.Errorf("send notifications/initialized: %w", err)
	}
	return resp, nil
}

// closeStdin closes the child's stdin, which is how kapi ends a session.
func (s *mcpSession) closeStdin() { _ = s.in.Close() }

// waitExit waits up to d for the child to exit, reporting whether it did. It
// never calls Wait itself — the reaper started in startMCP owns that.
func (s *mcpSession) waitExit(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.exited:
		return true
	case <-timer.C:
		return false
	}
}

// close tears the session down unconditionally.
func (s *mcpSession) close() {
	_ = s.stdin.Close()
	if !s.hasExited() && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	<-s.exited
	closeAll(s.stdout, s.stderr)
}

// initializeResult is the subset of the initialize result the suite inspects.
type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	Capabilities map[string]any `json:"capabilities"`
}

func checkModeBInitialize(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeB(); st != "" {
		return st, d, err
	}
	s, err := r.startMCP(ctx)
	if err != nil {
		return Fail, err.Error(), err
	}
	defer s.close()

	resp, err := s.initialize()
	if err != nil {
		return Fail, err.Error(), err
	}
	var res initializeResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return Fail, fmt.Sprintf("initialize result is not an object: %v", err), err
	}
	if res.ProtocolVersion == "" {
		return Fail, "initialize result omits protocolVersion", nil
	}
	if res.ServerInfo.Name == "" {
		return Fail, "initialize result omits serverInfo.name", nil
	}
	return Pass, fmt.Sprintf("server %q (protocol %s)", res.ServerInfo.Name, res.ProtocolVersion), nil
}

// toolsListResult is the subset of tools/list the suite inspects.
type toolsListResult struct {
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
}

// mcpToolNames initializes a session and returns the served tool names.
func (r *runner) mcpToolNames(ctx context.Context) ([]string, error) {
	s, err := r.startMCP(ctx)
	if err != nil {
		return nil, err
	}
	defer s.close()

	if _, err := s.initialize(); err != nil {
		return nil, err
	}
	resp, err := s.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var res toolsListResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("tools/list result is not an object: %w", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, t := range res.Tools {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names, nil
}

func checkModeBToolsList(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeB(); st != "" {
		return st, d, err
	}
	served, err := r.mcpToolNames(ctx)
	if err != nil {
		return Fail, err.Error(), err
	}
	var missing []string
	for _, declared := range r.man.Capabilities.MCPTools {
		if !slices.Contains(served, declared.Name) {
			missing = append(missing, declared.Name)
		}
	}
	if len(missing) > 0 {
		return Fail, fmt.Sprintf("manifest declares %v but tools/list served %v", missing, served), nil
	}
	return Pass, fmt.Sprintf("all %d declared tool(s) served (tools/list: %v)",
		len(r.man.Capabilities.MCPTools), served), nil
}

func checkModeBStdoutClean(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeB(); st != "" {
		return st, d, err
	}
	// The decoder used by the other Mode-B checks tolerates whitespace but not
	// arbitrary text, so this check reads the raw first token off stdout. Any
	// log line the server writes there surfaces as a JSON syntax error whose
	// offset points at the offending bytes.
	s, err := r.startMCP(ctx)
	if err != nil {
		return Fail, err.Error(), err
	}
	defer s.close()

	if _, err := s.initialize(); err != nil {
		if _, ok := errors.AsType[*json.SyntaxError](err); ok {
			return Fail, fmt.Sprintf("stdout carries non-JSON output: %v", err), err
		}
		return Skip, "the initialize handshake did not complete — see modeB.initialize", nil
	}
	return Pass, "stdout parsed as pure JSON-RPC through the handshake", nil
}

func checkModeBStdinClose(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeB(); st != "" {
		return st, d, err
	}
	s, err := r.startMCP(ctx)
	if err != nil {
		return Fail, err.Error(), err
	}
	// initialize first, so the check measures a real session ending rather
	// than a server that happens to exit before it is used.
	if _, err := s.initialize(); err != nil {
		s.close()
		return Skip, "the initialize handshake did not complete — see modeB.initialize", nil
	}

	grace := max(r.suite.timeout()/4, time.Second)
	start := time.Now()
	s.closeStdin()
	if !s.waitExit(grace) {
		s.close()
		return Fail, fmt.Sprintf("still running %s after stdin closed", grace), nil
	}
	return Pass, fmt.Sprintf("exited %s after stdin closed (budget %s)",
		time.Since(start).Round(time.Millisecond), grace), nil
}
