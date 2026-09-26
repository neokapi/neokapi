package workspacetest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

// RemoteFactory prepares a fresh, empty remote for one test and returns a way
// to open a handle on it. Each call of the opener is another machine: a
// second clone of the repository, another mount of the share, another client
// of the bucket.
type RemoteFactory func(t *testing.T) func(t *testing.T) workspace.Remote

// RunRemoteConformance drives every behaviour a remote owes the sync engine,
// and then the sync engine itself over two workspaces sharing the remote.
func RunRemoteConformance(t *testing.T, newRemote RemoteFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T) workspace.Remote)
	}{
		{"describes itself", remoteDescribesItself},
		{"an empty remote holds nothing", emptyRemoteHoldsNothing},
		{"objects read back and list by directory", objectsReadBack},
		{"an object is created once", objectIsCreatedOnce},
		{"a name outside the layout is refused", nameOutsideLayoutIsRefused},
		{"another handle sees what one wrote", anotherHandleSees},
		{"two handles writing at once both land", twoHandlesWriteAtOnce},
		{"a pushed log pulls into another workspace", pushedLogPulls},
		{"operations that stay here never travel", localKindsStay},
		{"an interrupted push is not written twice", interruptedPushIsNotRepeated},
		{"an operation older than the applied ones asks for a rebuild", olderOperationRebuilds},
		{"a first pull starts from the newest checkpoint", firstPullUsesCheckpoint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newRemote(t))
		})
	}
}

func openRemote(t *testing.T, open func(t *testing.T) workspace.Remote) workspace.Remote {
	r := open(t)
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// segmentNamed builds a valid segment name for a writer and an id.
func segmentNamed(writer string, n int) string {
	return workspace.SegmentName(writer, fmt.Sprintf("0000000000000000000000%02d", n))
}

func remoteDescribesItself(t *testing.T, open func(t *testing.T) workspace.Remote) {
	d := openRemote(t, open).Describe()
	assert.NotEmpty(t, d.Kind)
	assert.NotEmpty(t, d.Location)
}

func emptyRemoteHoldsNothing(t *testing.T, open func(t *testing.T) workspace.Remote) {
	r := openRemote(t, open)
	ctx := t.Context()
	for _, dir := range []string{workspace.RemoteLogDir, workspace.RemoteBlobDir, workspace.RemoteCheckpointsDir} {
		names, err := r.List(ctx, dir)
		require.NoError(t, err)
		assert.Empty(t, names)
	}
	_, err := r.Get(ctx, segmentNamed("w1", 1))
	require.ErrorIs(t, err, workspace.ErrNoObject)
}

func objectsReadBack(t *testing.T, open func(t *testing.T) workspace.Remote) {
	r := openRemote(t, open)
	ctx := t.Context()
	blob := []byte("payload")
	blobName := workspace.BlobObjectName(workspace.BlobAddress(blob))
	require.NoError(t, r.Put(ctx,
		workspace.Object{Name: blobName, Data: blob},
		workspace.Object{Name: segmentNamed("w2", 2), Data: []byte("two\n")},
		workspace.Object{Name: segmentNamed("w1", 1), Data: []byte("one\n")},
	))
	logs, err := r.List(ctx, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Equal(t, []string{segmentNamed("w1", 1), segmentNamed("w2", 2)}, logs, "names list sorted, one directory at a time")
	blobs, err := r.List(ctx, workspace.RemoteBlobDir)
	require.NoError(t, err)
	assert.Equal(t, []string{blobName}, blobs)

	got, err := r.Get(ctx, segmentNamed("w1", 1))
	require.NoError(t, err)
	assert.Equal(t, "one\n", string(got))
	got, err = r.Get(ctx, blobName)
	require.NoError(t, err)
	assert.Equal(t, blob, got)
}

func objectIsCreatedOnce(t *testing.T, open func(t *testing.T) workspace.Remote) {
	r := openRemote(t, open)
	ctx := t.Context()
	name := segmentNamed("w1", 1)
	require.NoError(t, r.Put(ctx, workspace.Object{Name: name, Data: []byte("first\n")}))
	require.NoError(t, r.Put(ctx, workspace.Object{Name: name, Data: []byte("first\n")}),
		"writing the bytes an object already holds changes nothing")
	err := r.Put(ctx, workspace.Object{Name: name, Data: []byte("second\n")})
	require.ErrorIs(t, err, workspace.ErrObjectExists, "an object is never replaced")
	got, err := r.Get(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, "first\n", string(got))
}

func nameOutsideLayoutIsRefused(t *testing.T, open func(t *testing.T) workspace.Remote) {
	r := openRemote(t, open)
	ctx := t.Context()
	for _, name := range []string{"../escape", "log/w1/../../x.jsonl", "notes.txt", "log/w1/short.jsonl", "blobs/XYZ"} {
		require.Error(t, r.Put(ctx, workspace.Object{Name: name, Data: []byte("x")}), name)
		_, err := r.Get(ctx, name)
		require.Error(t, err, name)
	}
	_, err := r.List(ctx, "other/")
	assert.Error(t, err)
}

func anotherHandleSees(t *testing.T, open func(t *testing.T) workspace.Remote) {
	a, b := openRemote(t, open), openRemote(t, open)
	ctx := t.Context()
	require.NoError(t, a.Put(ctx, workspace.Object{Name: segmentNamed("wa", 1), Data: []byte("a\n")}))
	got, err := b.Get(ctx, segmentNamed("wa", 1))
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(got))
	require.NoError(t, b.Put(ctx, workspace.Object{Name: segmentNamed("wb", 2), Data: []byte("b\n")}))
	c := openRemote(t, open)
	names, err := c.List(ctx, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Equal(t, []string{segmentNamed("wa", 1), segmentNamed("wb", 2)}, names)
}

func twoHandlesWriteAtOnce(t *testing.T, open func(t *testing.T) workspace.Remote) {
	a, b := openRemote(t, open), openRemote(t, open)
	ctx := t.Context()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, r := range []workspace.Remote{a, b} {
		wg.Go(func() {
			for n := range 3 {
				if err := r.Put(ctx, workspace.Object{Name: segmentNamed(fmt.Sprintf("w%d", i), n), Data: []byte("x\n")}); err != nil {
					errs[i] = err
					return
				}
			}
		})
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	names, err := openRemote(t, open).List(ctx, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Len(t, names, 6, "no write is lost to the other")
}

// testApplier records what the sync engine asked of it. A checkpoint is the
// JSON of its mark, which is all the engine reads.
type testApplier struct {
	mu        sync.Mutex
	applied   int
	rebuilds  int
	installed []string
	through   string
}

type testMark struct {
	Through  string   `json:"through"`
	Segments []string `json:"segments"`
}

func (a *testApplier) Apply(_ context.Context, rebuild bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applied++
	if rebuild {
		a.rebuilds++
	}
	return nil
}

func (a *testApplier) Checkpoint(_ context.Context, segments []string) ([]byte, string, error) {
	data, err := json.Marshal(testMark{Through: a.through, Segments: segments})
	return data, a.through, err
}

func (a *testApplier) CheckpointMark(data []byte) (string, []string, error) {
	var m testMark
	err := json.Unmarshal(data, &m)
	return m.Through, m.Segments, err
}

func (a *testApplier) InstallCheckpoint(_ context.Context, data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.installed = append(a.installed, string(data))
	return nil
}

var testLocalKinds = []string{workspace.OpRegisterProject, "rules.write", "checkpoint.write"}

func newTestWorkspace(t *testing.T) *workspace.Workspace {
	w, err := workspace.OpenLocal(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func pushedLogPulls(t *testing.T, open func(t *testing.T) workspace.Remote) {
	ctx := t.Context()
	a, b := newTestWorkspace(t), newTestWorkspace(t)
	blob := []byte(strings.Repeat("rows ", 100))
	address, err := a.PutBlob(ctx, blob)
	require.NoError(t, err)
	recorded, err := a.Record(ctx,
		workspace.Op{Project: "prj", Kind: "terms.write", Payload: []byte(`{"blob":"` + address + `"}`)},
		workspace.Op{Project: "prj", Kind: "context.observe", Payload: []byte(`{"text":"x"}`), Address: "obs:1"},
		workspace.Op{Project: "other", Kind: "context.observe"},
	)
	require.NoError(t, err)

	appA, appB := &testApplier{}, &testApplier{}
	syncA := a.NewSync(openRemote(t, open), "prj", appA, workspace.SyncOptions{LocalKinds: testLocalKinds})
	syncB := b.NewSync(openRemote(t, open), "prj", appB, workspace.SyncOptions{LocalKinds: testLocalKinds})

	st, err := syncA.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, st.ToPush, "only the project's operations are counted")

	before, err := syncB.Pull(ctx)
	require.NoError(t, err)
	assert.True(t, before.Empty, "a remote nothing was pushed to says so")
	assert.Zero(t, before.Merged)

	pushed, err := syncA.Push(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, pushed.Pushed)
	assert.Equal(t, 1, pushed.Segments)
	assert.Equal(t, 1, pushed.Blobs)
	assert.Zero(t, pushed.ToPush)
	assert.False(t, pushed.Contacted.IsZero())

	again, err := syncA.Push(ctx)
	require.NoError(t, err)
	assert.Zero(t, again.Pushed, "a second push has nothing to write")

	fetched, err := syncB.Fetch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, fetched.ToPull, "a fetch counts what a pull would merge")

	pulled, err := syncB.Pull(ctx)
	require.NoError(t, err)
	assert.False(t, pulled.Empty)
	assert.Equal(t, 2, pulled.Merged)
	assert.Zero(t, pulled.ToPull)
	assert.Zero(t, pulled.ToPush, "what was pulled is not pushed back")
	assert.Equal(t, 1, appB.applied)

	held, err := b.Select(ctx, workspace.OpQuery{Project: "prj"})
	require.NoError(t, err)
	require.Len(t, held, 2)
	assert.Equal(t, recorded[0].ID, held[0].ID, "operations keep their ids")
	assert.Equal(t, recorded[0].At, held[0].At)
	got, err := b.Blob(ctx, address)
	require.NoError(t, err, "the blob an operation names travels with it")
	assert.Equal(t, blob, got)

	// B's own operation goes back the other way.
	_, err = b.Record(ctx, workspace.Op{Project: "prj", Kind: "context.keep", Payload: []byte(`{}`)})
	require.NoError(t, err)
	_, err = syncB.Push(ctx)
	require.NoError(t, err)
	back, err := syncA.Pull(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, back.Merged)
	assert.False(t, back.Rebuilt, "an operation after every applied one is caught up, not rebuilt")

	nothing, err := syncA.Pull(ctx)
	require.NoError(t, err)
	assert.Zero(t, nothing.Merged)
	assert.False(t, nothing.Empty, "a caught-up pull from a remote that holds a log is not empty")
}

func localKindsStay(t *testing.T, open func(t *testing.T) workspace.Remote) {
	ctx := t.Context()
	a, b := newTestWorkspace(t), newTestWorkspace(t)
	_, err := a.Register(ctx, "prj", "Docs", "/fakehome/src/docs")
	require.NoError(t, err)
	_, err = a.Record(ctx,
		workspace.Op{Project: "prj", Kind: "rules.write", Payload: []byte(`{}`)},
		workspace.Op{Project: "prj", Kind: "context.keep", Payload: []byte(`{}`)})
	require.NoError(t, err)
	syncA := a.NewSync(openRemote(t, open), "prj", &testApplier{}, workspace.SyncOptions{LocalKinds: testLocalKinds})
	pushed, err := syncA.Push(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, pushed.Pushed, "the registration and the widened rule stay here")

	syncB := b.NewSync(openRemote(t, open), "prj", &testApplier{}, workspace.SyncOptions{LocalKinds: testLocalKinds})
	_, err = syncB.Pull(ctx)
	require.NoError(t, err)
	held, err := b.Select(ctx, workspace.OpQuery{Project: "prj"})
	require.NoError(t, err)
	assert.Equal(t, []string{"context.keep"}, kinds(held))
}

func interruptedPushIsNotRepeated(t *testing.T, open func(t *testing.T) workspace.Remote) {
	ctx := t.Context()
	a := newTestWorkspace(t)
	_, err := a.Record(ctx, workspace.Op{Project: "prj", Kind: "context.keep", Payload: []byte(`{}`)})
	require.NoError(t, err)
	remote := openRemote(t, open)
	s := a.NewSync(remote, "prj", &testApplier{}, workspace.SyncOptions{LocalKinds: testLocalKinds})
	_, err = s.Push(ctx)
	require.NoError(t, err)
	// The segment landed and the workspace lost its record of it.
	require.NoError(t, s.Forget(ctx))
	st, err := s.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, st.ToPush)
	again, err := s.Push(ctx)
	require.NoError(t, err)
	assert.Zero(t, again.Pushed, "the push finds its own segment and writes nothing")
	names, err := remote.List(ctx, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Len(t, names, 1)
}

func olderOperationRebuilds(t *testing.T, open func(t *testing.T) workspace.Remote) {
	ctx := t.Context()
	a, b := newTestWorkspace(t), newTestWorkspace(t)
	old := workspace.NewOpID(time.Now().Add(-time.Hour), "")
	_, err := a.Record(ctx, workspace.Op{ID: old, Project: "prj", Kind: "context.keep", Payload: []byte(`{}`)})
	require.NoError(t, err)
	_, err = b.Record(ctx, workspace.Op{Project: "prj", Kind: "context.observe", Payload: []byte(`{}`)})
	require.NoError(t, err)
	_, err = a.NewSync(openRemote(t, open), "prj", &testApplier{}, workspace.SyncOptions{LocalKinds: testLocalKinds}).Push(ctx)
	require.NoError(t, err)
	app := &testApplier{}
	pulled, err := b.NewSync(openRemote(t, open), "prj", app, workspace.SyncOptions{LocalKinds: testLocalKinds}).Pull(ctx)
	require.NoError(t, err)
	assert.True(t, pulled.Rebuilt, "an operation that sorts before one applied here is replayed in id order")
	assert.Equal(t, 1, app.rebuilds)
}

func firstPullUsesCheckpoint(t *testing.T, open func(t *testing.T) workspace.Remote) {
	ctx := t.Context()
	a, b := newTestWorkspace(t), newTestWorkspace(t)
	recorded, err := a.Record(ctx,
		workspace.Op{Project: "prj", Kind: "context.keep", Payload: []byte(`{}`)},
		workspace.Op{Project: "prj", Kind: "context.observe", Payload: []byte(`{}`)})
	require.NoError(t, err)
	appA := &testApplier{through: recorded[1].ID}
	pushed, err := a.NewSync(openRemote(t, open), "prj", appA,
		workspace.SyncOptions{LocalKinds: testLocalKinds, CheckpointEvery: 2}).Push(ctx)
	require.NoError(t, err)
	assert.Equal(t, recorded[1].ID, pushed.Checkpoint, "a remote that gained enough operations gets a checkpoint")

	later, err := a.Record(ctx, workspace.Op{Project: "prj", Kind: "context.drop", Payload: []byte(`{}`)})
	require.NoError(t, err)
	pushed, err = a.NewSync(openRemote(t, open), "prj", appA,
		workspace.SyncOptions{LocalKinds: testLocalKinds, CheckpointEvery: 2}).Push(ctx)
	require.NoError(t, err)
	assert.Empty(t, pushed.Checkpoint, "one operation since the checkpoint is not enough for another")

	appB := &testApplier{}
	pulled, err := b.NewSync(openRemote(t, open), "prj", appB, workspace.SyncOptions{LocalKinds: testLocalKinds}).Pull(ctx)
	require.NoError(t, err)
	assert.Equal(t, recorded[1].ID, pulled.Checkpoint)
	assert.Len(t, appB.installed, 1)
	assert.Equal(t, 1, appB.rebuilds, "the stores are rebuilt from the checkpoint and what came after")
	assert.Equal(t, 3, pulled.Merged, "the history travels with the checkpoint")
	held, err := b.Select(ctx, workspace.OpQuery{Project: "prj"})
	require.NoError(t, err)
	assert.Equal(t, later[0].ID, held[len(held)-1].ID)
}
