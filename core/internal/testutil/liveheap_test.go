package testutil

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBusyGoroutine(t *testing.T) {
	const caller = "goroutine 1 [running]:\nmain.main()\n\n"
	for _, tc := range []struct {
		name, dump, want string
	}{
		{"only the caller", caller, ""},
		{"every other goroutine blocked", caller +
			"goroutine 7 [chan send]:\nx()\n\n" +
			"goroutine 8 [select, 2 minutes]:\ny()\n\n" +
			"goroutine 9 [sync.Cond.Wait]:\nz()\n\n" +
			"goroutine 10 [IO wait]:\nw()\n", ""},
		{"a runnable goroutine", caller + "goroutine 7 [chan send]:\n\ngoroutine 12 [runnable]:\nr()\n",
			"goroutine 12 [runnable]:"},
		{"a running goroutine", caller + "goroutine 13 [running]:\nr()\n", "goroutine 13 [running]:"},
		{"a preempted goroutine", caller + "goroutine 14 [preempted]:\nr()\n", "goroutine 14 [preempted]:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(busyGoroutine([]byte(tc.dump))))
		})
	}
}

// LiveHeap returns once the goroutine below has blocked, and counts what it
// holds while blocked.
func TestLiveHeapWaitsForABlockedPipeline(t *testing.T) {
	held := make(chan []byte)
	release := make(chan struct{})
	go func() {
		buf := make([]byte, 8<<20)
		held <- buf[:1]
		<-release
		runtime.KeepAlive(buf)
	}()
	<-held
	before := LiveHeap()
	close(release)
	assert.Greater(t, before, uint64(8<<20), "the blocked goroutine's buffer is live")
}
