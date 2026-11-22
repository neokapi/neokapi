//go:build unix

package desktopclose

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalHelperEnv makes the test binary act as a desktop app with unsaved
// work: its guard always has a blocker, so a quit is held, and nobody
// answers unless the parent writes "cancel" to its stdin (the person choosing
// Cancel and going on working).
const signalHelperEnv = "DESKTOPCLOSE_SIGNAL_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(signalHelperEnv) == "1" {
		runSignalHelper()
		return
	}
	os.Exit(m.Run())
}

func runSignalHelper() {
	g := &Guard{Blockers: func() []Blocker { return []Blocker{tabA} }}
	g.SetClosers(func() { os.Exit(0) }, func() { os.Exit(0) })
	quit := func() {
		if g.Allow(RouteQuit) {
			os.Exit(0)
		}
		fmt.Println("held")
	}
	g.ExitOnSecondSignal(quit, os.Exit)
	fmt.Println("ready")
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		if in.Text() == "cancel" {
			g.Acknowledge()
			g.Cancel()
			fmt.Println("cancelled")
		}
	}
	select {}
}

type signalHelper struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines *bufio.Scanner
}

func startSignalHelper(t *testing.T) *signalHelper {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), signalHelperEnv+"=1")
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	h := &signalHelper{cmd: cmd, stdin: stdin, lines: bufio.NewScanner(out)}
	h.expect(t, "ready")
	return h
}

func (h *signalHelper) expect(t *testing.T, line string) {
	t.Helper()
	require.True(t, h.lines.Scan(), "helper ended before %q", line)
	require.Equal(t, line, h.lines.Text())
}

func (h *signalHelper) expectExit(t *testing.T, code int) {
	t.Helper()
	exited := make(chan error, 1)
	go func() { exited <- h.cmd.Wait() }()
	select {
	case err := <-exited:
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, "the signal ends the process")
		assert.Equal(t, code, exitErr.ExitCode())
	case <-time.After(10 * time.Second):
		t.Fatal("the process did not end")
	}
}

func TestExitOnSecondSignal_FirstAsksSecondEnds(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			h := startSignalHelper(t)
			require.NoError(t, h.cmd.Process.Signal(sig))
			h.expect(t, "held")
			require.NoError(t, h.cmd.Process.Signal(sig))
			h.expectExit(t, 128+int(sig))
		})
	}
}

// The person answers the held quit with Cancel and keeps working: a later
// signal asks again rather than ending the process, and only a second signal
// while that one is held ends it.
func TestExitOnSecondSignal_AnsweredQuitStartsTheCountAgain(t *testing.T) {
	h := startSignalHelper(t)
	require.NoError(t, h.cmd.Process.Signal(syscall.SIGTERM))
	h.expect(t, "held")
	_, err := io.WriteString(h.stdin, "cancel\n")
	require.NoError(t, err)
	h.expect(t, "cancelled")

	require.NoError(t, h.cmd.Process.Signal(syscall.SIGTERM))
	h.expect(t, "held")
	require.NoError(t, h.cmd.Process.Signal(syscall.SIGINT))
	h.expectExit(t, 128+int(syscall.SIGINT))
}

func TestExitOnSecondSignal_StopRemovesTheHandling(t *testing.T) {
	codes := make(chan int, 1)
	g := &Guard{}
	stop := g.ExitOnSecondSignal(nil, func(code int) { codes <- code })
	stop()
	select {
	case code := <-codes:
		t.Fatalf("exit called after stop with %d", code)
	default:
	}
}
