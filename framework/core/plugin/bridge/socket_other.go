//go:build !linux

package bridge

import "os"

// generateSocketPath returns "" on non-Linux platforms.
// macOS TCP localhost outperforms both kqueue UDS and NIO UDS in benchmarks.
// Linux epoll UDS bypasses the TCP stack with kernel splice/zero-copy.
func generateSocketPath() string { return "" }

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
}

const bridgeSocketEnvVar = "NEOKAPI_BRIDGE_SOCKET"
