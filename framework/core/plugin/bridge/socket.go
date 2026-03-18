//go:build linux

package bridge

import (
	"os"
	"path/filepath"
)

// generateSocketPath returns a Unix domain socket path for bridge IPC.
// Returns "" if the path cannot be created (caller falls back to TCP).
// Set NEOKAPI_BRIDGE_TCP=1 to force TCP (useful for debugging).
//
// UDS is only used on Linux where epoll gives measurable gains over TCP
// localhost (kernel splice, zero-copy). On macOS, TCP localhost is faster
// than both kqueue UDS and NIO UDS in benchmarks.
func generateSocketPath() string {
	if os.Getenv("NEOKAPI_BRIDGE_TCP") == "1" {
		return ""
	}
	dir := filepath.Join(os.TempDir(), "kapi-bridge")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	f, err := os.CreateTemp(dir, "*.sock")
	if err != nil {
		return ""
	}
	path := f.Name()
	f.Close()
	os.Remove(path)
	return path
}

func grpcTarget(addr string) string {
	if isSocketAddr(addr) {
		return "unix:" + addr
	}
	return "passthrough:///" + addr
}

func isSocketAddr(addr string) bool {
	return len(addr) > 0 && addr[0] == '/'
}

func cleanupSocket(path string) {
	if path == "" {
		return
	}
	os.Remove(path)
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "kapi-bridge" {
		os.Remove(dir)
	}
}

const bridgeSocketEnvVar = "NEOKAPI_BRIDGE_SOCKET"
