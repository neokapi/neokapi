package flow_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1468 item 1, the core/flow half: RunFileWithReaderWriter and RunStream both
// opened the skeleton store with `if store, err := …; err == nil` and ran on
// without one when it failed. For a same-format pass the skeleton is the only
// carrier of the structure the reader could not model, so running on silently
// replaces the document with a reconstruction of itself — and the run reports
// the file written.
//
// Failure is induced by a real filesystem condition: TMPDIR pointed at a
// directory that does not exist. Note that the output write is unaffected by it
// (runPipelineToWriter stages its temp file in the OUTPUT directory), so the
// only thing a broken TMPDIR breaks here is the skeleton — which is exactly the
// condition under test.

// markdownOnlyASkeletonPreserves uses a setext heading, a reference link and
// irregular spacing; none of it survives a re-serialization from the content
// model.
const markdownOnlyASkeletonPreserves = `Release notes
=============

Some  *emphasis*   and a [reference][1].

[1]: https://example.com
`

func breakTempDir(t *testing.T) string {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "no-such-tmpdir")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	require.Equal(t, missing, os.TempDir(), "sanity: os.TempDir must follow the swapped TMPDIR")
	return missing
}

func skeletonTestRunner(t *testing.T) (*flow.FileRunner, *registry.FormatRegistry) {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: reg, SourceLocale: "en-US"}), reg
}

// noopTools is a minimal, order-preserving tool chain: the run must still have
// one, and a pass-through tool keeps the assertion about the document alone.
func noopTools() []tool.Tool { return []tool.Tool{&tool.BaseTool{ToolName: "noop"}} }

// TestRunFileWithReaderWriter_SkeletonStoreFailureFailsTheRun is the regression
// for core/flow/filerunner.go's swallow. What a user saw: `kapi run translate
// notes.md` exit 0, "file written", and a file whose setext heading, reference
// link and spacing had all been normalised away.
func TestRunFileWithReaderWriter_SkeletonStoreFailureFailsTheRun(t *testing.T) {
	runner, reg := skeletonTestRunner(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out", "notes.md")
	require.NoError(t, os.WriteFile(in, []byte(markdownOnlyASkeletonPreserves), 0o644))

	rd, err := reg.NewReader("markdown")
	require.NoError(t, err)
	wr, err := reg.NewWriter("markdown")
	require.NoError(t, err)

	breakTempDir(t)

	err = runner.RunFileWithReaderWriter(context.Background(), "noop", noopTools(), in, out, "", rd, wr)
	require.Error(t, err, "a same-format run that cannot preserve the document must fail")
	assert.Contains(t, err.Error(), "notes.md", "the error must name the file")
	assert.Contains(t, err.Error(), "formatting", "the error must say what would have been lost")
	assert.NoFileExists(t, out, "and no reconstructed file may be written")
}

// TestRunFileWithReaderWriter_RoundTripIsByteIdentical is the control: with a
// working temp filesystem the same run reproduces the source exactly, so the
// test above is pinning the failure and not the pipeline.
func TestRunFileWithReaderWriter_RoundTripIsByteIdentical(t *testing.T) {
	runner, reg := skeletonTestRunner(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out", "notes.md")
	require.NoError(t, os.WriteFile(in, []byte(markdownOnlyASkeletonPreserves), 0o644))

	rd, err := reg.NewReader("markdown")
	require.NoError(t, err)
	wr, err := reg.NewWriter("markdown")
	require.NoError(t, err)

	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "noop", noopTools(), in, out, "", rd, wr))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, markdownOnlyASkeletonPreserves, string(got))
}

// TestRunFileWithReaderWriter_CrossFormatRunsWithoutASkeleton is the
// discriminating control: absent and present-but-unobtainable are different
// facts. A cross-format run reconstructs from the content model by design and
// never wires a skeleton, so it must keep working with the very same broken
// TMPDIR that fails the same-format run — "fail whenever TMPDIR is broken" is
// not an acceptable substitute for the fix.
func TestRunFileWithReaderWriter_CrossFormatRunsWithoutASkeleton(t *testing.T) {
	runner, reg := skeletonTestRunner(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out", "notes.html")
	require.NoError(t, os.WriteFile(in, []byte(markdownOnlyASkeletonPreserves), 0o644))

	rd, err := reg.NewReader("markdown")
	require.NoError(t, err)
	wr, err := reg.NewWriter("html")
	require.NoError(t, err)

	breakTempDir(t)

	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "noop", noopTools(), in, out, "", rd, wr),
		"a cross-format run needs no skeleton, so an unusable TMPDIR must not fail it")
	got, rerr := os.ReadFile(out)
	require.NoError(t, rerr)
	assert.Contains(t, string(got), "emphasis")
}

// TestRunStream_SkeletonStoreFailureFailsTheEntry covers the container-entry
// twin of the site above. What a user saw: `kapi run translate bundle.zip` exit
// 0, the archive repacked, and the member inside it replaced by a
// reconstruction — the repack making the substitution harder to notice.
func TestRunStream_SkeletonStoreFailureFailsTheEntry(t *testing.T) {
	runner, _ := skeletonTestRunner(t)

	breakTempDir(t)

	var out bytes.Buffer
	err := runner.RunStream(context.Background(), "noop", noopTools(),
		strings.NewReader(markdownOnlyASkeletonPreserves), "bundle.zip!notes.md",
		registry.FormatID("markdown"), &out, "")
	require.Error(t, err, "an entry that cannot be preserved must fail rather than be repacked as a reconstruction")
	assert.Contains(t, err.Error(), "bundle.zip!notes.md", "the error must name the entry")
	assert.Empty(t, out.Bytes(), "and nothing may be emitted for it")
}

// TestRunStream_RoundTripIsByteIdentical is the control for the entry path.
func TestRunStream_RoundTripIsByteIdentical(t *testing.T) {
	runner, _ := skeletonTestRunner(t)

	var out bytes.Buffer
	require.NoError(t, runner.RunStream(context.Background(), "noop", noopTools(),
		strings.NewReader(markdownOnlyASkeletonPreserves), "bundle.zip!notes.md",
		registry.FormatID("markdown"), &out, ""))

	assert.Equal(t, markdownOnlyASkeletonPreserves, out.String())
}
