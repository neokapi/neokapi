package storeutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

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

// ContextObjectDigest is the hex SHA-256 digest of an object's bytes.
func ContextObjectDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
