package host

import (
	"testing"
	"time"

	coregraph "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileHitsRenderTheWindow(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	windows := []project.ProfileWindow{
		{Name: "spring-sale", Validity: &coregraph.Validity{ValidFrom: &from, ValidTo: &to}},
	}
	hits := profileHits(windows)
	require.Len(t, hits, 1)
	assert.Equal(t, "spring-sale", hits[0].Name)
	assert.Equal(t, "2026-01-01", hits[0].ValidFrom, "midnight bounds render as bare dates")
	assert.Equal(t, "2026-07-01", hits[0].ValidTo)

	assert.Nil(t, profileHits(nil))
}

func TestValidityState(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	v := &coregraph.Validity{ValidFrom: &from, ValidTo: &to}

	assert.Equal(t, "upcoming", validityState(v, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "active", validityState(v, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "expired", validityState(v, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)), "valid_to is exclusive")
	assert.Equal(t, "active", validityState(nil, time.Now()), "an unbounded profile is always active")
}
