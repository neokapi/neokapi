package s3blob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testAccessKey = "minioadmin"
	testSecretKey = "minioadmin"
	testBucket    = "bowrain-test"
)

// newMinioStore starts a throwaway MinIO (S3-compatible) container, creates the
// test bucket, and returns a Store pointed at it. This exercises the real S3 API
// surface — presigning, multipart-free PUT/GET, HeadObject 404 semantics — the
// same code that runs against Amazon S3 in production, per the no-mocks rule.
func newMinioStore(t *testing.T) *Store {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping MinIO container test in -short mode")
	}

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Image:        "minio/minio:latest",
		Cmd:          []string{"server", "/data"},
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     testAccessKey,
			"MINIO_ROOT_PASSWORD": testSecretKey,
		},
		WaitingFor: wait.ForHTTP("/minio/health/ready").
			WithPort("9000/tcp").
			WithStartupTimeout(60 * time.Second),
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)
	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	store, err := New(ctx, Options{
		Region:          "us-east-1",
		Bucket:          testBucket,
		Endpoint:        endpoint,
		UsePathStyle:    true,
		AccessKeyID:     testAccessKey,
		SecretAccessKey: testSecretKey,
	})
	require.NoError(t, err)

	// White-box: use the store's own client to provision the bucket.
	_, err = store.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(testBucket)})
	require.NoError(t, err)

	return store
}

func TestUploadAndDownload(t *testing.T) {
	s := newMinioStore(t)
	ctx := t.Context()
	data := []byte("hello s3 blob storage")

	ref, err := s.Upload(ctx, data, storage.UploadOptions{ContentType: "text/plain"})
	require.NoError(t, err)
	assert.NotEmpty(t, ref.Key)
	assert.Equal(t, int64(len(data)), ref.Size)
	assert.Equal(t, "text/plain", ref.ContentType)

	rc, err := s.Download(ctx, ref.Key)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestDedup(t *testing.T) {
	s := newMinioStore(t)
	ctx := t.Context()
	data := []byte("dedup test data")

	ref1, err := s.Upload(ctx, data, storage.UploadOptions{})
	require.NoError(t, err)
	ref2, err := s.Upload(ctx, data, storage.UploadOptions{})
	require.NoError(t, err)
	assert.Equal(t, ref1.Key, ref2.Key, "same data should produce same key")
}

func TestExistsAndDelete(t *testing.T) {
	s := newMinioStore(t)
	ctx := t.Context()

	exists, err := s.Exists(ctx, "0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	assert.False(t, exists, "a missing key must report not-exists, not error")

	ref, err := s.Upload(ctx, []byte("exists then delete"), storage.UploadOptions{})
	require.NoError(t, err)

	exists, err = s.Exists(ctx, ref.Key)
	require.NoError(t, err)
	assert.True(t, exists)

	require.NoError(t, s.Delete(ctx, ref.Key))

	exists, err = s.Exists(ctx, ref.Key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Deleting an already-absent key is a no-op, not an error.
	require.NoError(t, s.Delete(ctx, ref.Key))
}

func TestDownloadNotFound(t *testing.T) {
	s := newMinioStore(t)
	_, err := s.Download(t.Context(), "1111111111111111111111111111111111111111111111111111111111111111")
	assert.ErrorIs(t, err, storage.ErrBlobNotFound)
}

// The whole point of the S3 backend over the local one: real pre-signed URLs, so
// the client transfers bytes directly to/from S3 instead of proxying through the
// server. This round-trips a PUT and a GET entirely over pre-signed URLs.
func TestPresignedURLRoundTrip(t *testing.T) {
	s := newMinioStore(t)
	ctx := t.Context()
	key := "2222222222222222222222222222222222222222222222222222222222222222"
	data := []byte("uploaded via a pre-signed PUT, never touching the server")

	putURL, err := s.GenerateUploadURL(ctx, key, storage.SignOptions{ExpiresIn: 5 * time.Minute})
	require.NoError(t, err)
	require.NotEmpty(t, putURL)

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(data))
	require.NoError(t, err)
	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	require.NoError(t, putResp.Body.Close())
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// The server can see it...
	rc, err := s.Download(ctx, key)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, data, got)

	// ...and so can a client with only a pre-signed GET URL.
	getURL, err := s.GenerateDownloadURL(ctx, key, storage.SignOptions{})
	require.NoError(t, err)
	getResp, err := http.Get(getURL)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	assert.Equal(t, data, body)
}

// A pre-signed URL must carry the host the *browser* can reach, not the host the
// server uses to talk to the object store — the classic MinIO/LocalStack compose
// gotcha. PublicEndpoint controls the signed host; this needs no container
// because presigning is a local operation.
func TestPublicEndpointRewritesSignedHost(t *testing.T) {
	s, err := New(t.Context(), Options{
		Region:          "eu-north-1",
		Bucket:          testBucket,
		Endpoint:        "http://minio-internal:9000",
		PublicEndpoint:  "http://localhost:4566",
		UsePathStyle:    true,
		AccessKeyID:     testAccessKey,
		SecretAccessKey: testSecretKey,
	})
	require.NoError(t, err)

	url, err := s.GenerateDownloadURL(t.Context(), "deadbeef", storage.SignOptions{})
	require.NoError(t, err)
	assert.Contains(t, url, "localhost:4566", "signed URL must use the browser-reachable host")
	assert.NotContains(t, url, "minio-internal", "signed URL must not leak the internal endpoint")
}

func TestNewRequiresBucket(t *testing.T) {
	_, err := New(t.Context(), Options{Region: "eu-north-1"})
	assert.Error(t, err)
}
