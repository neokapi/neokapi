package desktopclose

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// ExitOnSecondSignal makes a SIGTERM or SIGINT ask like any quit, and a
// second one while that quit is still held end the process.
//
// Wails v3.1.0 handles the first SIGTERM or SIGINT by calling App.Quit and
// then stops listening (internal/signal/signal.go). A guarded quit can be
// held, and with nobody at the screen to answer, only SIGKILL would then end
// the app. So each signal is handled here. One that arrives with no held
// quit calls quit (App.Quit), which the guard holds or lets through. One that
// arrives while that quit is still held is the sender insisting: it exits
// with 128 plus the signal number, as a shell reports a process killed by
// it. When the person answers the held quit and keeps working, the count
// starts again, so a later signal asks again. exit is os.Exit outside tests.
// The returned stop removes the handling.
func (g *Guard) ExitOnSecondSignal(quit func(), exit func(code int)) (stop func()) {
	var mu sync.Mutex
	seen := 0
	g.Configure(func(g *Guard) {
		g.onSettled = func() {
			mu.Lock()
			seen = 0
			mu.Unlock()
		}
	})

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				mu.Lock()
				seen++
				n := seen
				mu.Unlock()
				if n < 2 {
					if quit != nil {
						quit()
					}
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
