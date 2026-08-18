package source

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The upgrade case, from the client's side: no file in the repository changed,
// but this kapi's readers now record something the last push did not carry.
//
// The local cache holds record hashes, so the change is visible without asking
// the server anything — which matters, because a push whose scan finds nothing
// changed returns before it ever opens a connection. A transfer hash folded
// from source text alone would make this project unpushable: the text is the
// same forever, so the field would never arrive.
func TestConnectorDiff_ARecordedFieldIsWorkWithoutAFileEdit(t *testing.T) {
	const content = `{"greeting":"Hello","farewell":"Goodbye"}`
	proj, reg := newDiffTestProject(t, content)

	seedCacheWithHashes(t, proj, reg, func(b *model.Block) string {
		// What the previous kapi wrote: the content hash alone.
		return model.ComputeIdentity(b).ContentHash
	})

	conn := NewLocalConnector(&host.App{}, proj, reg)
	defer conn.Close()

	diff, err := conn.Diff(context.Background(), nil)
	require.NoError(t, err)

	assert.True(t, diff.HasChanges(),
		"an upgraded kapi has work to do even though the repository is untouched")
	assert.Equal(t, 2, diff.Changed)
}

// And the push after it is quiet again: the re-upload happens once, not on
// every push from then on.
func TestConnectorDiff_NothingToDoOnceTheFieldHasBeenPushed(t *testing.T) {
	const content = `{"greeting":"Hello","farewell":"Goodbye"}`
	proj, reg := newDiffTestProject(t, content)

	seedCacheWithHashes(t, proj, reg, func(b *model.Block) string {
		return model.ComputeIdentity(b).RecordHash()
	})

	conn := NewLocalConnector(&host.App{}, proj, reg)
	defer conn.Close()

	diff, err := conn.Diff(context.Background(), nil)
	require.NoError(t, err)

	assert.False(t, diff.HasChanges())
}

// seedCacheWithHashes writes the sync cache as a kapi of some vintage would
// have left it, hashing each scanned block with the given function.
func seedCacheWithHashes(t *testing.T, proj *bproject.Project, reg *registry.FormatRegistry, hash func(*model.Block) string) {
	t.Helper()

	conn := NewLocalConnector(&host.App{}, proj, reg)
	_, blockMap, err := conn.scanLocalBlocks(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, blockMap)

	cache := bproject.LoadSyncCache(proj.Layout)
	for itemName, blocks := range blockMap {
		fc := &bproject.FileCache{Blocks: map[string]string{}}
		for _, b := range blocks {
			fc.Blocks[convergence.BlockKey(b)] = hash(b)
		}
		cache.Files[itemName] = fc
	}
	require.NoError(t, cache.Save(proj.Layout))
}
