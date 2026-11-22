package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
)

func TestComputeContextEntryHash_SourceOnlyChangesTheHash(t *testing.T) {
	plain := &pb.SyncContextEntry{Name: "build", Channel: "desktop"}
	only := &pb.SyncContextEntry{Name: "build", Channel: "desktop", SourceOnly: true}
	assert.NotEqual(t, ComputeContextEntryHash(plain), ComputeContextEntryHash(only))
}
