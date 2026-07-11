package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUploadSweepStore serves a fixed candidate list and records clears.
type fakeUploadSweepStore struct {
	candidates []BrandScanUploadCleanup
	cleared    []string
}

func (f *fakeUploadSweepStore) SweepExpiredBrandScanUploads(context.Context, time.Duration, int) ([]BrandScanUploadCleanup, error) {
	return f.candidates, nil
}

func (f *fakeUploadSweepStore) ClearBrandScanUploadKeys(_ context.Context, id string) error {
	f.cleared = append(f.cleared, id)
	// Mirror the real store: a cleared job is no longer a sweep candidate.
	remaining := f.candidates[:0]
	for _, c := range f.candidates {
		if c.JobID != id {
			remaining = append(remaining, c)
		}
	}
	f.candidates = remaining
	return nil
}

// fakeSweepBlobStore is an in-memory BlobStore whose Delete errors on a
// missing key (azureblob semantics — the stricter backend) and can be forced
// to fail per key (transient outage).
type fakeSweepBlobStore struct {
	blobs      map[string][]byte
	deleted    []string
	failDelete map[string]bool
}

func newFakeSweepBlobStore(keys ...string) *fakeSweepBlobStore {
	s := &fakeSweepBlobStore{blobs: map[string][]byte{}, failDelete: map[string]bool{}}
	for _, k := range keys {
		s.blobs[k] = []byte("envelope")
	}
	return s
}

func (s *fakeSweepBlobStore) Upload(context.Context, []byte, corestorage.UploadOptions) (*corestorage.BlobRef, error) {
	return nil, corestorage.ErrNotSupported
}

func (s *fakeSweepBlobStore) Download(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.blobs[key]
	if !ok {
		return nil, corestorage.ErrBlobNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeSweepBlobStore) GenerateUploadURL(context.Context, string, corestorage.SignOptions) (string, error) {
	return "", corestorage.ErrNotSupported
}

func (s *fakeSweepBlobStore) GenerateDownloadURL(context.Context, string, corestorage.SignOptions) (string, error) {
	return "", corestorage.ErrNotSupported
}

func (s *fakeSweepBlobStore) Delete(_ context.Context, key string) error {
	if s.failDelete[key] {
		return errors.New("simulated backend outage")
	}
	if _, ok := s.blobs[key]; !ok {
		return errors.New("delete blob: not found")
	}
	delete(s.blobs, key)
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *fakeSweepBlobStore) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.blobs[key]
	return ok, nil
}

// TestBrandScanUploadSweeper_DeletesAndClears: the happy path — every
// envelope of an expired job is deleted and the job's keys are cleared.
func TestBrandScanUploadSweeper_DeletesAndClears(t *testing.T) {
	store := &fakeUploadSweepStore{candidates: []BrandScanUploadCleanup{
		{JobID: "job-1", UploadKeys: []string{"blob-a", "blob-b"}},
	}}
	blobs := newFakeSweepBlobStore("blob-a", "blob-b", "blob-unrelated")

	s := NewBrandScanUploadSweeper(store, blobs, 0, 0)
	jobsCleaned, blobsDeleted, err := s.sweepOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, jobsCleaned)
	assert.Equal(t, 2, blobsDeleted)
	assert.ElementsMatch(t, []string{"blob-a", "blob-b"}, blobs.deleted)
	assert.Equal(t, []string{"job-1"}, store.cleared)

	exists, err := blobs.Exists(t.Context(), "blob-unrelated")
	require.NoError(t, err)
	assert.True(t, exists, "blobs outside the job's keys must survive")
}

// TestBrandScanUploadSweeper_TransientFailureRetries: when a delete fails and
// the blob still exists, the job is NOT cleared, so the next sweep retries it
// instead of orphaning the remaining envelope.
func TestBrandScanUploadSweeper_TransientFailureRetries(t *testing.T) {
	store := &fakeUploadSweepStore{candidates: []BrandScanUploadCleanup{
		{JobID: "job-1", UploadKeys: []string{"blob-a", "blob-b"}},
		{JobID: "job-2", UploadKeys: []string{"blob-c"}},
	}}
	blobs := newFakeSweepBlobStore("blob-a", "blob-b", "blob-c")
	blobs.failDelete["blob-b"] = true

	s := NewBrandScanUploadSweeper(store, blobs, 0, 0)
	jobsCleaned, blobsDeleted, err := s.sweepOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, jobsCleaned, "only the unaffected job is cleaned")
	assert.Equal(t, 2, blobsDeleted)
	assert.Equal(t, []string{"job-2"}, store.cleared, "the failed job stays for the next sweep")

	// The outage recovers; the next sweep finishes the job.
	delete(blobs.failDelete, "blob-b")
	store.cleared = nil
	jobsCleaned, _, err = s.sweepOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, jobsCleaned)
	assert.Equal(t, []string{"job-1"}, store.cleared)
}

// TestBrandScanUploadSweeper_AbsentBlobCountsAsGone: a key whose blob no
// longer exists (content-addressed keys can be shared across jobs; azureblob
// errors on delete-missing) is a success for retention purposes.
func TestBrandScanUploadSweeper_AbsentBlobCountsAsGone(t *testing.T) {
	store := &fakeUploadSweepStore{candidates: []BrandScanUploadCleanup{
		{JobID: "job-1", UploadKeys: []string{"blob-gone", "blob-a"}},
	}}
	blobs := newFakeSweepBlobStore("blob-a") // blob-gone was never stored

	s := NewBrandScanUploadSweeper(store, blobs, 0, 0)
	jobsCleaned, blobsDeleted, err := s.sweepOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, jobsCleaned)
	assert.Equal(t, 1, blobsDeleted)
	assert.Equal(t, []string{"job-1"}, store.cleared)
}
