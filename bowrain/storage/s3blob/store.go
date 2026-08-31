// Package s3blob provides an Amazon S3 (and S3-compatible: MinIO, LocalStack)
// implementation of storage.BlobStore.
//
// Credentials come from the default AWS chain — the ECS/EKS task role in
// production, static keys from the environment for local dev and tests — so no
// secret is ever passed through config. An optional endpoint override plus
// path-style addressing lets the *same* code target MinIO or LocalStack in
// docker-compose, which is what gives local/prod parity: identical code path,
// only the endpoint and credentials differ.
//
// Blobs are content-addressed (SHA-256) and laid out with git-like sharding
// {prefix}/{key[0:2]}/{key[2:4]}/{key}, matching the local and Azure backends so
// a bucket populated by one is readable by another.
//
// Pre-signed PUT/GET URLs are real here (unlike the local filesystem backend),
// so the asset upload/download path goes directly to S3 and never proxies bytes
// through the server. When the server's internal S3 endpoint differs from the
// host a browser can reach (MinIO/LocalStack in compose), PublicEndpoint signs
// URLs against the browser-reachable host.
package s3blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	"github.com/neokapi/neokapi/core/storage"
)

// Store implements storage.BlobStore using Amazon S3 or an S3-compatible service.
type Store struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	prefix        string
}

// Options configures an S3 blob store.
type Options struct {
	Region string // AWS region, e.g. "eu-north-1"
	Bucket string // target bucket (required)
	Prefix string // key prefix within the bucket (default "blobs")

	// Endpoint overrides the S3 endpoint the server talks to. Empty in
	// production (the SDK resolves the regional endpoint); set to e.g.
	// "http://localstack:4566" or "http://minio:9000" for compose.
	Endpoint string

	// PublicEndpoint is the S3 host used when signing URLs, i.e. the host a
	// browser or CLI can reach. Empty in production (presigned URLs already
	// carry the public regional host). In compose set it to the host the
	// browser uses, e.g. "http://localhost:4566", so a URL signed by the
	// server (which uses the in-network Endpoint) still resolves client-side.
	PublicEndpoint string

	// UsePathStyle forces path-style addressing (bucket in the path, not the
	// host). Required by MinIO/LocalStack; leave false for real AWS.
	UsePathStyle bool

	// Static credentials for local dev / tests. Empty in production, where the
	// default chain resolves the task role.
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// New constructs an S3 blob store. It validates that a bucket is configured but
// does not verify connectivity — the first Upload/Download surfaces auth or
// endpoint errors.
func New(ctx context.Context, opts Options) (*Store, error) {
	if opts.Bucket == "" {
		return nil, errors.New("s3blob: bucket is required")
	}
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "blobs"
	}

	loadOpts := []func(*config.LoadOptions) error{}
	if opts.Region != "" {
		loadOpts = append(loadOpts, config.WithRegion(opts.Region))
	}
	if opts.AccessKeyID != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.SecretAccessKey, opts.SessionToken)))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3blob: load aws config: %w", err)
	}

	applyEndpoint := func(endpoint string) func(*s3.Options) {
		return func(o *s3.Options) {
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
			o.UsePathStyle = opts.UsePathStyle
		}
	}

	client := s3.NewFromConfig(awsCfg, applyEndpoint(opts.Endpoint))

	// Sign URLs against the browser-reachable host when it differs from the
	// server's internal endpoint (compose). In production both are empty and
	// the presign client is the same as the main client.
	presignSource := client
	if opts.PublicEndpoint != "" {
		presignSource = s3.NewFromConfig(awsCfg, applyEndpoint(opts.PublicEndpoint))
	}

	return &Store{
		client:        client,
		presignClient: s3.NewPresignClient(presignSource),
		bucket:        opts.Bucket,
		prefix:        prefix,
	}, nil
}

// objectKey maps a content-addressed key to its sharded S3 object key.
func (s *Store) objectKey(key string) string {
	if len(key) < 4 {
		return path.Join(s.prefix, key)
	}
	return path.Join(s.prefix, key[0:2], key[2:4], key)
}

// Upload stores binary content and returns a content-addressed BlobRef.
// If a blob with the same SHA-256 already exists, the existing ref is returned
// without re-uploading.
func (s *Store) Upload(ctx context.Context, data []byte, opts storage.UploadOptions) (*storage.BlobRef, error) {
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])

	exists, err := s.Exists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return &storage.BlobRef{Key: key, Size: int64(len(data)), ContentType: opts.ContentType}, nil
	}

	in := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s.objectKey(key)),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}
	if opts.ContentType != "" {
		in.ContentType = aws.String(opts.ContentType)
	}
	if _, err := s.client.PutObject(ctx, in); err != nil {
		return nil, fmt.Errorf("s3blob: put object: %w", err)
	}

	return &storage.BlobRef{Key: key, Size: int64(len(data)), ContentType: opts.ContentType}, nil
}

// Download retrieves binary content by storage key. The caller must close the
// returned reader.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, storage.ErrBlobNotFound
		}
		return nil, fmt.Errorf("s3blob: get object: %w", err)
	}
	return out.Body, nil
}

// GenerateUploadURL returns a pre-signed PUT URL for direct client upload.
func (s *Store) GenerateUploadURL(ctx context.Context, key string, opts storage.SignOptions) (string, error) {
	req, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	}, s3.WithPresignExpires(signExpiry(opts)))
	if err != nil {
		return "", fmt.Errorf("s3blob: presign put: %w", err)
	}
	return req.URL, nil
}

// GenerateDownloadURL returns a pre-signed GET URL for direct client download.
func (s *Store) GenerateDownloadURL(ctx context.Context, key string, opts storage.SignOptions) (string, error) {
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	}, s3.WithPresignExpires(signExpiry(opts)))
	if err != nil {
		return "", fmt.Errorf("s3blob: presign get: %w", err)
	}
	return req.URL, nil
}

// Delete removes a blob by storage key. Deleting a missing key is not an error.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("s3blob: delete object: %w", err)
	}
	return nil
}

// Exists reports whether a blob with the given key exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("s3blob: head object: %w", err)
	}
	return true, nil
}

func signExpiry(opts storage.SignOptions) time.Duration {
	if opts.ExpiresIn <= 0 {
		return time.Hour
	}
	return opts.ExpiresIn
}

// isNotFound reports whether an S3 error means the object (or bucket key) was
// absent. HeadObject surfaces a bare *types.NotFound; GetObject a *types.NoSuchKey;
// some S3-compatible servers only set the smithy API error code.
func isNotFound(err error) bool {
	var nsk *s3types.NoSuchKey
	var nf *s3types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return true
	}
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

// Store is a plain BlobStore, not a ChunkedBlobStore: the sync push path uploads
// each ≤2 MiB chunk as its own content-addressed object (S3 multipart requires
// ≥5 MiB parts, so per-chunk multipart is not applicable), and the only
// ChunkedBlobStore method the server actually calls, GenerateChunkUploadURLs, is
// tied to a per-chunk-hash "direct" transport that is a separate follow-up.
var _ storage.BlobStore = (*Store)(nil)
