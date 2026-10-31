package sqlitestore

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
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
	var exists *platstore.ContextObjectExistsError
	require.ErrorAs(t, err, &exists)
	assert.Equal(t, seg1, exists.Name, "the error names the refused object")
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
	blobName := workspace.BlobObjectName(workspace.BlobAddress([]byte("right")))
	err := s.PutContextObjects(ctx, p.ID, "m", []platstore.ContextObject{{Name: blobName, Data: []byte("wrong")}})
	require.Error(t, err, "a blob is stored only under the address of its bytes")
	require.NotErrorIs(t, err, platstore.ErrContextObjectExists)
	_, err = s.GetContextObject(ctx, p.ID, blobName)
	require.ErrorIs(t, err, platstore.ErrNoContextObject)
	require.NoError(t, s.PutContextObjects(ctx, p.ID, "m", []platstore.ContextObject{{Name: blobName, Data: []byte("right")}}))

	_, err = s.ListContextObjects(ctx, p.ID, "log")
	require.Error(t, err, "a directory is named with its slash, as the layout names it")
}

func TestContextObjects_Capability(t *testing.T) {
	s := newTestStore(t)
	got, ok := platstore.ContextObjects(s)
	require.True(t, ok)
	assert.Same(t, s, got.(*SQLiteStore))
}

// A project's context binds to the first key named and keeps it; the digests
// list each object under a directory with the digest of its bytes.
func TestContextObjects_KeyAndDigests(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	other := createTestProject(t, s)

	key, err := s.ContextKey(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, key, "no push has bound a key yet")
	held, err := s.BindContextKey(ctx, p.ID, "site")
	require.NoError(t, err)
	assert.Equal(t, "site", held)
	held, err = s.BindContextKey(ctx, p.ID, "other")
	require.NoError(t, err)
	assert.Equal(t, "site", held, "a bound key stays")
	key, err = s.ContextKey(ctx, other.ID)
	require.NoError(t, err)
	assert.Empty(t, key, "another project binds its own")
	_, err = s.ContextKey(ctx, "no-such-project")
	require.ErrorIs(t, err, platstore.ErrNoContextProject)
	_, err = s.BindContextKey(ctx, "no-such-project", "site")
	require.ErrorIs(t, err, platstore.ErrNoContextProject)

	seg := workspace.SegmentName("w1", "000000000000000000000001")
	blob := []byte("payload")
	require.NoError(t, s.PutContextObjects(ctx, p.ID, "m", []platstore.ContextObject{
		{Name: seg, Data: []byte("one\n")},
		{Name: workspace.BlobObjectName(workspace.BlobAddress(blob)), Data: blob},
	}))
	digests, err := s.DigestContextObjects(ctx, p.ID, workspace.RemoteLogDir)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("one\n"))
	assert.Equal(t, map[string]string{seg: hex.EncodeToString(sum[:])}, digests)
	_, err = s.DigestContextObjects(ctx, p.ID, "log")
	require.Error(t, err)
}

// Version 39 opens a working copy from before it: the projects gain the
// context key column, the term candidates leave the review queue beside the
// entity reviews, and the list of rejected terms is dropped.
func TestMigrations_TermCandidatesLeaveTheReviewQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	ctx := t.Context()
	p := createTestProject(t, s)
	for _, stmt := range []string{
		`ALTER TABLE projects DROP COLUMN context_key`,
		`CREATE TABLE rejected_terms (project_id TEXT NOT NULL, term_text TEXT NOT NULL, locale TEXT NOT NULL)`,
		`INSERT INTO review_items (id, project_id, type, data) VALUES ('tc', '` + p.ID + `', 'term_candidate', '{}')`,
		`INSERT INTO review_items (id, project_id, type, data) VALUES ('er', '` + p.ID + `', 'entity_review', '{}')`,
		`DELETE FROM schema_migrations WHERE version >= 39`,
	} {
		_, err = s.db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, s.Close())

	reopened, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	key, err := reopened.ContextKey(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, key)
	var ids []string
	rows, err := reopened.db.QueryContext(ctx, `SELECT id FROM review_items ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"er"}, ids)
	var n int
	require.NoError(t, reopened.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'rejected_terms'`).Scan(&n))
	assert.Zero(t, n)
}
