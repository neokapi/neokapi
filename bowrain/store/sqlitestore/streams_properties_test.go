package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamProperties_RoundTrip verifies the streams.properties column added
// for stream-level bindings (e.g. the brand-voice profile) is persisted through
// create, read, and update — the store change that makes the resolver's stream
// rung real rather than a dropped write.
func TestStreamProperties_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID:  p.ID,
		Name:       "v2",
		Properties: map[string]string{coreprofile.PropertyProfileID: "profile-abc"},
	}))

	got, err := s.GetStream(ctx, p.ID, "v2")
	require.NoError(t, err)
	assert.Equal(t, "profile-abc", got.Properties[coreprofile.PropertyProfileID],
		"stream brand-voice binding survives create/read")

	// A property update must merge-persist through UpdateStream.
	got.Properties[coreprofile.PropertyChannel] = "email"
	require.NoError(t, s.UpdateStream(ctx, got))

	reloaded, err := s.GetStream(ctx, p.ID, "v2")
	require.NoError(t, err)
	assert.Equal(t, "profile-abc", reloaded.Properties[coreprofile.PropertyProfileID])
	assert.Equal(t, "email", reloaded.Properties[coreprofile.PropertyChannel])
}

// TestStreamProperties_EmptyDefaults verifies a stream created without
// properties reads back as an empty (not corrupt) map — the column defaults to
// '{}'.
func TestStreamProperties_EmptyDefaults(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{ProjectID: p.ID, Name: "v3"}))

	got, err := s.GetStream(ctx, p.ID, "v3")
	require.NoError(t, err)
	assert.Empty(t, got.Properties)
}
