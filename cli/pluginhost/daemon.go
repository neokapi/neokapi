package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// envMaxDaemons is the environment variable that caps the daemon pool.
const envMaxDaemons = "KAPI_MAX_DAEMONS"

// envExternalSocketPrefix is prepended to a normalised plugin name to
// pick up a pre-started daemon's Unix socket. e.g. for plugin
// "okapi-bridge" the env var is `KAPI_DAEMON_SOCKET_OKAPI_BRIDGE`.
const envExternalSocketPrefix = "KAPI_DAEMON_SOCKET_"

// externalDaemonSocket returns the Unix socket path of a pre-started
// daemon for `pluginName`, or "" when the env var is unset. Used by
// pseudobench to attach to a long-lived daemon and skip JVM startup
// on every kapi invocation.
func externalDaemonSocket(pluginName string) string {
	key := envExternalSocketPrefix + strings.ToUpper(strings.ReplaceAll(pluginName, "-", "_"))
	return os.Getenv(key)
}

// defaultMaxDaemons is the default cap when KAPI_MAX_DAEMONS is unset or
// invalid.
const defaultMaxDaemons = 8

// defaultStartupTimeout is used when manifest.daemon.startup_timeout_seconds
// is zero.
const defaultStartupTimeout = 30 * time.Second

// defaultIdleTimeout is used when manifest.daemon.idle_timeout_seconds is
// zero.
const defaultIdleTimeout = 5 * time.Minute

// defaultShutdownGrace is the time we wait for a daemon to exit after
// SIGTERM before SIGKILL.
const defaultShutdownGrace = 3 * time.Second

// Handshake is the JSON envelope a Mode-C daemon prints as its first
// stdout line. The daemon must keep stdout open afterwards (subsequent
// lines are forwarded as logs).
type Handshake struct {
	// Socket is the absolute path to the Unix socket the daemon binds to.
	Socket string `json:"socket"`

	// Version is the daemon's reported version (free-form).
	Version string `json:"version,omitempty"`

	// PID is informational (the daemon's PID, may differ from cmd.Process
	// in unusual setups).
	PID int `json:"pid,omitempty"`
}

// DaemonClient is a live connection to one Mode-C plugin daemon.
//
// The client wraps:
//   - the spawned subprocess (Cmd, Process)
//   - the gRPC client connection to its socket
//   - bookkeeping for the pool's idle/LRU policy
//
// Concurrent RPCs over Conn are safe (gRPC ClientConn is thread-safe).
// The pool guarantees Acquire returns either a fresh, healthy client or
// an error.
type DaemonClient struct {
	Plugin *Plugin

	// Conn is the gRPC client connection to the daemon's Unix socket.
	// Always non-nil when Acquire returns successfully.
	Conn *grpc.ClientConn

	// Socket is the absolute path the daemon bound to.
	Socket string

	// Version is the version string the daemon announced in its handshake.
	Version string

	// pool is the owning pool (for release/eviction bookkeeping).
	pool *DaemonPool

	// cmd is the daemon subprocess.
	cmd *exec.Cmd

	// startedAt is the wall-clock time the daemon was spawned.
	startedAt time.Time

	// mu guards lastUsed and closed.
	mu sync.Mutex

	// lastUsed is updated every time the pool hands this client out.
	lastUsed time.Time

	// closed reports whether the client has been torn down.
	closed bool

	// stopCh is closed when the watcher goroutine should exit.
	stopCh chan struct{}

	// drainDone is closed by the stdout-drain goroutine when it stops
	// reading (EOF or error). close() waits on it before cmd.Wait() so the
	// drain never reads from the stdout pipe concurrently with Wait — a
	// pattern os/exec documents as unsafe (Wait may close the pipe out from
	// under the reader). Nil when there is no drain goroutine (external
	// attach mode, where cmd is nil).
	drainDone chan struct{}
}

// touch refreshes the client's lastUsed timestamp. Callers (the pool's
// Acquire path) hold this open while the client is in use; for the LRU
// we only need approximate accuracy.
func (c *DaemonClient) touch() {
	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()
}

// LastUsed reports when the client was last handed out.
func (c *DaemonClient) LastUsed() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastUsed
}

// IsClosed reports whether the client has been torn down. After Close,
// the gRPC Conn is closed and the daemon process has been signalled.
func (c *DaemonClient) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// PID returns the daemon process PID, or 0 if not running.
func (c *DaemonClient) PID() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// close tears down one daemon: close gRPC conn, send SIGTERM, wait
// briefly, then SIGKILL. Idempotent.
func (c *DaemonClient) close(grace time.Duration) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if c.stopCh != nil {
		select {
		case <-c.stopCh:
			// already closed
		default:
			close(c.stopCh)
		}
	}
	c.mu.Unlock()

	if c.Conn != nil {
		_ = c.Conn.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		// Best-effort SIGTERM + grace + SIGKILL.
		_ = c.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			// Join the stdout-drain goroutine before Wait: os/exec
			// warns it is unsafe for another goroutine to read the
			// StdoutPipe while Wait runs (Wait closes the pipe). After
			// SIGTERM the daemon's stdout closes, so the drain hits EOF
			// and signals drainDone promptly. If the daemon ignores
			// SIGTERM the grace timer below escalates to Kill, which
			// also closes stdout and unblocks the drain.
			if c.drainDone != nil {
				<-c.drainDone
			}
			_ = c.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(grace):
			_ = c.cmd.Process.Kill()
			<-done
		}
	}
	// Best-effort socket cleanup — only if we spawned the daemon
	// ourselves. External attach mode (cmd == nil) leaves the socket
	// alone; the harness that started the daemon owns it.
	if c.Socket != "" && c.cmd != nil {
		_ = os.Remove(c.Socket)
	}
}

// DaemonPoolOptions configures the pool.
type DaemonPoolOptions struct {
	// MaxDaemons caps the number of concurrent daemons. When zero, the
	// pool reads $KAPI_MAX_DAEMONS, falling back to defaultMaxDaemons (8).
	MaxDaemons int

	// StartupTimeout overrides the manifest's startup_timeout_seconds.
	// Zero defers to the manifest, then defaultStartupTimeout.
	StartupTimeout time.Duration

	// IdleTimeout overrides the manifest's idle_timeout_seconds. Zero
	// defers to the manifest, then defaultIdleTimeout.
	IdleTimeout time.Duration

	// ShutdownGrace is the time the pool waits for a daemon to exit
	// after SIGTERM before SIGKILL. Zero uses defaultShutdownGrace.
	ShutdownGrace time.Duration

	// SocketDir is the directory where daemon sockets live (informational
	// only — the daemon picks the actual path). Empty uses os.TempDir().
	SocketDir string

	// Logger receives one-line debug events ("spawning daemon", "evicting
	// daemon", "daemon exited"). Nil discards them.
	Logger func(format string, args ...any)
}

// DaemonPool spawns and reuses Mode-C daemons. It enforces a configurable
// cap (KAPI_MAX_DAEMONS, default 8) using LRU eviction.
//
// One pool is owned by the kapi process; it shuts every daemon down on
// Shutdown(). Acquire is safe under concurrent use.
type DaemonPool struct {
	opts DaemonPoolOptions

	mu         sync.Mutex
	clients    map[string]*DaemonClient // key: plugin name
	spawnLocks map[string]*sync.Mutex   // per-plugin spawn coordination — prevents concurrent first-callers from each spawning a JVM only to kill all but one
	closed     bool
}

// NewDaemonPool builds an empty pool. Daemons are spawned lazily in
// Acquire.
func NewDaemonPool(opts DaemonPoolOptions) *DaemonPool {
	if opts.MaxDaemons <= 0 {
		opts.MaxDaemons = resolveMaxDaemons()
	}
	if opts.ShutdownGrace <= 0 {
		opts.ShutdownGrace = defaultShutdownGrace
	}
	if opts.Logger == nil {
		opts.Logger = func(string, ...any) {}
	}
	return &DaemonPool{
		opts:       opts,
		clients:    map[string]*DaemonClient{},
		spawnLocks: map[string]*sync.Mutex{},
	}
}

// MaxDaemons returns the effective cap.
func (p *DaemonPool) MaxDaemons() int { return p.opts.MaxDaemons }

// resolveMaxDaemons honours $KAPI_MAX_DAEMONS, falling back to the
// default. Invalid values silently fall back too.
func resolveMaxDaemons() int {
	if v := os.Getenv(envMaxDaemons); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxDaemons
}

// Acquire returns a healthy DaemonClient for the given plugin. If a
// daemon is already running and healthy, Acquire reuses it. Otherwise
// the pool spawns a fresh daemon, evicting the LRU entry first if the
// cap is reached.
//
// The returned client must be used while the pool is alive; the pool
// owns the lifetime. Callers do NOT close the client themselves.
//
// Concurrent first-callers for the same plugin coordinate via a
// per-plugin spawn lock so only one JVM is started — the rest wait
// and reuse it. Without this, N concurrent lanes would each spawn a
// JVM, then kill N-1 of them right after handshake, wasting startup
// time and flooding logs.
func (p *DaemonPool) Acquire(ctx context.Context, plugin *Plugin) (*DaemonClient, error) {
	if plugin == nil {
		return nil, errors.New("daemon pool: plugin is nil")
	}
	if !plugin.Manifest.IsModeC() {
		return nil, fmt.Errorf("daemon pool: plugin %q is not Mode-C (no formats/tools/source_connectors)", plugin.Name())
	}

	// Fast path: cached, healthy client.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("daemon pool: closed")
	}
	if existing, ok := p.clients[plugin.Name()]; ok && !existing.IsClosed() && existing.Conn != nil {
		existing.touch()
		p.mu.Unlock()
		return existing, nil
	}
	p.mu.Unlock()

	// Slow path: serialize spawn-per-plugin so only one goroutine runs
	// the JVM start. Other concurrent callers block here, then hit the
	// re-check below and reuse the freshly spawned client.
	spawnLock := p.spawnLockFor(plugin.Name())
	spawnLock.Lock()
	defer spawnLock.Unlock()

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("daemon pool: closed")
	}
	// Re-check cache after acquiring spawn lock — we may have been the
	// loser in a race and the winner already populated p.clients.
	if existing, ok := p.clients[plugin.Name()]; ok && !existing.IsClosed() && existing.Conn != nil {
		existing.touch()
		p.mu.Unlock()
		return existing, nil
	}
	// Drop a stale entry if present (process crashed, conn closed).
	if stale, ok := p.clients[plugin.Name()]; ok {
		delete(p.clients, plugin.Name())
		go stale.close(p.opts.ShutdownGrace)
	}
	// Evict LRU if we'd exceed the cap.
	for len(p.clients) >= p.opts.MaxDaemons {
		victim := p.lruLocked()
		if victim == nil {
			break
		}
		p.opts.Logger("daemon pool: evicting LRU daemon %q", victim.Plugin.Name())
		delete(p.clients, victim.Plugin.Name())
		go victim.close(p.opts.ShutdownGrace)
	}
	p.mu.Unlock()

	// Spawn outside both locks. spawnLock still held, so no concurrent
	// spawn for this plugin.
	client, err := p.spawn(ctx, plugin)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		client.close(p.opts.ShutdownGrace)
		return nil, errors.New("daemon pool: closed")
	}
	p.clients[plugin.Name()] = client
	client.touch()
	p.mu.Unlock()

	// Start idle watcher (if applicable).
	idle := p.idleTimeoutFor(plugin)
	if idle > 0 {
		go p.watchIdle(client, idle)
	}

	return client, nil
}

// spawnLockFor returns the spawn-coordination mutex for a plugin,
// creating one on first use. Per-plugin (not per-pool) so spawns for
// different plugins remain parallel.
func (p *DaemonPool) spawnLockFor(name string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l, ok := p.spawnLocks[name]; ok {
		return l
	}
	l := &sync.Mutex{}
	p.spawnLocks[name] = l
	return l
}

// lruLocked returns the least-recently-used client. p.mu must be held.
func (p *DaemonPool) lruLocked() *DaemonClient {
	var oldest *DaemonClient
	for _, c := range p.clients {
		if oldest == nil || c.LastUsed().Before(oldest.LastUsed()) {
			oldest = c
		}
	}
	return oldest
}

// idleTimeoutFor resolves the idle timeout for a plugin: explicit option
// > manifest > default.
func (p *DaemonPool) idleTimeoutFor(plugin *Plugin) time.Duration {
	if p.opts.IdleTimeout > 0 {
		return p.opts.IdleTimeout
	}
	if plugin.Manifest.Daemon != nil && plugin.Manifest.Daemon.IdleTimeoutSeconds > 0 {
		return time.Duration(plugin.Manifest.Daemon.IdleTimeoutSeconds) * time.Second
	}
	return defaultIdleTimeout
}

// startupTimeoutFor resolves the startup timeout for a plugin: explicit
// option > manifest > default.
func (p *DaemonPool) startupTimeoutFor(plugin *Plugin) time.Duration {
	if p.opts.StartupTimeout > 0 {
		return p.opts.StartupTimeout
	}
	if plugin.Manifest.Daemon != nil && plugin.Manifest.Daemon.StartupTimeoutSeconds > 0 {
		return time.Duration(plugin.Manifest.Daemon.StartupTimeoutSeconds) * time.Second
	}
	return defaultStartupTimeout
}

// watchIdle terminates the daemon if it sits idle longer than `idle`.
func (p *DaemonPool) watchIdle(client *DaemonClient, idle time.Duration) {
	tick := time.NewTicker(idle / 2)
	defer tick.Stop()
	for {
		select {
		case <-client.stopCh:
			return
		case now := <-tick.C:
			if now.Sub(client.LastUsed()) < idle {
				continue
			}
			// Re-check LastUsed under p.mu before tearing the daemon
			// down. Acquire touches a reused client while holding p.mu
			// (see the fast/slow-path touch()es), so a check outside the
			// lock is a TOCTOU window: a concurrent Acquire could hand
			// this client out between our LastUsed read and close(),
			// leaving the caller with a torn-down daemon. Holding p.mu
			// across the final LastUsed read serialises us against that
			// touch, so a recently-used daemon is never closed.
			p.mu.Lock()
			existing, ok := p.clients[client.Plugin.Name()]
			if !ok || existing != client {
				// Already evicted/replaced — nothing to do.
				p.mu.Unlock()
				return
			}
			if time.Since(client.LastUsed()) < idle {
				// A concurrent Acquire touched it; keep watching.
				p.mu.Unlock()
				continue
			}
			delete(p.clients, client.Plugin.Name())
			p.opts.Logger("daemon pool: %q idle for %s — terminating", client.Plugin.Name(), time.Since(client.LastUsed()))
			p.mu.Unlock()
			client.close(p.opts.ShutdownGrace)
			return
		}
	}
}

// Shutdown closes every daemon in the pool and prevents new acquires.
// Safe to call multiple times.
func (p *DaemonPool) Shutdown() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	clients := make([]*DaemonClient, 0, len(p.clients))
	for _, c := range p.clients {
		clients = append(clients, c)
	}
	p.clients = map[string]*DaemonClient{}
	p.mu.Unlock()

	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c *DaemonClient) {
			defer wg.Done()
			c.close(p.opts.ShutdownGrace)
		}(c)
	}
	wg.Wait()
}

// Active returns the names of currently-running daemons. Mainly useful
// for tests and `kapi plugin list --running`.
func (p *DaemonPool) Active() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.clients))
	for name := range p.clients {
		out = append(out, name)
	}
	return out
}

// spawn launches one daemon subprocess, parses its stdout handshake,
// and dials its Unix socket over gRPC.
func (p *DaemonPool) spawn(ctx context.Context, plugin *Plugin) (*DaemonClient, error) {
	if runtime.GOOS == "windows" {
		// Mode C uses Unix sockets. On Windows we'd swap in named pipes
		// or 127.0.0.1 + dynamic port; not yet implemented.
		return nil, errors.New("daemon pool: Mode C is not yet supported on Windows")
	}

	startupTimeout := p.startupTimeoutFor(plugin)

	// External attach: when KAPI_DAEMON_SOCKET_<PLUGIN> points at a Unix
	// socket, skip exec entirely and dial that socket. Lets pseudobench
	// (and other harnesses) measure a long-lived daemon's per-call cost
	// without paying JVM startup on every kapi invocation. The DaemonClient
	// is built with cmd=nil; close() already no-ops the kill path when
	// cmd is nil, and we leave the socket in place — caller owns its
	// lifecycle.
	if attachSocket := externalDaemonSocket(plugin.Name()); attachSocket != "" {
		dialCtx, dialCancel := context.WithTimeout(ctx, startupTimeout)
		defer dialCancel()
		conn, err := dialUnixSocket(dialCtx, attachSocket)
		if err != nil {
			return nil, fmt.Errorf("daemon pool: dial external socket %s: %w", attachSocket, err)
		}
		client := &DaemonClient{
			Plugin:    plugin,
			Conn:      conn,
			Socket:    attachSocket,
			Version:   "external",
			pool:      p,
			cmd:       nil,
			startedAt: time.Now(),
			stopCh:    make(chan struct{}),
		}
		client.touch()
		p.opts.Logger("daemon pool: attached %q at external socket %s", plugin.Name(), attachSocket)
		return client, nil
	}
	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	// IMPORTANT: do NOT use exec.CommandContext here. CommandContext
	// kills the subprocess when the context expires, which is exactly
	// the wrong behavior for a long-lived daemon: startCtx fires
	// shortly after we read the handshake (defer cancel() at function
	// exit), and we need the daemon to keep running until the pool's
	// own Shutdown path tears it down.
	cmd := exec.Command(plugin.BinaryPath, "daemon") //nolint:noctx // long-lived daemon subprocess
	cmd.Env = append(os.Environ(),
		"KAPI_PLUGIN_DIR="+plugin.Dir,
		"KAPI_PLUGIN_NAME="+plugin.Name(),
		"KAPI_PLUGIN_VERSION="+plugin.Version(),
	)
	// Host-owned model assets: stage the plugin's declared default model (the
	// host downloads + verifies it, with a progress bar) before the daemon
	// starts, and pass the model-cache locations via env. No-op for plugins
	// that declare no models.
	modelEnv, err := ensureModelEnv(plugin)
	if err != nil {
		return nil, fmt.Errorf("daemon pool: ensure models for %q: %w", plugin.Name(), err)
	}
	for k, v := range modelEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("daemon pool: stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("daemon pool: start %s: %w", plugin.BinaryPath, err)
	}

	// Read the first stdout line as the handshake. We must do this with
	// a deadline; the daemon promises to print the line within
	// startupTimeout.
	type handshakeResult struct {
		hs  Handshake
		err error
		br  *bufio.Reader
	}
	hsCh := make(chan handshakeResult, 1)
	go func() {
		br := bufio.NewReader(stdout)
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			hsCh <- handshakeResult{err: fmt.Errorf("read handshake: %w", err), br: br}
			return
		}
		var hs Handshake
		line = strings.TrimSpace(line)
		if err := json.Unmarshal([]byte(line), &hs); err != nil {
			hsCh <- handshakeResult{err: fmt.Errorf("parse handshake %q: %w", line, err), br: br}
			return
		}
		hsCh <- handshakeResult{hs: hs, br: br}
	}()

	var hs Handshake
	var br *bufio.Reader
	select {
	case <-startCtx.Done():
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("daemon pool: handshake timeout after %s for %q", startupTimeout, plugin.Name())
	case res := <-hsCh:
		if res.err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("daemon pool: %w (plugin %q)", res.err, plugin.Name())
		}
		hs = res.hs
		br = res.br
	}

	if hs.Socket == "" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("daemon pool: handshake from %q missing socket", plugin.Name())
	}

	// After handshake, drain remaining stdout to stderr (treat as logs).
	// drainDone is closed when the goroutine stops reading; close() joins
	// on it before cmd.Wait() so the drain never races Wait on the pipe.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		// Use the bufio.Reader started by the goroutine above so we
		// don't lose buffered bytes.
		if br == nil {
			return
		}
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				fmt.Fprintf(os.Stderr, "[%s] %s", plugin.Name(), line)
			}
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
					p.opts.Logger("daemon %q stdout: %v", plugin.Name(), err)
				}
				return
			}
		}
	}()

	// Dial the announced Unix socket.
	dialCtx, dialCancel := context.WithTimeout(ctx, startupTimeout)
	defer dialCancel()
	conn, err := dialUnixSocket(dialCtx, hs.Socket)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("daemon pool: dial %s: %w", hs.Socket, err)
	}

	client := &DaemonClient{
		Plugin:    plugin,
		Conn:      conn,
		Socket:    hs.Socket,
		Version:   hs.Version,
		pool:      p,
		cmd:       cmd,
		startedAt: time.Now(),
		stopCh:    make(chan struct{}),
		drainDone: drainDone,
	}
	client.touch()

	p.opts.Logger("daemon pool: spawned %q at %s (pid %d)", plugin.Name(), hs.Socket, cmd.Process.Pid)
	return client, nil
}

// maxGRPCMsgSize is the per-message limit for the daemon gRPC connection.
// The default gRPC cap is 4 MB; bridge plugins stream whole documents inline
// (ContentRef_Inline), so we raise the limit to 256 MB per call to avoid
// ResourceExhausted errors on large files.
const maxGRPCMsgSize = 256 << 20 // 256 MB

// dialUnixSocket opens a gRPC client connection over a Unix socket.
// Insecure transport is fine here — the socket sits under $TMPDIR with
// 0600 mode, owned by the same user.
func dialUnixSocket(ctx context.Context, socket string) (*grpc.ClientConn, error) {
	target := "unix://" + socket
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGRPCMsgSize),
			grpc.MaxCallSendMsgSize(maxGRPCMsgSize),
		),
	)
	if err != nil {
		return nil, err
	}
	// Optional: actively probe by waiting until READY or the context expires.
	// gRPC will lazily connect on the first RPC otherwise; we want to fail
	// fast if the daemon isn't actually serving.
	if err := waitReady(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// waitReady blocks until the connection enters READY state or ctx fires.
// TRANSIENT_FAILURE is treated as a hard stop since the daemon is unlikely
// to recover on its own at startup time.
func waitReady(ctx context.Context, conn *grpc.ClientConn) error {
	conn.Connect()
	for {
		s := conn.GetState()
		switch s {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return errors.New("connection shut down before ready")
		case connectivity.TransientFailure:
			return errors.New("connection entered transient failure before ready")
		}
		if !conn.WaitForStateChange(ctx, s) {
			return ctx.Err()
		}
	}
}
