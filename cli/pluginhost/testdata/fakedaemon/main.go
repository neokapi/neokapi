// Command fakedaemon is a minimal Mode-C plugin used by daemon_test.go.
//
// It binds a Unix socket, prints a JSON handshake on stdout, and serves
// a gRPC server that registers no service methods (the pool only needs
// a successful TCP-level dial + RPC connection to consider it ready).
//
// Behavior is controlled via env vars:
//
//	FAKE_DAEMON_NAME       Plugin name embedded in handshake (default "fake")
//	FAKE_DAEMON_VERSION    Version embedded in handshake (default "0.0.1")
//	FAKE_DAEMON_NO_HANDSHAKE  If "1", do not print the handshake (forces a
//	                           startup-timeout error in the pool)
//	FAKE_DAEMON_CRASH_AFTER  Duration string (e.g. "200ms"); the daemon
//	                           exits with status 1 after this delay.
//	                           Mimics a crashed plugin.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
)

func main() {
	name := envOr("FAKE_DAEMON_NAME", "fake")
	version := envOr("FAKE_DAEMON_VERSION", "0.0.1")

	// Pick a unique socket path under TMPDIR.
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("kapi-%s-%d.sock", name, os.Getpid()))
	_ = os.Remove(socket)

	lis, err := net.Listen("unix", socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakedaemon: listen %s: %v\n", socket, err)
		os.Exit(1)
	}
	if err := os.Chmod(socket, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "fakedaemon: chmod %s: %v\n", socket, err)
	}

	server := grpc.NewServer()
	go func() {
		if err := server.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "fakedaemon: serve: %v\n", err)
		}
	}()

	// Print handshake unless explicitly suppressed. When suppressed, we
	// also do NOT emit any subsequent stdout lines — the daemon just
	// hangs, which is what the pool's startup-timeout test expects.
	if os.Getenv("FAKE_DAEMON_NO_HANDSHAKE") == "1" {
		// Block forever (until SIGTERM).
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		_ = lis.Close()
		_ = os.Remove(socket)
		return
	}
	hs := map[string]any{
		"socket":  socket,
		"version": version,
		"pid":     os.Getpid(),
	}
	enc, _ := json.Marshal(hs)
	fmt.Println(string(enc))
	// Subsequent log lines on stdout are forwarded by the pool.
	fmt.Println("fakedaemon ready")

	// Crash after delay, if requested.
	if d := os.Getenv("FAKE_DAEMON_CRASH_AFTER"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			go func() {
				time.Sleep(dur)
				server.Stop()
				_ = lis.Close()
				_ = os.Remove(socket)
				os.Exit(1)
			}()
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	server.GracefulStop()
	_ = lis.Close()
	_ = os.Remove(socket)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
