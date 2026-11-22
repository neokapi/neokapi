package projector_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestStagedPushWritesNothingUntilConfirmed: a staged push computes the
// objects the remote lacks and writes none of them. Until it is confirmed the
// operations stay unpushed, so a push that never landed is staged again; once
// confirmed, the remote is known to hold them and the next stage is empty. A
// regular push reads the same staged objects.
func TestStagedPushWritesNothingUntilConfirmed(t *testing.T) {
	ctx := t.Context()
	remote := workspace.NewFileRemote(t.TempDir())
	opts := workspace.SyncOptions{LocalKinds: projector.LocalKinds, CheckpointEvery: -1}

	from, fromWS, _ := open(t)
	require.NoError(t, from.Rules().WidenRule(ctx, workspace.Rule{ID: "prj_docs\x00a", Kind: "term", Origin: key, Payload: []byte(`{"a":1}`)}))
	sync := fromWS.NewSync(remote, key, from.Syncer(), opts)

	staged, err := sync.StagePush(ctx)
	require.NoError(t, err)
	require.NotNil(t, staged)
	assert.Positive(t, staged.Pushed)
	assert.Equal(t, 1, staged.Segments)
	require.NotEmpty(t, staged.Objects)
	last := staged.Objects[len(staged.Objects)-1]
	assert.True(t, strings.HasPrefix(last.Name, workspace.RemoteLogDir), "segments come after the blobs they name")

	held, err := remote.List(ctx, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Empty(t, held, "staging writes nothing to the remote")

	again, err := sync.StagePush(ctx)
	require.NoError(t, err)
	require.NotNil(t, again, "an unconfirmed push is staged again")
	assert.Equal(t, staged.Objects, again.Objects, "with the same objects")

	// The caller writes the objects (here, to the remote itself) and confirms.
	require.NoError(t, remote.Put(ctx, staged.Objects...))
	require.NoError(t, staged.Confirm(ctx))

	after, err := sync.StagePush(ctx)
	require.NoError(t, err)
	assert.Nil(t, after, "a confirmed push leaves nothing to stage")

	st, err := sync.Status(ctx)
	require.NoError(t, err)
	assert.Zero(t, st.ToPush)

	// Another machine pulls what the staged push carried.
	to, toWS, _ := open(t)
	_, err = toWS.NewSync(remote, key, to.Syncer(), opts).Pull(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"prj_docs\x00a"}, ruleIDs(t, toWS))
}
