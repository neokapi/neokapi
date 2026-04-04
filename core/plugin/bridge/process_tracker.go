package bridge

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// processTracker keeps a registry of all bridge subprocesses started by this
// Go process. On SIGINT/SIGTERM, it kills all tracked processes as a safety net
// to prevent orphaned JVM subprocesses.
var processTracker = newTracker()

func newTracker() *tracker {
	t := &tracker{}
	t.initSignalOnce = sync.OnceFunc(t.startSignalHandler)
	return t
}

type tracker struct {
	mu             sync.Mutex
	processes      map[*os.Process]struct{}
	initSignalOnce func()
}

func (t *tracker) track(p *os.Process) {
	t.initSignalOnce()

	t.mu.Lock()
	if t.processes == nil {
		t.processes = make(map[*os.Process]struct{})
	}
	t.processes[p] = struct{}{}
	t.mu.Unlock()
}

func (t *tracker) untrack(p *os.Process) {
	t.mu.Lock()
	delete(t.processes, p)
	t.mu.Unlock()
}

func (t *tracker) killAll() {
	t.mu.Lock()
	procs := make([]*os.Process, 0, len(t.processes))
	for p := range t.processes {
		procs = append(procs, p)
	}
	t.processes = nil
	t.mu.Unlock()

	for _, p := range procs {
		_ = p.Kill()
	}
}

func (t *tracker) startSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		t.killAll()
		// Reset and re-raise so the default handler runs.
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGINT)
	}()
}

// KillTrackedProcesses kills all tracked bridge subprocesses. Exported for use
// by test cleanup helpers and shutdown hooks.
func KillTrackedProcesses() {
	processTracker.killAll()
}
