//go:build unix

package desktopclose

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalHelperEnv makes the test binary act as a desktop app whose quit is
// held: it handles the first signal the way Wails does (one Quit, then it
// stops listening) against a guard with a blocker, so the quit is held and
// nobody answers.
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
	ExitOnSecondSignal(os.Exit)

	// Wails' handler: the first signal calls Quit, which the guard holds,
	// and then it stops listening.
	wails := make(chan os.Signal, 1)
	signal.Notify(wails, syscall.SIGTERM, syscall.SIGINT)
	fmt.Println("ready")
	<-wails
	signal.Stop(wails)
	if g.Allow(RouteQuit) {
		os.Exit(0)
	}
	fmt.Println("held")
	select {}
}

func startSignalHelper(t *testing.T) (*exec.Cmd, *bufio.Scanner) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), signalHelperEnv+"=1")
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	lines := bufio.NewScanner(out)
	require.True(t, lines.Scan())
	require.Equal(t, "ready", lines.Text())
	return cmd, lines
}

func TestExitOnSecondSignal_FirstAsksSecondEnds(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			cmd, lines := startSignalHelper(t)

			require.NoError(t, cmd.Process.Signal(sig))
			require.True(t, lines.Scan())
			assert.Equal(t, "held", lines.Text(), "the first signal's quit is held")

			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			require.NoError(t, cmd.Process.Signal(sig))
			select {
			case err := <-exited:
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr, "the second signal ends the process")
				assert.Equal(t, 128+int(sig), exitErr.ExitCode())
			case <-time.After(10 * time.Second):
				t.Fatal("the second signal did not end the process")
			}
		})
	}
}

func TestExitOnSecondSignal_StopRemovesTheHandling(t *testing.T) {
	codes := make(chan int, 1)
	stop := ExitOnSecondSignal(func(code int) { codes <- code })
	stop()
	select {
	case code := <-codes:
		t.Fatalf("exit called after stop with %d", code)
	default:
	}
}
