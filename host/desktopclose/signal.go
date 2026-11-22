package desktopclose

import (
	"os"
	"os/signal"
	"syscall"
)

// ExitOnSecondSignal makes the second SIGTERM or SIGINT end the process.
//
// Wails v3.1.0 handles the first SIGTERM or SIGINT by calling App.Quit and
// then stops listening (internal/signal/signal.go). A guarded quit can be
// held, and with nobody at the screen to answer, only SIGKILL would then end
// the app. The first signal still asks; the second one is the sender
// insisting, and exits with 128 plus the signal number, as a shell reports a
// process killed by it. exit is os.Exit outside tests. The returned stop
// removes the handling.
func ExitOnSecondSignal(exit func(code int)) (stop func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		seen := 0
		for {
			select {
			case sig := <-signals:
				seen++
				if seen < 2 {
					continue
				}
				code := 1
				if s, ok := sig.(syscall.Signal); ok {
					code = 128 + int(s)
				}
				exit(code)
				return
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}
