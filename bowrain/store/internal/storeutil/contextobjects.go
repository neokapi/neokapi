package storeutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/workspace"
)

// CheckContextDir refuses a directory the context layout does not have.
func CheckContextDir(dir string) error {
	switch dir {
	case workspace.RemoteLogDir, workspace.RemoteBlobDir, workspace.RemoteCheckpointsDir:
		return nil
	}
	return fmt.Errorf("%q is not a directory of the context layout", dir)
}

// CheckContextObjectName refuses a name outside the context layout.
func CheckContextObjectName(name string) error {
	if !workspace.ValidObjectName(name) {
		return fmt.Errorf("%q is not a name in the context layout", name)
	}
	return nil
}

// CheckContextObject refuses an object a put must not write: a name outside
// the context layout, or a blob whose name is not the address of its bytes.
// An object is created once, so a blob stored under the wrong bytes would
// refuse every later push of the right ones.
func CheckContextObject(name string, data []byte) error {
	if err := CheckContextObjectName(name); err != nil {
		return err
	}
	if strings.HasPrefix(name, workspace.RemoteBlobDir) &&
		workspace.BlobObjectName(workspace.BlobAddress(data)) != name {
		return fmt.Errorf("%q is not the address of the bytes it holds", name)
	}
	return nil
}

// ContextObjectDigest is the hex SHA-256 digest of an object's bytes.
func ContextObjectDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
