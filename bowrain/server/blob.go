package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/storage/blobcfg"
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
	case "s3":
		s.initS3BlobStore()
	case "local":
		s.initLocalBlobStore(cfg)
	default:
		slog.Warn("unknown blob storage backend, falling back to local", "backend", backend)
		s.initLocalBlobStore(cfg)
	}
}

// initS3BlobStore initializes the S3 (or S3-compatible) blob store from the
// shared env contract. Like the other backends it degrades gracefully — a
// misconfiguration warns rather than aborting startup.
func (s *Server) initS3BlobStore() {
	bs, err := blobcfg.NewS3FromEnv(context.Background())
	if err != nil {
		slog.Warn("failed to create S3 blob store", "error", err)
		return
	}
	s.BlobStore = bs
	slog.Info("using S3 blob storage", "bucket", os.Getenv("S3_BLOB_BUCKET"), "endpoint", os.Getenv("S3_ENDPOINT"))
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
