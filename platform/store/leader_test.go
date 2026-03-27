package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeaderElector_AcquireAndRelease(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	e := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 10*time.Second, 1*time.Second)

	// Initially not leader.
	assert.False(t, e.IsLeader())

	// Acquire.
	e.tryAcquireOrRenew()
	assert.True(t, e.IsLeader())

	// Release.
	e.release()
	assert.False(t, e.IsLeader())
}

func TestLeaderElector_TwoInstances(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	e1 := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 10*time.Second, 1*time.Second)
	e2 := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 10*time.Second, 1*time.Second)

	// e1 acquires first.
	e1.tryAcquireOrRenew()
	assert.True(t, e1.IsLeader())

	// e2 tries — should fail (lease not expired, different holder).
	e2.tryAcquireOrRenew()
	assert.False(t, e2.IsLeader())

	// e1 releases.
	e1.release()
	assert.False(t, e1.IsLeader())

	// Now e2 can acquire.
	e2.tryAcquireOrRenew()
	assert.True(t, e2.IsLeader())
}

func TestLeaderElector_ExpiredLeaseTakeover(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	// e1 acquires with very short TTL (2 seconds — RFC3339 has second precision).
	e1 := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 2*time.Second, 1*time.Second)
	e1.tryAcquireOrRenew()
	assert.True(t, e1.IsLeader())

	// Wait for lease to expire.
	time.Sleep(3 * time.Second)

	// e2 should be able to take over.
	e2 := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 10*time.Second, 1*time.Second)
	e2.tryAcquireOrRenew()
	assert.True(t, e2.IsLeader())

	// e1 should lose leadership on next check.
	e1.tryAcquireOrRenew()
	assert.False(t, e1.IsLeader())
}

func TestLeaderElector_RenewalKeepsLease(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	e1 := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 10*time.Second, 1*time.Second)
	e1.tryAcquireOrRenew()
	assert.True(t, e1.IsLeader())

	// Renew.
	e1.tryAcquireOrRenew()
	assert.True(t, e1.IsLeader())

	// Another instance still can't take it.
	e2 := NewLeaderElector(s.DB(), DialectSQLite, "test-lease", 10*time.Second, 1*time.Second)
	e2.tryAcquireOrRenew()
	assert.False(t, e2.IsLeader())
}
