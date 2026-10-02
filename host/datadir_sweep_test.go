package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sweep removes the root of every test binary that is not running, and a
// root carrying this process's own id, which an earlier process with the same
// id left. Everything else under the temporary directory stays.
func TestSweepTestDataDirs(t *testing.T) {
	const self, running, dead = 4242, 200, 100
	tmp := t.TempDir()
	dir := func(name string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, name, "workspaces", "default"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name, "workspaces", "default", "workspace.db"), []byte("x"), 0o600))
	}
	file := func(name string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o600))
	}
	root := func(pid int) string { return testDataDirPrefix + strconv.Itoa(pid) }

	dir(root(self))
	dir(root(dead))
	dir(root(running))
	dir(testDataDirPrefix + "abc")
	dir(testDataDirPrefix + "0100")
	dir("kapi-other-100")
	file(root(dead + 1))

	removed := sweepTestDataDirs(tmp, self, func(pid int) bool { return pid == running })
	assert.ElementsMatch(t, []string{root(self), root(dead)}, removed)

	tests := []struct {
		name string
		kept bool
	}{
		{root(self), false},
		{root(dead), false},
		{root(running), true},
		{testDataDirPrefix + "abc", true},
		{testDataDirPrefix + "0100", true},
		{"kapi-other-100", true},
		{root(dead + 1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := os.Stat(filepath.Join(tmp, tt.name))
			if tt.kept {
				assert.NoError(t, err, "kept")
			} else {
				assert.True(t, os.IsNotExist(err), "removed")
			}
		})
	}

	assert.Empty(t, sweepTestDataDirs(filepath.Join(tmp, "missing"), self, processAlive),
		"a temporary directory that cannot be read removes nothing")
}

// processAlive answers for this process, for a process that has exited, and
// for an id no process can have.
func TestProcessAlive(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("no process table")
	}
	assert.True(t, processAlive(os.Getpid()))
	assert.False(t, processAlive(exitedPID(t)))
	assert.False(t, processAlive(0))
	assert.False(t, processAlive(-1))
}

// exitedPID runs this test binary with no test selected and returns the id it
// ran under, once it has exited and been reaped.
func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	require.NoError(t, cmd.Run())
	return cmd.ProcessState.Pid()
}

// envDataRootChild marks the run of this test binary that
// TestATestBinaryLeavesNoDataRootBehind starts.
const envDataRootChild = "KAPI_TEST_DATA_ROOT_CHILD"

// A test binary that writes into its data root leaves nothing under the
// temporary directory when it exits, and removes the root a dead binary left
// there when it starts.
func TestATestBinaryLeavesNoDataRootBehind(t *testing.T) {
	if os.Getenv(envDataRootChild) != "" {
		t.Skip("the child run")
	}
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("no subprocesses")
	}
	tmp := t.TempDir()
	stale := filepath.Join(tmp, testDataDirPrefix+strconv.Itoa(exitedPID(t)))
	require.NoError(t, os.MkdirAll(filepath.Join(stale, "workspaces"), 0o700))

	cmd := exec.Command(os.Args[0], "-test.run=^TestDataRootChild$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), envDataRootChild+"=1", "TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	require.Contains(t, string(out), "--- PASS: TestDataRootChild", "%s", out)

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), testDataDirPrefix), "%s was left behind", e.Name())
	}
}

// TestDataRootChild is the child run's test: it writes into the data root, as
// a test that opens a project does, after the sweep has removed the stale root.
func TestDataRootChild(t *testing.T) {
	if os.Getenv(envDataRootChild) == "" {
		t.Skip("run by TestATestBinaryLeavesNoDataRootBehind")
	}
	dir := DataDir()
	require.Equal(t, testDataRoot(os.TempDir(), os.Getpid()), dir)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o600))

	entries, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)
	var roots []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), testDataDirPrefix) {
			roots = append(roots, e.Name())
		}
	}
	assert.Equal(t, []string{filepath.Base(dir)}, roots, "the stale root was swept when this binary resolved its own")
}
