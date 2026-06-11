// Package azureblob provides an Azure Blob Storage implementation of BlobStore.
// It uses Azure Managed Identity (DefaultAzureCredential) for production and
// connection string fallback for local development.
//
// Blob naming convention: {key[0:2]}/{key[2:4]}/{key} with git-like sharding
// to avoid directory listing bottlenecks.
package azureblob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/neokapi/neokapi/core/storage"
)

// Store implements storage.BlobStore using Azure Blob Storage.
type Store struct {
	client        *azblob.Client
	serviceClient *service.Client
	containerName string
}

// New creates an Azure Blob Store using DefaultAzureCredential (production).
func New(accountURL, containerName string) (*Store, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure identity: %w", err)
	}
	client, err := azblob.NewClient(accountURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure blob client: %w", err)
	}
	svcClient, err := service.NewClient(accountURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure service client: %w", err)
	}
	return &Store{
		client:        client,
		serviceClient: svcClient,
		containerName: containerName,
	}, nil
}

// NewWithConnectionString creates an Azure Blob Store for local development
// (e.g., Azurite emulator).
func NewWithConnectionString(connStr, containerName string) (*Store, error) {
	client, err := azblob.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, fmt.Errorf("azure blob client: %w", err)
	}
	svcClient, err := service.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, fmt.Errorf("azure service client: %w", err)
	}
	return &Store{
		client:        client,
		serviceClient: svcClient,
		containerName: containerName,
	}, nil
}

func blobName(key string) string {
	if len(key) < 4 {
		return key
	}
	return key[0:2] + "/" + key[2:4] + "/" + key
}

// Upload stores binary content and returns a content-addressed BlobRef.
func (s *Store) Upload(ctx context.Context, data []byte, opts storage.UploadOptions) (*storage.BlobRef, error) {
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])
	name := blobName(key)

	// Check for dedup.
	exists, err := s.Exists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return &storage.BlobRef{
			Key:         key,
			Size:        int64(len(data)),
			ContentType: opts.ContentType,
		}, nil
	}

	uploadOpts := &blockblob.UploadOptions{}
	if opts.ContentType != "" {
		uploadOpts.HTTPHeaders = &blob.HTTPHeaders{
			BlobContentType: &opts.ContentType,
		}
	}

	_, err = s.client.UploadBuffer(ctx, s.containerName, name, data, &azblob.UploadBufferOptions{
		HTTPHeaders: uploadOpts.HTTPHeaders,
	})
	if err != nil {
		return nil, fmt.Errorf("upload blob: %w", err)
	}

	return &storage.BlobRef{
		Key:         key,
		Size:        int64(len(data)),
		ContentType: opts.ContentType,
	}, nil
}

// Download retrieves binary content by storage key.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.client.DownloadStream(ctx, s.containerName, blobName(key), nil)
	if err != nil {
		return nil, fmt.Errorf("download blob: %w", err)
	}
	return resp.Body, nil
}

// GenerateUploadURL returns a SAS URL for direct client upload.
func (s *Store) GenerateUploadURL(ctx context.Context, key string, opts storage.SignOptions) (string, error) {
	return s.generateSASURL(ctx, key, opts, sas.BlobPermissions{Write: true, Create: true})
}

// GenerateDownloadURL returns a SAS URL for direct client download.
func (s *Store) GenerateDownloadURL(ctx context.Context, key string, opts storage.SignOptions) (string, error) {
	return s.generateSASURL(ctx, key, opts, sas.BlobPermissions{Read: true})
}

func (s *Store) generateSASURL(ctx context.Context, key string, opts storage.SignOptions, perms sas.BlobPermissions) (string, error) {
	expiry := opts.ExpiresIn
	if expiry == 0 {
		expiry = time.Hour
	}

	now := time.Now().UTC()

	// Try user delegation key first (Managed Identity — no storage account key needed).
	info := service.KeyInfo{
		Start:  new(now.Format(sas.TimeFormat)),
		Expiry: new(now.Add(expiry).Format(sas.TimeFormat)),
	}
	udk, err := s.serviceClient.GetUserDelegationCredential(ctx, info, nil)
	if err != nil {
		// Fall back to connection-string SAS if user delegation is unavailable
		// (e.g., Azurite or storage account key auth).
		return s.generateAccountSASURL(key, expiry, perms)
	}

	sasQueryParams, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     now.Add(-5 * time.Minute),
		ExpiryTime:    now.Add(expiry),
		Permissions:   perms.String(),
		ContainerName: s.containerName,
		BlobName:      blobName(key),
	}.SignWithUserDelegation(udk)
	if err != nil {
		return "", fmt.Errorf("sign SAS: %w", err)
	}

	blobClient := s.client.ServiceClient().NewContainerClient(s.containerName).NewBlobClient(blobName(key))
	return fmt.Sprintf("%s?%s", blobClient.URL(), sasQueryParams.Encode()), nil
}

func (s *Store) generateAccountSASURL(key string, expiry time.Duration, perms sas.BlobPermissions) (string, error) {
	// For connection-string based clients (local dev / Azurite), we generate
	// SAS via the blob client's built-in method.
	blobClient := s.client.ServiceClient().NewContainerClient(s.containerName).NewBlockBlobClient(blobName(key))

	now := time.Now().UTC()
	sasURL, err := blobClient.GetSASURL(perms, now.Add(expiry), &blob.GetSASURLOptions{
		StartTime: new(now.Add(-5 * time.Minute)),
	})
	if err != nil {
		return "", fmt.Errorf("generate account SAS: %w", err)
	}
	return sasURL, nil
}

// Delete removes a blob by storage key.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteBlob(ctx, s.containerName, blobName(key), nil)
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// Exists checks whether a blob with the given key exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.ServiceClient().NewContainerClient(s.containerName).NewBlobClient(blobName(key)).GetProperties(ctx, nil)
	if err != nil {
		if isBlobNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("check blob existence: %w", err)
	}
	return true, nil
}

// isBlobNotFound checks if an Azure error indicates a 404 Not Found.
func isBlobNotFound(err error) bool {
	// Azure SDK errors embed the status code; check for BlobNotFound code.
	return bytes.Contains([]byte(err.Error()), []byte("BlobNotFound")) ||
		bytes.Contains([]byte(err.Error()), []byte("404"))
}

// readSeekNopCloser wraps an io.ReadSeeker with a no-op Close.
type readSeekNopCloser struct {
	io.ReadSeeker
}

func (readSeekNopCloser) Close() error { return nil }

// ---------------------------------------------------------------------------
// ChunkedBlobStore implementation (Bowrain AD-009)
// ---------------------------------------------------------------------------

// InitUpload prepares a chunked upload session. For Azure Block Blobs,
// this is a no-op — blocks can be staged at any time.
func (s *Store) InitUpload(_ context.Context, uploadKey string) (string, error) {
	return uploadKey, nil
}

// StageChunk uploads one chunk as a Block Blob block.
func (s *Store) StageChunk(ctx context.Context, uploadID string, chunkIndex int, data []byte) error {
	bName := blobName(uploadID)
	blockID := fmt.Sprintf("%08d", chunkIndex)
	encodedID := hex.EncodeToString([]byte(blockID))

	bbClient := s.client.ServiceClient().NewContainerClient(s.containerName).NewBlockBlobClient(bName)
	_, err := bbClient.StageBlock(ctx, encodedID, readSeekNopCloser{bytes.NewReader(data)}, nil)
	if err != nil {
		return fmt.Errorf("stage block %d: %w", chunkIndex, err)
	}
	return nil
}

// CommitUpload finalizes the chunked upload by committing all staged blocks.
func (s *Store) CommitUpload(ctx context.Context, uploadID string, totalChunks int) error {
	bName := blobName(uploadID)

	blockIDs := make([]string, totalChunks)
	for i := range totalChunks {
		blockID := fmt.Sprintf("%08d", i)
		blockIDs[i] = hex.EncodeToString([]byte(blockID))
	}

	bbClient := s.client.ServiceClient().NewContainerClient(s.containerName).NewBlockBlobClient(bName)
	_, err := bbClient.CommitBlockList(ctx, blockIDs, nil)
	if err != nil {
		return fmt.Errorf("commit block list: %w", err)
	}
	return nil
}

// AbortUpload is a no-op for Azure — uncommitted blocks expire automatically after 7 days.
func (s *Store) AbortUpload(_ context.Context, _ string) error {
	return nil
}

// GenerateChunkUploadURLs returns SAS URLs for direct client upload of each chunk.
func (s *Store) GenerateChunkUploadURLs(ctx context.Context, uploadID string, chunkCount int, opts storage.SignOptions) ([]string, error) {
	if s.serviceClient == nil {
		return nil, storage.ErrNotSupported
	}

	expiry := opts.ExpiresIn
	if expiry == 0 {
		expiry = 1 * time.Hour
	}

	// Generate a user delegation key for SAS tokens.
	now := time.Now().UTC()
	info := service.KeyInfo{
		Start:  new(now.Format(sas.TimeFormat)),
		Expiry: new(now.Add(expiry).Format(sas.TimeFormat)),
	}
	udk, err := s.serviceClient.GetUserDelegationCredential(ctx, info, nil)
	if err != nil {
		return nil, fmt.Errorf("user delegation key: %w", err)
	}

	urls := make([]string, chunkCount)
	bName := blobName(uploadID)

	for i := range chunkCount {
		sasValues := sas.BlobSignatureValues{
			Protocol:      sas.ProtocolHTTPS,
			StartTime:     now,
			ExpiryTime:    now.Add(expiry),
			Permissions:   new(sas.BlobPermissions{Write: true, Create: true}).String(),
			ContainerName: s.containerName,
			BlobName:      bName,
		}
		token, err := sasValues.SignWithUserDelegation(udk)
		if err != nil {
			return nil, fmt.Errorf("sign chunk %d SAS: %w", i, err)
		}
		urls[i] = fmt.Sprintf("%s/%s/%s?%s",
			s.client.URL(), s.containerName, bName, token.Encode())
	}

	return urls, nil
}
