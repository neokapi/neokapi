// Mode-C daemon entry point for the bowrain plugin (#438 phase 7).
//
// kapi spawns this binary as `kapi-bowrain daemon` and communicates with
// it over a Unix socket using gRPC. The daemon implements:
//
//   - SourceConnectorService — push/pull/status/list operations on a
//     bowrain-tracked .kapi project.
//   - DaemonControlService — Health and Shutdown lifecycle RPCs.
//
// Daemon lifecycle:
//
//  1. Bind a Unix socket at $TMPDIR/kapi-bowrain-<PID>.sock with mode 0600.
//  2. Print one JSON line on stdout: {"socket":"…","version":"…","pid":N}.
//  3. Serve gRPC on the socket; remain alive across multiple RPCs.
//  4. Exit on Shutdown RPC, SIGTERM/SIGINT, or idle timeout.
//
// The daemon caches *Project instances by absolute root path so repeated
// RPCs against the same project reuse the loaded recipe and sync cache.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	bowrainconn "github.com/neokapi/neokapi/bowrain/core/connector"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v1"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// daemonIdleTimeout is the wall-clock idle window before the daemon
// terminates itself. Mirrors what the kapi-side pool reads from the
// manifest's daemon.idle_timeout_seconds. Kept slightly higher than the
// pool's idle so the pool is the canonical lifecycle owner.
const daemonIdleTimeout = 10 * time.Minute

func buildDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "daemon",
		Short:         "Run as a Mode-C daemon (long-lived, gRPC over local socket)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runDaemon,
	}
}

func runDaemon(cmd *cobra.Command, args []string) error {
	socketPath := defaultSocketPath()
	// Best-effort cleanup of any stale socket file.
	_ = os.Remove(socketPath)

	var lc net.ListenConfig
	lis, err := lc.Listen(cmd.Context(), "unix", socketPath)
	if err != nil {
		return fmt.Errorf("daemon: bind %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		// Non-fatal; the socket is already bound. Log via stderr.
		fmt.Fprintf(os.Stderr, "daemon: chmod %s: %v\n", socketPath, err)
	}

	server := grpc.NewServer()
	d := newDaemonService(server)
	pb.RegisterSourceConnectorServiceServer(server, d)
	pb.RegisterDaemonControlServiceServer(server, d)

	// Print handshake (FIRST stdout line). The kapi-side pool reads
	// exactly one line and parses it as JSON.
	hs := map[string]any{
		"socket":  socketPath,
		"version": pluginVersion,
		"pid":     os.Getpid(),
	}
	enc, _ := json.Marshal(hs)
	fmt.Println(string(enc))

	// Subsequent stdout lines are forwarded to kapi's stderr as logs.
	fmt.Println("daemon: ready")

	// Shutdown coordination: SIGTERM/SIGINT, RPC, or idle timeout.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(lis)
	}()

	idleTicker := time.NewTicker(daemonIdleTimeout / 2)
	defer idleTicker.Stop()

	for {
		select {
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "daemon: signal %s — shutting down\n", sig)
			d.gracefulStop(server)
			_ = os.Remove(socketPath)
			return nil
		case err := <-serveDone:
			_ = os.Remove(socketPath)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				return fmt.Errorf("daemon: serve: %w", err)
			}
			return nil
		case <-d.shutdownCh:
			fmt.Fprintf(os.Stderr, "daemon: Shutdown RPC — exiting\n")
			d.gracefulStop(server)
			_ = os.Remove(socketPath)
			return nil
		case now := <-idleTicker.C:
			if d.idleSince(now) >= daemonIdleTimeout {
				fmt.Fprintf(os.Stderr, "daemon: idle for %s — exiting\n", daemonIdleTimeout)
				d.gracefulStop(server)
				_ = os.Remove(socketPath)
				return nil
			}
		}
	}
}

// defaultSocketPath returns "$TMPDIR/kapi-bowrain-<pid>.sock".
func defaultSocketPath() string {
	dir := os.TempDir()
	return filepath.Join(dir, fmt.Sprintf("kapi-bowrain-%d.sock", os.Getpid()))
}

// daemonService implements SourceConnectorService and DaemonControlService.
//
// One process serves many projects; we cache *Project instances by their
// absolute root so push/pull on the same project re-uses the loaded
// recipe and sync cache.
type daemonService struct {
	pb.UnimplementedSourceConnectorServiceServer
	pb.UnimplementedDaemonControlServiceServer

	startedAt time.Time

	// shutdownCh is closed when a Shutdown RPC arrives.
	shutdownCh chan struct{}
	shutdownMu sync.Once

	mu        sync.Mutex
	projects  map[string]*projectEntry
	lastUsed  time.Time
	formatReg *registry.FormatRegistry // shared across projects, lazily built
}

type projectEntry struct {
	project   *bproject.Project
	connector *connector.BowrainSourceConnector
	formatReg *registry.FormatRegistry
}

func newDaemonService(_ *grpc.Server) *daemonService {
	return &daemonService{
		startedAt:  time.Now(),
		shutdownCh: make(chan struct{}),
		projects:   map[string]*projectEntry{},
		lastUsed:   time.Now(),
		formatReg:  buildFormatRegistry(),
	}
}

// buildFormatRegistry constructs a fresh FormatRegistry populated with
// every built-in format. Reuses cli.App.InitRegistries() so the daemon
// sees the same readers/writers as the host kapi process.
func buildFormatRegistry() *registry.FormatRegistry {
	a := &cli.App{}
	a.InitRegistries()
	return a.FormatReg
}

func (d *daemonService) idleSince(now time.Time) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	return now.Sub(d.lastUsed)
}

func (d *daemonService) touch() {
	d.mu.Lock()
	d.lastUsed = time.Now()
	d.mu.Unlock()
}

func (d *daemonService) gracefulStop(server *grpc.Server) {
	// Close all loaded projects (saves sync caches).
	d.mu.Lock()
	for _, p := range d.projects {
		if p.connector != nil {
			_ = p.connector.Close()
		}
	}
	d.projects = nil
	d.mu.Unlock()

	// GracefulStop waits for in-flight RPCs.
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		server.Stop()
	}
}

// projectFor loads a *Project for the given root, caching the result.
// Returns the cached entry on subsequent calls. The entry holds a live
// SourceConnector with sync-cache state.
func (d *daemonService) projectFor(root string) (*projectEntry, error) {
	if root == "" {
		return nil, errors.New("project root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	d.mu.Lock()
	if entry, ok := d.projects[abs]; ok {
		d.lastUsed = time.Now()
		d.mu.Unlock()
		return entry, nil
	}
	d.mu.Unlock()

	// Load outside the mutex.
	proj, err := bproject.FindProject(abs)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}

	formatReg := d.formatReg
	conn, err := connector.NewSourceConnector(proj, formatReg)
	if err != nil {
		// Some calls (ListFiles, Status) don't need the server, so fall
		// back to a local-only connector.
		conn = connector.NewLocalConnector(proj, formatReg)
	}

	entry := &projectEntry{
		project:   proj,
		connector: conn,
		formatReg: formatReg,
	}

	d.mu.Lock()
	if existing, ok := d.projects[abs]; ok {
		// Race: another caller loaded the same project. Drop ours.
		d.lastUsed = time.Now()
		d.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	d.projects[abs] = entry
	d.lastUsed = time.Now()
	d.mu.Unlock()
	return entry, nil
}

// ─────────────────────────────────────────────────────────────────────
// SourceConnectorService implementation
// ─────────────────────────────────────────────────────────────────────

func (d *daemonService) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	d.touch()
	if req.GetProject() == nil {
		return nil, errors.New("project ref required")
	}
	entry, err := d.projectFor(req.GetProject().GetRoot())
	if err != nil {
		return nil, err
	}
	st, err := entry.connector.Status(ctx)
	if err != nil {
		return nil, err
	}
	resp := &pb.StatusResponse{
		ConnectorId: st.ConnectorID,
		ItemCount:   int32(st.ItemCount),
		FileCount:   int32(st.FileCount),
		WordCount:   int32(st.WordCount),
		PendingPull: int32(st.PendingPull),
		PendingPush: int32(st.PendingPush),
		Errors:      st.Errors,
	}
	if !st.LastSync.IsZero() {
		resp.LastSync = st.LastSync.Format(time.RFC3339)
	}
	return resp, nil
}

func (d *daemonService) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	d.touch()
	if req.GetProject() == nil {
		return nil, errors.New("project ref required")
	}
	entry, err := d.projectFor(req.GetProject().GetRoot())
	if err != nil {
		return nil, err
	}
	files, err := entry.connector.ListFiles(ctx, req.GetPaths())
	if err != nil {
		return nil, err
	}
	out := make([]*pb.FileEntry, 0, len(files))
	for _, f := range files {
		out = append(out, &pb.FileEntry{
			Path:       f.Path,
			Format:     f.Format,
			BlockCount: int32(f.BlockCount),
			WordCount:  int32(f.WordCount),
			DirtyCount: int32(f.DirtyCount),
		})
	}
	return &pb.ListFilesResponse{Files: out}, nil
}

func (d *daemonService) Push(ctx context.Context, req *pb.PushRequest) (*pb.PushResponse, error) {
	d.touch()
	if req.GetProject() == nil {
		return nil, errors.New("project ref required")
	}
	entry, err := d.projectFor(req.GetProject().GetRoot())
	if err != nil {
		return nil, err
	}
	res, err := entry.connector.Push(ctx, bowrainconn.PushOptions{
		Paths:  req.GetPaths(),
		Force:  req.GetForce(),
		DryRun: req.GetDryRun(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.PushResponse{
		BlocksPushed: int32(res.BlocksPushed),
		AssetsPushed: int32(res.AssetsPushed),
		FilesScanned: int32(res.FilesScanned),
		ChunkCount:   int32(res.ChunkCount),
		WordCount:    int32(res.WordCount),
		PushId:       res.PushID,
	}, nil
}

func (d *daemonService) Pull(ctx context.Context, req *pb.PullRequest) (*pb.PullResponse, error) {
	d.touch()
	if req.GetProject() == nil {
		return nil, errors.New("project ref required")
	}
	entry, err := d.projectFor(req.GetProject().GetRoot())
	if err != nil {
		return nil, err
	}
	locales := make([]model.LocaleID, 0, len(req.GetLocales()))
	for _, l := range req.GetLocales() {
		locales = append(locales, model.LocaleID(l))
	}
	res, err := entry.connector.Pull(ctx, bowrainconn.PullOptions{
		Locales: locales,
		Force:   req.GetForce(),
		DryRun:  req.GetDryRun(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.PullResponse{
		BlocksPulled: int32(res.BlocksPulled),
		FilesWritten: int32(res.FilesWritten),
		LocalesCount: int32(res.LocalesCount),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────
// DaemonControlService implementation
// ─────────────────────────────────────────────────────────────────────

func (d *daemonService) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Version:       pluginVersion,
		UptimeSeconds: int64(time.Since(d.startedAt).Seconds()),
	}, nil
}

func (d *daemonService) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	d.shutdownMu.Do(func() {
		close(d.shutdownCh)
	})
	return &pb.ShutdownResponse{}, nil
}
