package av

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetResolver() {
	binDir = ""
	locator = nil
	binOnce = sync.Once{}
}

func TestResolveBin(t *testing.T) {
	t.Cleanup(resetResolver)
	resetResolver()
	dir := t.TempDir()
	bundled := filepath.Join(dir, exeName("ffmpeg"))
	require.NoError(t, os.WriteFile(bundled, []byte("#!/bin/sh\n"), 0o755))
	SetBinDir(dir)
	assert.Equal(t, bundled, resolveBin("ffmpeg"))
	assert.Equal(t, "ffprobe", resolveBin("ffprobe"))
}

func TestLocatorLazyOnce(t *testing.T) {
	t.Cleanup(resetResolver)
	resetResolver()
	dir := t.TempDir()
	calls := 0
	SetBinLocator(func() string { calls++; return dir })
	_ = resolveBin("ffmpeg")
	_ = resolveBin("ffprobe")
	assert.Equal(t, 1, calls, "locator is called at most once")
	assert.Equal(t, dir, resolveDir())
}
