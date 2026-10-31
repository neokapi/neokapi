package sqlitestore

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/workspace"
)

// A context object is created once: the same bytes again change nothing,
// other bytes are refused, and a refused batch writes none of its objects.
func TestContextObjects_CreatedOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	other := createTestProject(t, s)

	seg1 := workspace.SegmentName("w1", "000000000000000000000001")
	seg2 := workspace.SegmentName("w2", "000000000000000000000002")
	blob := []byte("payload")
	blobName := workspace.BlobObjectName(workspace.BlobAddress(blob))

	require.NoError(t, s.PutContextObjects(ctx, p.ID, "member-a", []platstore.ContextObject{
		{Name: seg2, Data: []byte("two\n")},
		{Name: seg1, Data: []byte("one\n")},
		{Name: blobName, Data: blob},
	}))

	logs, err := s.ListContextObjects(ctx, p.ID, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Equal(t, []string{seg1, seg2}, logs)
	blobs, err := s.ListContextObjects(ctx, p.ID, workspace.RemoteBlobDir)
	require.NoError(t, err)
	assert.Equal(t, []string{blobName}, blobs)
	none, err := s.ListContextObjects(ctx, other.ID, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Empty(t, none, "another project holds its own objects")

	got, err := s.GetContextObject(ctx, p.ID, seg1)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("one\n"))
	assert.Equal(t, "one\n", string(got.Data))
	assert.Equal(t, hex.EncodeToString(sum[:]), got.SHA256)
	assert.Equal(t, "member-a", got.PushedBy)
	assert.False(t, got.PushedAt.IsZero())

	require.NoError(t, s.PutContextObjects(ctx, p.ID, "member-b", []platstore.ContextObject{{Name: seg1, Data: []byte("one\n")}}),
		"the bytes an object already holds change nothing")
	got, err = s.GetContextObject(ctx, p.ID, seg1)
	require.NoError(t, err)
	assert.Equal(t, "member-a", got.PushedBy, "the member who created the object stays on it")

	seg3 := workspace.SegmentName("w1", "000000000000000000000003")
	err = s.PutContextObjects(ctx, p.ID, "member-b", []platstore.ContextObject{
		{Name: seg3, Data: []byte("three\n")},
		{Name: seg1, Data: []byte("changed\n")},
	})
	require.ErrorIs(t, err, platstore.ErrContextObjectExists)
	_, err = s.GetContextObject(ctx, p.ID, seg3)
	require.ErrorIs(t, err, platstore.ErrNoContextObject, "a refused batch writes none of its objects")
	got, err = s.GetContextObject(ctx, p.ID, seg1)
	require.NoError(t, err)
	assert.Equal(t, "one\n", string(got.Data))
}

func TestContextObjects_NamesOutsideTheLayoutAreRefused(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	for _, name := range []string{"../escape", "log/w1/../../x.jsonl", "notes.txt", "blobs/XYZ"} {
		require.Error(t, s.PutContextObjects(ctx, p.ID, "m", []platstore.ContextObject{{Name: name, Data: []byte("x")}}), name)
		_, err := s.GetContextObject(ctx, p.ID, name)
		require.Error(t, err, name)
		require.NotErrorIs(t, err, platstore.ErrNoContextObject, name)
	}
	_, err := s.ListContextObjects(ctx, p.ID, "log")
	require.Error(t, err, "a directory is named with its slash, as the layout names it")
}

func TestContextObjects_Capability(t *testing.T) {
	s := newTestStore(t)
	got, ok := platstore.ContextObjects(s)
	require.True(t, ok)
	assert.Same(t, s, got.(*SQLiteStore))
}
