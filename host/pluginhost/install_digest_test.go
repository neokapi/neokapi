package pluginhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The install record names the archive by the sha256 of its bytes.
func TestArchiveDigest(t *testing.T) {
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", archiveDigest([]byte("abc")))
}
