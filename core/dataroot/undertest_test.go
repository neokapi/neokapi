package dataroot

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestATestBinaryIsUnderTest(t *testing.T) {
	assert.True(t, UnderTest())
}

// A released binary whose file name happens to end in ".test" keeps the
// platform's locations: only a binary `go test` built takes the test path.
func TestARenamedBinaryIsNotATest(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("no subprocesses")
	}
	name := "kapi.test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-trimpath", "-o", bin, "./testdata/undertest")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s", out)

	out, err = exec.Command(bin).Output()
	require.NoError(t, err)
	assert.Equal(t, "false", strings.TrimSpace(string(out)))
}
