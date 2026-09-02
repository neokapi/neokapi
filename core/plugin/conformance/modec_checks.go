package conformance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/neokapi/neokapi/core/clip"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// HandshakeTypeStdio is the only handshake type the protocol defines: the
// daemon prints one JSON line on stdout, then keeps stdout open for logs.
const HandshakeTypeStdio = "stdio-handshake"

// maxGRPCMsgSize matches the host's per-message ceiling. Mode-C plugins stream
// whole documents inline, so the 4 MB gRPC default is too small.
const maxGRPCMsgSize = 256 << 20

// Handshake is the JSON envelope a Mode-C daemon prints as its first stdout
// line, mirroring what the host parses.
type Handshake struct {
	// Socket is the absolute path of the Unix socket the daemon bound.
	Socket string `json:"socket"`

	// Version is the daemon's self-reported version (free-form).
	Version string `json:"version,omitempty"`

	// PID is informational.
	PID int `json:"pid,omitempty"`
}

func modeCChecks() []registryEntry {
	return []registryEntry{
		{
			ID:            "modeC.handshake",
			Title:         "`<binary> daemon` prints a socket handshake as its first stdout line",
			Mode:          ModeC,
			Required:      true,
			Why:           "the host reads exactly one line to learn where to dial; without it the daemon is unreachable and the spawn times out.",
			run:           checkModeCHandshake,
			lifecycleWait: (*runner).startupTimeout,
		},
		{
			ID:       "modeC.socket-listening",
			Title:    "the announced path is a bound Unix socket",
			Mode:     ModeC,
			Required: true,
			Why:      "the host dials `unix://<socket>` and only that; a path that is not a live socket fails the dial.",
			run:      checkModeCSocketListening,
		},
		{
			ID:       "modeC.socket-private",
			Title:    "the socket is not reachable by other users",
			Mode:     ModeC,
			Required: false,
			Why:      "the host dials with insecure transport precisely because the socket is user-private; a permissive mode removes that premise.",
			run:      checkModeCSocketPrivate,
		},
		{
			ID:       "modeC.grpc-ready",
			Title:    "the socket serves gRPC and reaches READY",
			Mode:     ModeC,
			Required: true,
			Why:      "the host probes the connection to READY before dispatching and fails fast when a daemon is not serving.",
			run:      checkModeCGRPCReady,
		},
		{
			ID:       "modeC.bridge-service",
			Title:    "BridgeService is registered on the socket",
			Mode:     ModeC,
			Required: true,
			Why:      "formats, flow tools, and connectors are all dispatched over this service; an unregistered service answers every call with Unimplemented.",
			run:      checkModeCBridgeService,
		},
		{
			ID:       "modeC.format-probe",
			Title:    "the probed format reads the supplied document",
			Mode:     ModeC,
			Required: true,
			Why:      "a declared format that cannot open a document is an entry in the host's format registry that fails at use time.",
			run:      checkModeCFormatProbe,
		},
		{
			ID:       "modeC.segment-rpc",
			Title:    "declared segmenters answer the Segment RPC",
			Mode:     ModeC,
			Required: true,
			Why:      "a declared segmentation engine becomes selectable in the segmentation tool and is dispatched over this RPC.",
			run:      checkModeCSegmentRPC,
		},
		{
			ID:            "modeC.shutdown-rpc",
			Title:         "the Shutdown RPC stops the daemon",
			Mode:          ModeC,
			Required:      false,
			Why:           "graceful shutdown lets the host reclaim a daemon without signalling; the host falls back to SIGTERM, so this is advisory.",
			run:           checkModeCShutdownRPC,
			lifecycleWait: (*runner).shutdownGrace,
		},
		{
			ID:            "modeC.sigterm-exit",
			Title:         "SIGTERM terminates the daemon and releases its socket",
			Mode:          ModeC,
			Required:      true,
			Why:           "the host's pool tears daemons down with SIGTERM on eviction, idle timeout, and exit; a daemon that ignores it is killed and may leak its socket.",
			run:           checkModeCSigtermExit,
			lifecycleWait: (*runner).shutdownGrace,
		},
	}
}

// daemonSession is one `<binary> daemon` subprocess shared by every Mode-C
// check in a run.
type daemonSession struct {
	cmd  *exec.Cmd
	hs   Handshake
	conn *grpc.ClientConn

	// stdout and stderr are the read ends of pipes this session owns. They are
	// deliberately not exec's StdoutPipe/StderrPipe: those are closed by
	// cmd.Wait, which would race the reader goroutines below.
	stdout *os.File
	stderr *os.File

	// logs collects post-handshake stdout, which the protocol designates as
	// log output, so a failure detail can quote it.
	logsMu sync.Mutex
	logs   strings.Builder

	errMu  sync.Mutex
	errBuf strings.Builder

	// exited is closed by the single reaper goroutine once cmd.Wait returns.
	// One owner for Wait is the whole point: the process is reaped exactly
	// once no matter how many checks ask whether it has stopped.
	exited chan struct{}
}

// reap starts the one goroutine allowed to call cmd.Wait.
func (d *daemonSession) reap() {
	d.exited = make(chan struct{})
	go func() {
		_ = d.cmd.Wait()
		close(d.exited)
	}()
}

// hasExited reports, without blocking, whether the process has been reaped.
func (d *daemonSession) hasExited() bool {
	select {
	case <-d.exited:
		return true
	default:
		return false
	}
}

func (d *daemonSession) logText() string {
	d.logsMu.Lock()
	defer d.logsMu.Unlock()
	return strings.TrimSpace(d.logs.String())
}

func (d *daemonSession) stderrText() string {
	d.errMu.Lock()
	defer d.errMu.Unlock()
	return strings.TrimSpace(d.errBuf.String())
}

// diagnostics returns whatever the daemon said on either stream, for a failure
// detail. A Mode-C plugin's real error message almost always lands here.
func (d *daemonSession) diagnostics() string {
	parts := []string{}
	if s := d.stderrText(); s != "" {
		parts = append(parts, "stderr: "+clip.Runes(s, 300))
	}
	if s := d.logText(); s != "" {
		parts = append(parts, "stdout: "+clip.Runes(s, 300))
	}
	return strings.Join(parts, "; ")
}

// waitExit waits up to timeout for the process to exit, reporting whether it
// did. It never calls Wait itself — the reaper owns that.
func (d *daemonSession) waitExit(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-d.exited:
		return true
	case <-timer.C:
		return false
	}
}

// close tears the session down: drop the gRPC connection, kill the process if
// it is still alive, join the reaper, and remove a leaked socket so a rerun can
// bind the same path.
func (d *daemonSession) close() {
	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
	}
	if d.exited != nil {
		if !d.hasExited() && d.cmd.Process != nil {
			_ = d.cmd.Process.Kill()
		}
		<-d.exited
	}
	if d.stdout != nil {
		_ = d.stdout.Close()
	}
	if d.stderr != nil {
		_ = d.stderr.Close()
	}
	if d.hs.Socket != "" {
		_ = os.Remove(d.hs.Socket)
	}
}

// startupTimeout resolves the handshake budget: an explicit Suite.StartupTimeout
// first, then whatever the manifest asked for, then the host's default.
//
// It is deliberately not clamped by Suite.Timeout. Startup is a wait on the
// plugin's own process, not work the suite does, and clamping it made the
// per-check budget double as the startup budget: a caller who shortened Timeout
// for an unrelated reason silently starved startup, and on a loaded machine
// every negative Mode-C case then reported "no handshake line" instead of the
// protocol violation it was written to catch.
func (r *runner) startupTimeout() time.Duration {
	if d := r.suite.StartupTimeout; d > 0 {
		return d
	}
	if r.man != nil && r.man.Daemon != nil && r.man.Daemon.StartupTimeoutSeconds > 0 {
		return time.Duration(r.man.Daemon.StartupTimeoutSeconds) * time.Second
	}
	return DefaultStartupTimeout
}

func (r *runner) requireModeC() (Status, string, error) {
	if st, d, err := r.requireBinary(); st != "" {
		return st, d, err
	}
	if !r.man.IsModeC() {
		return Skip, "manifest declares no formats, flow tools, segmenters, or source connectors", nil
	}
	if !modeCSupported() {
		return Skip, "Mode C is a Unix-socket transport; not exercised on this platform", nil
	}
	return "", "", nil
}

// requireDaemon guards the checks that reuse the session established by
// modeC.handshake.
func (r *runner) requireDaemon() (Status, string, error) {
	if st, d, err := r.requireModeC(); st != "" {
		return st, d, err
	}
	if r.daemon == nil {
		if budget := r.startupBudgetExpired; budget > 0 {
			return Skip, fmt.Sprintf(
				"the daemon printed no handshake within the %s startup budget (see modeC.handshake)", budget), nil
		}
		return Skip, "the daemon did not start (see modeC.handshake)", nil
	}
	return "", "", nil
}

func checkModeCHandshake(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireModeC(); st != "" {
		return st, d, err
	}

	// Deliberately not exec.CommandContext: the daemon must outlive this
	// check's context so the rest of the Mode-C checks can use it. The session
	// is torn down by modeC.sigterm-exit, or by runner.cleanup on any early
	// return.
	cmd := exec.Command(r.binPath, "daemon") //nolint:noctx // long-lived daemon, torn down by the suite
	cmd.Env = r.pluginEnv()

	// Own the pipes rather than using cmd.StdoutPipe/StderrPipe: exec closes
	// those from Wait, which would race the reader goroutines below (the
	// daemon's stdout is read for the whole session, not just at startup).
	outR, outW, err := os.Pipe()
	if err != nil {
		return Fail, fmt.Sprintf("stdout pipe: %v", err), err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_, _ = outR.Close(), outW.Close()
		return Fail, fmt.Sprintf("stderr pipe: %v", err), err
	}
	cmd.Stdout, cmd.Stderr = outW, errW

	if err := cmd.Start(); err != nil {
		_, _ = outR.Close(), outW.Close()
		_, _ = errR.Close(), errW.Close()
		return Fail, fmt.Sprintf("start `daemon`: %v", err), err
	}
	// The child holds its own descriptors now; dropping ours makes the readers
	// see EOF when the daemon exits.
	_, _ = outW.Close(), errW.Close()

	sess := &daemonSession{cmd: cmd, stdout: outR, stderr: errR}
	sess.reap()
	go func() {
		buf, _ := io.ReadAll(errR)
		sess.errMu.Lock()
		sess.errBuf.Write(buf)
		sess.errMu.Unlock()
	}()

	type hsResult struct {
		line string
		hs   Handshake
		err  error
		br   *bufio.Reader
	}
	ch := make(chan hsResult, 1)
	go func() {
		br := bufio.NewReader(outR)
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			ch <- hsResult{err: fmt.Errorf("read handshake line: %w", err), br: br}
			return
		}
		line = strings.TrimSpace(line)
		var hs Handshake
		if err := json.Unmarshal([]byte(line), &hs); err != nil {
			ch <- hsResult{line: line, err: fmt.Errorf("parse handshake: %w", err), br: br}
			return
		}
		ch <- hsResult{line: line, hs: hs, br: br}
	}()

	// The startup budget is the authority on how long a daemon may take to
	// announce itself, and the per-check deadline is that budget plus
	// Suite.Timeout — so this timer always fires first and the failure is always
	// attributed to the budget that actually governs it. A ctx deadline can still
	// arrive first if the *caller* bounded the whole run; that is the run running
	// out of time, not a statement about the daemon, so it is reported as itself.
	budget := r.startupTimeout()
	timer := time.NewTimer(budget)
	defer timer.Stop()

	var got hsResult
	select {
	case <-timer.C:
		sess.close()
		return r.startupTimedOut(budget, sess)
	case <-ctx.Done():
		sess.close()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Fail, withDiagnostics(fmt.Sprintf(
				"the run's own deadline expired while waiting for the handshake; the %s startup budget had not run out",
				budget), sess), ctx.Err()
		}
		return Fail, fmt.Sprintf("cancelled waiting for the handshake: %v", ctx.Err()), ctx.Err()
	case got = <-ch:
	}

	if got.err != nil {
		sess.close()
		detail := got.err.Error()
		if got.line != "" {
			detail = fmt.Sprintf("%s (first stdout line: %q)", detail, clip.Runes(got.line, 200))
		}
		if diag := sess.diagnostics(); diag != "" {
			detail = fmt.Sprintf("%s (%s)", detail, diag)
		}
		return Fail, detail, got.err
	}
	if got.hs.Socket == "" {
		sess.close()
		return Fail, fmt.Sprintf("handshake %q omits \"socket\"", clip.Runes(got.line, 200)), nil
	}
	if !filepath.IsAbs(got.hs.Socket) {
		sess.close()
		return Fail, fmt.Sprintf("handshake socket %q is not an absolute path", got.hs.Socket), nil
	}
	sess.hs = got.hs

	// Drain the rest of stdout as logs, exactly as the host does. This also
	// keeps the pipe from filling and blocking the daemon.
	go func() {
		br := got.br
		if br == nil {
			return
		}
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				sess.logsMu.Lock()
				sess.logs.WriteString(line)
				sess.logsMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	r.daemon = sess
	return Pass, fmt.Sprintf("socket %s (version %q, within %s)",
		sess.hs.Socket, sess.hs.Version, budget), nil
}

// startupTimedOut records and explains a handshake timeout, quoting whatever the
// daemon did manage to say so the author is not left guessing.
//
// It names the *startup budget* rather than reporting an anonymous deadline, and
// wraps [ErrStartupTimeout] so the one Mode-C outcome a slow machine can
// manufacture stays distinguishable from a protocol violation. The flag it sets
// is what lets the dependent checks skip with that same distinction instead of a
// flat "the daemon did not start".
func (r *runner) startupTimedOut(budget time.Duration, sess *daemonSession) (Status, string, error) {
	r.startupBudgetExpired = budget
	detail := withDiagnostics(
		fmt.Sprintf("no handshake line within the %s startup budget", budget), sess)
	return Fail, detail, fmt.Errorf("%w (%s)", ErrStartupTimeout, budget)
}

// withDiagnostics appends whatever the daemon said on either stream to a failure
// detail, so the author is not left guessing.
func withDiagnostics(detail string, sess *daemonSession) string {
	if diag := sess.diagnostics(); diag != "" {
		return fmt.Sprintf("%s (%s)", detail, diag)
	}
	return detail
}

func checkModeCSocketListening(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	info, err := os.Stat(r.daemon.hs.Socket)
	if err != nil {
		return Fail, fmt.Sprintf("announced socket %s: %v", r.daemon.hs.Socket, err), err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return Fail, fmt.Sprintf("announced path %s is not a socket (mode %v)",
			r.daemon.hs.Socket, info.Mode()), nil
	}
	return Pass, r.daemon.hs.Socket + " is a bound socket", nil
}

func checkModeCSocketPrivate(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	info, err := os.Stat(r.daemon.hs.Socket)
	if err != nil {
		return Skip, "the socket is not stattable (see modeC.socket-listening)", nil
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		return Fail, fmt.Sprintf("socket mode %v grants access to group/other; the host dials it with insecure transport on the assumption that it is user-private", perm), nil
	}
	return Pass, fmt.Sprintf("socket mode %v, owner only", perm), nil
}

// dial opens the gRPC client connection the remaining Mode-C checks share.
func (r *runner) dial(ctx context.Context) (*grpc.ClientConn, error) {
	if r.daemon.conn != nil {
		return r.daemon.conn, nil
	}
	conn, err := grpc.NewClient("unix://"+r.daemon.hs.Socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGRPCMsgSize),
			grpc.MaxCallSendMsgSize(maxGRPCMsgSize),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("dial unix://%s: %w", r.daemon.hs.Socket, err)
	}
	// Actively drive the connection to READY, as the host's pool does, so a
	// daemon that bound the socket but never served fails here rather than on
	// the first real RPC.
	conn.Connect()
	for {
		s := conn.GetState()
		if s == connectivity.Ready {
			r.daemon.conn = conn
			return conn, nil
		}
		if s == connectivity.Shutdown {
			_ = conn.Close()
			return nil, fmt.Errorf("connection to %s entered Shutdown", r.daemon.hs.Socket)
		}
		if !conn.WaitForStateChange(ctx, s) {
			_ = conn.Close()
			return nil, fmt.Errorf("connection to %s stalled in %s: %w", r.daemon.hs.Socket, s, ctx.Err())
		}
	}
}

func checkModeCGRPCReady(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	start := time.Now()
	if _, err := r.dial(ctx); err != nil {
		detail := err.Error()
		if diag := r.daemon.diagnostics(); diag != "" {
			detail = fmt.Sprintf("%s (%s)", detail, diag)
		}
		return Fail, detail, err
	}
	return Pass, fmt.Sprintf("READY after %s", time.Since(start).Round(time.Millisecond)), nil
}

// isUnimplemented reports whether err is a gRPC Unimplemented status — the
// signature of a service (or method) the daemon never registered.
func isUnimplemented(err error) bool {
	return status.Code(err) == codes.Unimplemented
}

func checkModeCBridgeService(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	conn, err := r.dial(ctx)
	if err != nil {
		return Skip, "the connection is not READY (see modeC.grpc-ready)", nil
	}

	// Open a Process stream and close the send side immediately. A daemon with
	// BridgeService registered answers with an error, a response, or a clean
	// EOF; one without it answers Unimplemented. Sending no header means no
	// document is opened, so this cannot have side effects.
	stream, err := pb.NewBridgeServiceClient(conn).Process(ctx)
	if err != nil {
		if isUnimplemented(err) {
			return Fail, "BridgeService is not registered on the socket (Process → Unimplemented)", err
		}
		return Fail, fmt.Sprintf("opening the Process stream failed: %v", err), err
	}
	_ = stream.CloseSend()
	_, recvErr := stream.Recv()
	if recvErr != nil && isUnimplemented(recvErr) {
		return Fail, "BridgeService is not registered on the socket (Process → Unimplemented)", recvErr
	}
	switch {
	case recvErr == nil:
		return Pass, "BridgeService registered (Process accepted a stream)", nil
	case errors.Is(recvErr, io.EOF):
		return Pass, "BridgeService registered (Process closed the stream cleanly on an empty request)", nil
	default:
		return Pass, fmt.Sprintf("BridgeService registered (Process rejected an empty request with %s)",
			status.Code(recvErr)), nil
	}
}

func checkModeCFormatProbe(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	probe := r.suite.FormatProbe
	if probe == nil {
		return Skip, "no Suite.FormatProbe configured: the suite never reads a document through a declared format on its own", nil
	}
	if !declaresFormat(r, probe.Format) {
		return Fail, fmt.Sprintf("probe names format %q, which the manifest does not declare", probe.Format), nil
	}
	conn, err := r.dial(ctx)
	if err != nil {
		return Skip, "the connection is not READY (see modeC.grpc-ready)", nil
	}

	// Stage the input on disk: the protocol prefers a path over inline bytes so
	// the plugin can resolve companion files relative to the document.
	name := probe.Filename
	if name == "" {
		name = "conformance-probe.txt"
	}
	dir, err := os.MkdirTemp("", "kapi-conformance-*")
	if err != nil {
		return Fail, fmt.Sprintf("stage probe input: %v", err), err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	inPath := filepath.Join(dir, filepath.Base(name))
	if err := os.WriteFile(inPath, probe.Input, 0o600); err != nil {
		return Fail, fmt.Sprintf("write probe input: %v", err), err
	}

	sourceLocale := probe.SourceLocale
	if sourceLocale == "" {
		sourceLocale = "en"
	}
	minBlocks := max(probe.MinBlocks, 1)

	stream, err := pb.NewBridgeServiceClient(conn).Process(ctx)
	if err != nil {
		return Fail, fmt.Sprintf("open Process stream: %v", err), err
	}
	header := &pb.ProcessHeader{
		FilterClass:  probe.Format,
		SourceLocale: sourceLocale,
		Input:        &pb.ContentRef{Location: &pb.ContentRef_Path{Path: inPath}},
	}
	if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
		return Fail, fmt.Sprintf("send process header: %v", err), err
	}
	// Read-only: no output ref and no processed parts to send back.
	_ = stream.CloseSend()

	blocks := 0
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			detail := fmt.Sprintf("reading %q via format %q failed after %d block(s): %v",
				name, probe.Format, blocks, err)
			if diag := r.daemon.diagnostics(); diag != "" {
				detail = fmt.Sprintf("%s (%s)", detail, diag)
			}
			return Fail, detail, err
		}
		blocks += countBlocks(resp)
		if _, done := resp.Response.(*pb.ProcessResponse_Complete); done {
			break
		}
	}
	if blocks < minBlocks {
		return Fail, fmt.Sprintf("format %q returned %d translatable block(s), want at least %d",
			probe.Format, blocks, minBlocks), nil
	}
	return Pass, fmt.Sprintf("format %q read %d translatable block(s) from %s", probe.Format, blocks, name), nil
}

// countBlocks counts translatable blocks in one Process response, across the
// single-part, batched-part, and lightweight content-block encodings the
// protocol allows a daemon to choose between.
func countBlocks(resp *pb.ProcessResponse) int {
	switch v := resp.Response.(type) {
	case *pb.ProcessResponse_ContentBatch:
		return len(v.ContentBatch.GetBlocks())
	case *pb.ProcessResponse_PartBatch:
		n := 0
		for _, p := range v.PartBatch.GetParts() {
			if p.GetBlock() != nil {
				n++
			}
		}
		return n
	case *pb.ProcessResponse_Part:
		if v.Part.GetBlock() != nil {
			return 1
		}
	}
	return 0
}

func declaresFormat(r *runner, name string) bool {
	for _, f := range r.man.Capabilities.Formats {
		if f.Name == name {
			return true
		}
	}
	return false
}

func checkModeCSegmentRPC(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	if len(r.man.Capabilities.Segmenters) == 0 {
		return Skip, "manifest declares no segmenters", nil
	}
	conn, err := r.dial(ctx)
	if err != nil {
		return Skip, "the connection is not READY (see modeC.grpc-ready)", nil
	}
	client := pb.NewBridgeServiceClient(conn)

	probe := r.suite.SegmentProbe
	engine := r.man.Capabilities.Segmenters[0].Name
	text := "One sentence. And a second one."
	locale := "en"
	minBoundaries := 0 // Reachability only, unless a probe asks for more.
	var params map[string]string

	if probe != nil {
		if !declaresSegmenter(r, probe.Engine) {
			return Fail, fmt.Sprintf("probe names segmenter %q, which the manifest does not declare", probe.Engine), nil
		}
		engine, params = probe.Engine, probe.Params
		if probe.Text != "" {
			text = probe.Text
		}
		if probe.Locale != "" {
			locale = probe.Locale
		}
		minBoundaries = max(probe.MinBoundaries, 1)
	}

	resp, err := client.Segment(ctx, &pb.SegmentRequest{
		Engine: engine, Text: text, Locale: locale, Params: params,
	})
	if err != nil {
		if isUnimplemented(err) {
			return Fail, fmt.Sprintf("segmenter %q is declared but the Segment RPC is not implemented", engine), err
		}
		detail := fmt.Sprintf("Segment(%q) failed: %v", engine, err)
		if diag := r.daemon.diagnostics(); diag != "" {
			detail = fmt.Sprintf("%s (%s)", detail, diag)
		}
		return Fail, detail, err
	}
	if e := resp.GetError(); e != "" {
		// Without a probe the suite supplies its own text, so an engine that
		// rejects it is reporting on the sample, not on its own health. The
		// RPC being reachable is all this check claims in that case.
		if probe == nil {
			return Pass, fmt.Sprintf("Segment(%q) is reachable; the engine declined the suite's sample text (%s)",
				engine, clip.Runes(e, 160)), nil
		}
		return Fail, fmt.Sprintf("Segment(%q) returned error: %s", engine, clip.Runes(e, 200)), nil
	}
	got := len(resp.GetBoundaries())
	if got < minBoundaries {
		return Fail, fmt.Sprintf("Segment(%q) returned %d boundaries, want at least %d",
			engine, got, minBoundaries), nil
	}
	return Pass, fmt.Sprintf("Segment(%q) returned %d boundary/boundaries", engine, got), nil
}

func declaresSegmenter(r *runner, name string) bool {
	for _, s := range r.man.Capabilities.Segmenters {
		if s.Name == name {
			return true
		}
	}
	return false
}

// shutdownGrace bounds how long the daemon may take to exit after Shutdown or
// SIGTERM. Like the startup budget it is independent of Suite.Timeout, so a
// caller can shorten the teardown wait — the one wait a well-behaved plugin
// never uses in full — without touching how patiently the suite waits for a
// daemon to start.
func (r *runner) shutdownGrace() time.Duration { return r.suite.shutdownGrace() }

func checkModeCShutdownRPC(ctx context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	conn, err := r.dial(ctx)
	if err != nil {
		return Skip, "the connection is not READY (see modeC.grpc-ready)", nil
	}
	if _, err := pb.NewBridgeServiceClient(conn).Shutdown(ctx, &pb.ShutdownRequest{}); err != nil {
		// A daemon that shuts down while answering legitimately drops the
		// call; only Unimplemented means the method is genuinely absent.
		if isUnimplemented(err) {
			return Fail, "Shutdown is not implemented; the host must fall back to SIGTERM", err
		}
		if code := status.Code(err); code != codes.Unavailable && code != codes.Canceled {
			return Fail, fmt.Sprintf("Shutdown returned %s: %v", code, err), err
		}
	}
	grace := r.shutdownGrace()
	start := time.Now()
	if !r.daemon.waitExit(grace) {
		return Fail, fmt.Sprintf("still running %s after Shutdown returned", grace), nil
	}
	return Pass, fmt.Sprintf("exited %s after the Shutdown RPC (budget %s)",
		time.Since(start).Round(time.Millisecond), grace), nil
}

func checkModeCSigtermExit(_ context.Context, r *runner) (Status, string, error) {
	if st, d, err := r.requireDaemon(); st != "" {
		return st, d, err
	}
	grace := r.shutdownGrace()

	// A daemon that implements Shutdown has usually already exited by now;
	// that satisfies the same requirement (the host got its process back).
	if r.daemon.hasExited() {
		leaked := socketStillPresent(r.daemon.hs.Socket)
		if leaked {
			return Fail, "the daemon exited but left its socket file behind", nil
		}
		return Pass, "already exited via the Shutdown RPC, socket released", nil
	}
	if r.daemon.cmd.Process == nil {
		return Fail, "no daemon process to signal", nil
	}
	start := time.Now()
	if err := r.daemon.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return Fail, fmt.Sprintf("send SIGTERM: %v", err), err
	}
	if !r.daemon.waitExit(grace) {
		return Fail, fmt.Sprintf("still running %s after SIGTERM", grace), nil
	}
	if socketStillPresent(r.daemon.hs.Socket) {
		return Fail, fmt.Sprintf("exited on SIGTERM but left %s behind", r.daemon.hs.Socket), nil
	}
	return Pass, fmt.Sprintf("exited %s after SIGTERM and released its socket (budget %s)",
		time.Since(start).Round(time.Millisecond), grace), nil
}

// socketStillPresent reports whether the socket file survived the daemon.
// A stale socket blocks the next bind at the same path.
func socketStillPresent(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
