package bridge

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrpcTarget(t *testing.T) {
	assert.Equal(t, "unix:/tmp/test.sock", grpcTarget("/tmp/test.sock"))
	assert.Equal(t, "passthrough:///localhost:50051", grpcTarget("localhost:50051"))
}

func TestIsSocketAddr(t *testing.T) {
	assert.True(t, isSocketAddr("/tmp/test.sock"))
	assert.False(t, isSocketAddr("localhost:50051"))
	assert.False(t, isSocketAddr(""))
}

func TestCleanupSocket(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.sock")
	require.NoError(t, os.WriteFile(p, nil, 0o600))
	cleanupSocket(p)
	_, err := os.Stat(p)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupSocketNoOp(t *testing.T) {
	cleanupSocket("")
	cleanupSocket("/tmp/nonexistent.sock")
}

func TestGenerateSocketPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		assert.Empty(t, generateSocketPath(), "should return empty on non-Linux")
		return
	}
	path := generateSocketPath()
	require.NotEmpty(t, path)
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
	assert.Less(t, len(path), 108)
	os.Remove(filepath.Dir(path))
}
