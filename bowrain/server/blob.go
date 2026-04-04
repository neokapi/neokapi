package server

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/storage/azureblob"
	"github.com/neokapi/neokapi/bowrain/storage/localblob"
)

// initBlobStore initializes the BlobStore based on server config.
func (s *Server) initBlobStore(cfg Config) {
	backend := cfg.BlobBackend
	if backend == "" {
		backend = os.Getenv("BLOB_STORAGE_BACKEND")
	}
	if backend == "" {
		backend = "local"
	}

	switch backend {
	case "azure":
		s.initAzureBlobStore(cfg)
	case "local":
		s.initLocalBlobStore(cfg)
	default:
		slog.Warn("unknown blob storage backend, falling back to local", "backend", backend)
		s.initLocalBlobStore(cfg)
	}
}

func (s *Server) initAzureBlobStore(cfg Config) {
	accountURL := cfg.AzureStorageAccountURL
	if accountURL == "" {
		accountURL = os.Getenv("AZURE_STORAGE_ACCOUNT_URL")
	}

	container := cfg.AzureStorageContainer
	if container == "" {
		container = os.Getenv("AZURE_STORAGE_CONTAINER")
	}
	if container == "" {
		container = "bowrain-assets"
	}

	connStr := cfg.AzureStorageConnStr
	if connStr == "" {
		connStr = os.Getenv("AZURE_STORAGE_CONNECTION_STRING")
	}

	// Prefer connection string (local dev / Azurite), fall back to Managed Identity.
	if connStr != "" {
		bs, err := azureblob.NewWithConnectionString(connStr, container)
		if err != nil {
			slog.Warn("failed to create Azure Blob Store from connection string", "error", err)
			return
		}
		s.BlobStore = bs
		slog.Info("using Azure Blob Storage (connection string)", "container", container)
		return
	}

	if accountURL != "" {
		bs, err := azureblob.New(accountURL, container)
		if err != nil {
			slog.Warn("failed to create Azure Blob Store", "error", err)
			return
		}
		s.BlobStore = bs
		slog.Info("using Azure Blob Storage (managed identity)", "account_url", accountURL, "container", container)
		return
	}

	slog.Warn("azure blob storage configured but no account URL or connection string provided")
}

func (s *Server) initLocalBlobStore(cfg Config) {
	dir := cfg.BlobStorageLocalDir
	if dir == "" {
		dir = os.Getenv("BLOB_STORAGE_LOCAL_DIR")
	}
	if dir == "" {
		// Default to DataDir/blobs or a temp location.
		if cfg.DataDir != "" {
			dir = filepath.Join(cfg.DataDir, "blobs")
		} else {
			dir = filepath.Join(os.TempDir(), "bowrain-blobs")
		}
	}

	bs, err := localblob.New(dir)
	if err != nil {
		slog.Warn("failed to create local blob store", "dir", dir, "error", err)
		return
	}
	s.BlobStore = bs
	slog.Info("using local blob storage", "dir", dir)
}
