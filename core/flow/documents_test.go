package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// saveDuringRun passes every part through and, on the first, writes saved to
// path, as a person saving the file in an editor while the run works.
type saveDuringRun struct {
	*tool.BaseTool
	path, saved string
	once        sync.Once
}

func (s *saveDuringRun) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		s.once.Do(func() { _ = os.WriteFile(s.path, []byte(s.saved), 0o644) })
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func pseudoTool(t *testing.T) tool.Tool {
	t.Helper()
	pt, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
	require.NoError(t, err)
	return pt
}

func newTestRegistry() *registry.FormatRegistry {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

func TestFileRunner_KeepsAFileSavedWhileTheRunWorked(t *testing.T) {
	cases := []struct {
		name string
		// inPlace runs the flow over the file it writes.
		inPlace bool
	}{
		{name: "a target-language file"},
		{name: "a file the run edits in place", inPlace: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "en.json")
			require.NoError(t, os.WriteFile(input, []byte(`{"greeting": "Hello World"}`), 0o644))
			output := filepath.Join(dir, "qps.json")
			if tc.inPlace {
				output = input
			} else {
				require.NoError(t, os.WriteFile(output, []byte(`{"greeting": "old"}`), 0o644))
			}
			saved := `{"greeting": "saved by a person"}`
			saver := &saveDuringRun{BaseTool: &tool.BaseTool{ToolName: "save"}, path: output, saved: saved}
			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg: newTestRegistry(), SourceLocale: "en-US",
				Home: filehome.New(nil, filehome.Options{}),
			})
			err := runner.RunFile(t.Context(), "pseudo", []tool.Tool{saver, pseudoTool(t)}, input, output, "qps")
			require.Error(t, err)
			assert.True(t, flow.IsMoved(err), "the run reports the moved file: %v", err)
			got, rerr := os.ReadFile(output)
			require.NoError(t, rerr)
			assert.Equal(t, saved, string(got), "the person's save stands")
			entries, _ := os.ReadDir(dir)
			assert.Len(t, entries, map[bool]int{true: 1, false: 2}[tc.inPlace], "no staged file is left behind")
		})
	}
}

// following records what a run shows it about one document.
type following struct {
	mu       sync.Mutex
	opened   []flow.Document
	entered  map[string]string
	left     map[string]string
	before   string
	commit   bool
	aborted  bool
	produced *filehome.Produced
}

func (f *following) Open(_ context.Context, d flow.Document) (flow.DocumentRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, d)
	return f, nil
}

func (f *following) Enter(b *model.Block) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entered[b.ID] = b.TargetText("qps")
}

func (f *following) Leave(b *model.Block) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.left[b.ID] = b.TargetText("qps")
}

func (f *following) Commit(ctx context.Context, p *filehome.Produced) error {
	f.produced = p
	f.before = p.Before()
	if !f.commit {
		return nil
	}
	return p.Commit(ctx)
}

func (f *following) Abort() { f.aborted = true }

func TestFileRunner_ShowsEachDocumentToItsFollower(t *testing.T) {
	cases := []struct {
		name    string
		commit  bool
		wantOut bool
	}{
		{name: "a follower that commits writes the document", commit: true, wantOut: true},
		{name: "a follower that leaves the document writes nothing", commit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "en.json")
			require.NoError(t, os.WriteFile(input, []byte(`{"greeting": "Hello World", "farewell": "Goodbye"}`), 0o644))
			output := filepath.Join(dir, "out", "qps.json")
			f := &following{entered: map[string]string{}, left: map[string]string{}, commit: tc.commit}
			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg: newTestRegistry(), SourceLocale: "en-US", Documents: f,
				Home: filehome.New(nil, filehome.Options{}),
			})
			require.NoError(t, runner.RunFile(t.Context(), "pseudo", []tool.Tool{pseudoTool(t)}, input, output, "qps"))

			require.Len(t, f.opened, 1)
			assert.Equal(t, flow.Document{Flow: "pseudo", Tools: []string{"pseudo-translate"}, InputPath: input, OutputPath: output, TargetLocale: "qps", Format: "json", OutputFormat: "json"}, f.opened[0])
			assert.False(t, f.opened[0].InPlace())
			assert.Len(t, f.entered, 2)
			for id, before := range f.entered {
				assert.Empty(t, before, "block %s enters with no target", id)
				assert.NotEmpty(t, f.left[id], "block %s leaves with the target the tool wrote", id)
			}
			assert.Empty(t, f.before, "the destination did not exist when the run began")
			assert.False(t, f.aborted)
			_, err := os.Stat(output)
			assert.Equal(t, tc.wantOut, err == nil, "output written: %v", err)
			assert.Equal(t, tc.wantOut, f.produced.Written())
		})
	}
}

func TestFileRunner_CommitsUnderTheHomesLock(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(input, []byte(`{"greeting": "Hello World"}`), 0o644))
	prepared := 0
	home := filehome.New(nil, filehome.Options{PrepareLocks: func() error {
		prepared++
		return nil
	}})
	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: newTestRegistry(), SourceLocale: "en-US", Home: home})
	require.NoError(t, runner.RunFile(t.Context(), "pseudo", []tool.Tool{pseudoTool(t)}, input, filepath.Join(dir, "qps.json"), "qps"))
	assert.Equal(t, 1, prepared, "the commit took a lock in the home")
}

// TestRunFile_LocksInTheDataRoot pins where a runner with no home of its own
// puts its lock files: under locks in the user's data root, where the kapi
// host locks outside a project. Two commits to one file under two values of
// TMPDIR take the same lock file, and neither writes into a temporary
// directory.
func TestRunFile_LocksInTheDataRoot(t *testing.T) {
	runner, reg := skeletonTestRunner(t)
	data, dir, firstTmp, secondTmp := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv(dataroot.EnvDir, data)
	in := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out.md")
	require.NoError(t, os.WriteFile(in, []byte(markdownOnlyASkeletonPreserves), 0o644))

	commit := func(tmp string) {
		t.Helper()
		t.Setenv("TMPDIR", tmp)
		rd, err := reg.NewReader("markdown")
		require.NoError(t, err)
		wr, err := reg.NewWriter("markdown")
		require.NoError(t, err)
		require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "noop", noopTools(), in, out, "", rd, wr))
	}
	commit(firstTmp)
	commit(secondTmp)

	locks, err := filepath.Glob(filepath.Join(data, "locks", "*.lock"))
	require.NoError(t, err, "the commits lock under the data root")
	assert.Len(t, locks, 1, "both commits take the one lock file of the target")
	for _, tmp := range []string{firstTmp, secondTmp} {
		entries, err := os.ReadDir(tmp)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing is written under TMPDIR %s", tmp)
	}
}

// TestRunFile_CreatingAndReplacingAFileTakeOneLock pins that the commit that
// creates a file and a later commit that replaces it take the same lock file
// when the file is named through a linked directory: the lock is named for
// the resolved path whether or not the file exists yet.
func TestRunFile_CreatingAndReplacingAFileTakeOneLock(t *testing.T) {
	runner, reg := skeletonTestRunner(t)
	data, real := t.TempDir(), t.TempDir()
	t.Setenv(dataroot.EnvDir, data)
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks: %v", err)
	}
	in := filepath.Join(real, "notes.md")
	require.NoError(t, os.WriteFile(in, []byte(markdownOnlyASkeletonPreserves), 0o644))

	commit := func(out string) {
		t.Helper()
		rd, err := reg.NewReader("markdown")
		require.NoError(t, err)
		wr, err := reg.NewWriter("markdown")
		require.NoError(t, err)
		require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "noop", noopTools(), in, out, "", rd, wr))
	}
	commit(filepath.Join(link, "out", "notes.md")) // creates the file and its directory
	commit(filepath.Join(real, "out", "notes.md")) // replaces it

	locks, err := filepath.Glob(filepath.Join(data, "locks", "*.lock"))
	require.NoError(t, err)
	assert.Len(t, locks, 1, "both commits take the one lock file of the target")
}
