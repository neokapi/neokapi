package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/bowrain/runtimedirs"
)

func TestRuntimeDirsNamesTheBlobDirectoryOnlyForLocalBlobs(t *testing.T) {
	t.Setenv("BLOB_STORAGE_BACKEND", "")
	t.Setenv("BLOB_STORAGE_LOCAL_DIR", "")
	temp := runtimedirs.Temp()

	s3 := RuntimeDirs(Config{BlobBackend: "s3"})
	assert.Equal(t, []runtimedirs.Dir{temp}, s3, "S3 blobs need only the temporary directory")

	local := RuntimeDirs(Config{BlobBackend: "local", BlobStorageLocalDir: "/data/blobs"})
	assert.Equal(t, []runtimedirs.Dir{temp, {Purpose: "blob", Env: "BLOB_STORAGE_LOCAL_DIR", Path: "/data/blobs"}}, local)

	// The defaults the local blob store itself uses.
	assert.Equal(t, filepath.Join("/srv", "blobs"), RuntimeDirs(Config{DataDir: "/srv"})[1].Path)
	assert.Equal(t, filepath.Join(os.TempDir(), "bowrain-blobs"), RuntimeDirs(Config{})[1].Path)
}
