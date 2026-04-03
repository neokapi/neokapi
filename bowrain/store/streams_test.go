package store

import (
	"context"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stream CRUD
// ---------------------------------------------------------------------------

func TestStreamCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Create "main" stream.
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "main",
	}))

	t.Run("create and get", func(t *testing.T) {
		st := &platstore.Stream{
			ProjectID:   p.ID,
			Name:        "feature/x",
			Parent:      "main",
			Description: "test stream",
			Visibility:  platstore.StreamPublic,
			CreatedBy:   "user-1",
		}
		require.NoError(t, s.CreateStream(ctx, st))

		got, err := s.GetStream(ctx, p.ID, "feature/x")
		require.NoError(t, err)
		assert.Equal(t, "feature/x", got.Name)
		assert.Equal(t, "main", got.Parent)
		assert.Equal(t, "test stream", got.Description)
		assert.False(t, got.Locked)
		assert.Empty(t, got.LockedBy)
		assert.Nil(t, got.LockedAt)
	})

	t.Run("list", func(t *testing.T) {
		streams, err := s.ListStreams(ctx, p.ID, false)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(streams), 2)
	})
}

// ---------------------------------------------------------------------------
// Stream Lock
// ---------------------------------------------------------------------------

func TestStreamLock(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "main",
	}))
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "release/v1", Parent: "main",
	}))

	t.Run("lock stream", func(t *testing.T) {
		err := s.LockStream(ctx, p.ID, "release/v1", "user-1")
		require.NoError(t, err)

		st, err := s.GetStream(ctx, p.ID, "release/v1")
		require.NoError(t, err)
		assert.True(t, st.Locked)
		assert.Equal(t, "user-1", st.LockedBy)
		assert.NotNil(t, st.LockedAt)
	})

	t.Run("lock already locked stream fails", func(t *testing.T) {
		err := s.LockStream(ctx, p.ID, "release/v1", "user-2")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already locked")
	})

	t.Run("unlock stream", func(t *testing.T) {
		err := s.UnlockStream(ctx, p.ID, "release/v1")
		require.NoError(t, err)

		st, err := s.GetStream(ctx, p.ID, "release/v1")
		require.NoError(t, err)
		assert.False(t, st.Locked)
		assert.Empty(t, st.LockedBy)
		assert.Nil(t, st.LockedAt)
	})

	t.Run("lock non-existent stream fails", func(t *testing.T) {
		err := s.LockStream(ctx, p.ID, "nope", "user-1")
		assert.Error(t, err)
	})

	t.Run("unlock non-existent stream fails", func(t *testing.T) {
		err := s.UnlockStream(ctx, p.ID, "nope")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Stream Tags
// ---------------------------------------------------------------------------

func TestStreamTagCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "main",
	}))
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature/a", Parent: "main",
	}))

	t.Run("create and get tag", func(t *testing.T) {
		tag := &platstore.StreamTag{
			ProjectID: p.ID,
			Stream:    "feature/a",
			Name:      "v1.0",
			Kind:      platstore.TagKindRelease,
			Cursor:    42,
			Metadata:  map[string]string{"version": "1.0.0"},
			CreatedBy: "user-1",
		}
		require.NoError(t, s.CreateStreamTag(ctx, tag))
		assert.NotEmpty(t, tag.ID)
		assert.False(t, tag.CreatedAt.IsZero())

		got, err := s.GetStreamTag(ctx, p.ID, "feature/a", "v1.0")
		require.NoError(t, err)
		assert.Equal(t, "v1.0", got.Name)
		assert.Equal(t, platstore.TagKindRelease, got.Kind)
		assert.Equal(t, int64(42), got.Cursor)
		assert.Equal(t, "1.0.0", got.Metadata["version"])
		assert.Equal(t, "user-1", got.CreatedBy)
	})

	t.Run("list stream tags", func(t *testing.T) {
		// Add another tag on the same stream.
		require.NoError(t, s.CreateStreamTag(ctx, &platstore.StreamTag{
			ProjectID: p.ID,
			Stream:    "feature/a",
			Name:      "milestone-1",
			Kind:      platstore.TagKindMilestone,
			Cursor:    20,
			CreatedBy: "user-2",
		}))

		tags, err := s.ListStreamTags(ctx, p.ID, "feature/a")
		require.NoError(t, err)
		assert.Len(t, tags, 2)
	})

	t.Run("unique tag name per stream", func(t *testing.T) {
		err := s.CreateStreamTag(ctx, &platstore.StreamTag{
			ProjectID: p.ID,
			Stream:    "feature/a",
			Name:      "v1.0", // duplicate
			Kind:      platstore.TagKindCustom,
			CreatedBy: "user-1",
		})
		assert.Error(t, err)
	})

	t.Run("same tag name on different streams is ok", func(t *testing.T) {
		err := s.CreateStreamTag(ctx, &platstore.StreamTag{
			ProjectID: p.ID,
			Stream:    "main",
			Name:      "v1.0",
			Kind:      platstore.TagKindRelease,
			Cursor:    10,
			CreatedBy: "user-1",
		})
		require.NoError(t, err)
	})

	t.Run("delete tag", func(t *testing.T) {
		err := s.DeleteStreamTag(ctx, p.ID, "feature/a", "milestone-1")
		require.NoError(t, err)

		tags, err := s.ListStreamTags(ctx, p.ID, "feature/a")
		require.NoError(t, err)
		assert.Len(t, tags, 1)
	})

	t.Run("delete non-existent tag fails", func(t *testing.T) {
		err := s.DeleteStreamTag(ctx, p.ID, "feature/a", "nope")
		assert.Error(t, err)
	})

	t.Run("get non-existent tag fails", func(t *testing.T) {
		_, err := s.GetStreamTag(ctx, p.ID, "feature/a", "nope")
		assert.Error(t, err)
	})
}

func TestListProjectTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "main",
	}))
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature/a", Parent: "main",
	}))

	// Tags on different streams.
	require.NoError(t, s.CreateStreamTag(ctx, &platstore.StreamTag{
		ProjectID: p.ID, Stream: "main", Name: "release-1", Kind: platstore.TagKindRelease,
		CreatedBy: "user-1",
	}))
	require.NoError(t, s.CreateStreamTag(ctx, &platstore.StreamTag{
		ProjectID: p.ID, Stream: "feature/a", Name: "merged-main", Kind: platstore.TagKindMerge,
		CreatedBy: "user-1",
	}))
	require.NoError(t, s.CreateStreamTag(ctx, &platstore.StreamTag{
		ProjectID: p.ID, Stream: "main", Name: "qa-done", Kind: platstore.TagKindCustom,
		CreatedBy: "user-2",
	}))

	t.Run("list all project tags", func(t *testing.T) {
		tags, err := s.ListProjectTags(ctx, p.ID, "")
		require.NoError(t, err)
		assert.Len(t, tags, 3)
	})

	t.Run("filter by kind", func(t *testing.T) {
		tags, err := s.ListProjectTags(ctx, p.ID, platstore.TagKindMerge)
		require.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, "merged-main", tags[0].Name)

		tags, err = s.ListProjectTags(ctx, p.ID, platstore.TagKindRelease)
		require.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, "release-1", tags[0].Name)
	})
}

func TestStreamTagDefaultKind(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "main",
	}))

	tag := &platstore.StreamTag{
		ProjectID: p.ID,
		Stream:    "main",
		Name:      "my-tag",
		CreatedBy: "user-1",
	}
	require.NoError(t, s.CreateStreamTag(ctx, tag))
	assert.Equal(t, platstore.TagKindCustom, tag.Kind)

	got, err := s.GetStreamTag(ctx, p.ID, "main", "my-tag")
	require.NoError(t, err)
	assert.Equal(t, platstore.TagKindCustom, got.Kind)
}

func TestStreamTagNilMetadata(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "main",
	}))

	tag := &platstore.StreamTag{
		ProjectID: p.ID,
		Stream:    "main",
		Name:      "bare-tag",
		Kind:      platstore.TagKindCustom,
		CreatedBy: "user-1",
		// Metadata is nil.
	}
	require.NoError(t, s.CreateStreamTag(ctx, tag))

	got, err := s.GetStreamTag(ctx, p.ID, "main", "bare-tag")
	require.NoError(t, err)
	assert.Equal(t, "bare-tag", got.Name)
}
