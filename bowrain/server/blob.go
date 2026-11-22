package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/runtimedirs"
	"github.com/neokapi/neokapi/bowrain/storage/blobcfg"
	"github.com/neokapi/neokapi/bowrain/storage/localblob"
)

// blobBackend is the blob storage backend cfg selects: the config, then
// BLOB_STORAGE_BACKEND, then "local".
func blobBackend(cfg Config) string {
	backend := cfg.BlobBackend
	if backend == "" {
		backend = os.Getenv("BLOB_STORAGE_BACKEND")
	}
	if backend == "" {
		backend = "local"
	}
	return backend
}

// localBlobDir is the directory the local blob backend writes for cfg: the
// config, then BLOB_STORAGE_LOCAL_DIR, then DataDir/blobs, then a directory
// under the system temporary directory.
func localBlobDir(cfg Config) string {
	dir := cfg.BlobStorageLocalDir
	if dir == "" {
		dir = os.Getenv("BLOB_STORAGE_LOCAL_DIR")
	}
	if dir == "" {
		if cfg.DataDir != "" {
			dir = filepath.Join(cfg.DataDir, "blobs")
		} else {
			dir = filepath.Join(os.TempDir(), "bowrain-blobs")
		}
	}
	return dir
}

// RuntimeDirs names the directories a server with cfg writes at run time: the
// temporary directory always, and the blob directory when blobs are stored
// locally. Every other backend keeps its state outside the container.
func RuntimeDirs(cfg Config) []runtimedirs.Dir {
	dirs := []runtimedirs.Dir{runtimedirs.Temp()}
	if blobBackend(cfg) != "s3" {
		dirs = append(dirs, runtimedirs.Dir{Purpose: "blob", Env: "BLOB_STORAGE_LOCAL_DIR", Path: localBlobDir(cfg)})
	}
	return dirs
}

// initBlobStore initializes the BlobStore based on server config.
func (s *Server) initBlobStore(cfg Config) {
	backend := blobBackend(cfg)

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
// shared env contract. Like the other backends it degrades gracefully: a
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
	dir := localBlobDir(cfg)

	bs, err := localblob.New(dir)
	if err != nil {
		slog.Warn("failed to create local blob store", "dir", dir, "error", err)
		return
	}
	s.BlobStore = bs
	slog.Info("using local blob storage", "dir", dir)
}
