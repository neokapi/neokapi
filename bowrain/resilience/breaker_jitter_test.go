package resilience

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryAfterIsJittered: every job parked during one outage reads nearly the
// same remaining cooldown, so identical delays bring them all back at the same
// instant — where the half-open probe admits one and parks the rest again, in
// lockstep. The spread must be one-sided: never earlier than the circuit would
// admit, only later.
func TestRetryAfterIsJittered(t *testing.T) {
	s := testSettings()
	s.Cooldown = 10 * time.Second
	b := newBreaker("test:jitter", KindAI, s)

	seen := map[time.Duration]bool{}
	for range 64 {
		d := b.retryAfter()
		assert.GreaterOrEqual(t, d, s.Cooldown, "never earlier than the cooldown")
		assert.LessOrEqual(t, d, s.Cooldown+s.Cooldown/2, "and no more than half again")
		seen[d] = true
	}
	assert.Greater(t, len(seen), 1, "identical delays are the stampede")
}

// TestRetryAfterKeepsItsFloor: a cooldown that has all but elapsed still buys a
// second, so a deferral never re-enqueues instantly into a circuit that is
// still open.
func TestRetryAfterKeepsItsFloor(t *testing.T) {
	b := newBreaker("test:jitter-floor", KindAI, testSettings())
	failN(t, b, 4)
	require.Equal(t, StateOpen, b.State())

	time.Sleep(testSettings().Cooldown)
	d := b.retryAfter()
	assert.GreaterOrEqual(t, d, time.Second)
	assert.LessOrEqual(t, d, 2*time.Second)
}
