// Package blobcfg centralizes how the server and worker resolve their blob
// backend from the environment, so the two can never drift (the historical
// LOCAL_BLOB_DIR vs BLOB_STORAGE_LOCAL_DIR split is exactly the kind of bug this
// prevents). It is the single source of truth for the S3 env contract that
// Terraform and docker-compose both target.
package blobcfg

import (
	"context"
	"errors"
	"os"

	"github.com/neokapi/neokapi/bowrain/storage/s3blob"
)

// S3 environment contract (server, worker, Terraform, and compose all use these
// exact names):
//
//	S3_BLOB_BUCKET        target bucket (required to select the s3 backend)
//	S3_BLOB_PREFIX        key prefix within the bucket (default "blobs")
//	AWS_REGION            AWS region (also read natively by the SDK)
//	S3_ENDPOINT           endpoint override for MinIO/LocalStack (empty on AWS)
//	S3_PUBLIC_ENDPOINT    browser-reachable host for pre-signed URLs (empty on AWS)
//	S3_FORCE_PATH_STYLE   "true" for MinIO/LocalStack, unset for AWS
//
// Credentials come from the standard AWS chain (AWS_ACCESS_KEY_ID/…, or the ECS
// task role in production) — never plumbed through here.

// S3Configured reports whether the environment selects the S3 backend, either
// explicitly (BLOB_STORAGE_BACKEND=s3) or implicitly (a bucket is set).
func S3Configured() bool {
	return os.Getenv("BLOB_STORAGE_BACKEND") == "s3" || os.Getenv("S3_BLOB_BUCKET") != ""
}

// NewS3FromEnv builds an S3 blob store from the environment contract above.
func NewS3FromEnv(ctx context.Context) (*s3blob.Store, error) {
	bucket := os.Getenv("S3_BLOB_BUCKET")
	if bucket == "" {
		return nil, errors.New("blobcfg: S3_BLOB_BUCKET is required for the s3 backend")
	}
	return s3blob.New(ctx, s3blob.Options{
		Region:         os.Getenv("AWS_REGION"),
		Bucket:         bucket,
		Prefix:         os.Getenv("S3_BLOB_PREFIX"),
		Endpoint:       os.Getenv("S3_ENDPOINT"),
		PublicEndpoint: os.Getenv("S3_PUBLIC_ENDPOINT"),
		UsePathStyle:   os.Getenv("S3_FORCE_PATH_STYLE") == "true",
	})
}
