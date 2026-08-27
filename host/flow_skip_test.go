package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --fail-on-unknown documents itself as "exit with error if any file cannot be
// processed (default: skip with a warning)". host/toolrun.go did that; this
// path did not, and the batch runner is an errgroup, so one file with no
// registered format cancelled every sibling. A directory of 843 readable files
// and one .h produced no output at all and a single error line.
//
// The engine benchmark ran into exactly that and went three months without a
// refresh, with the page showing numbers from 2026-05-20 and nothing saying so.

// flagSet builds the two flags the strictness check reads.
func flagSet(t *testing.T, set map[string]bool) Command {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Bool("fail-on-unknown", false, "")
	fs.Bool("strict", false, "")
	for name, v := range set {
		if v {
			require.NoError(t, fs.Set(name, "true"))
		}
	}
	return &testCommand{flags: fs}
}

type testCommand struct {
	Command
	flags *pflag.FlagSet
}

func (c *testCommand) Flags() *pflag.FlagSet { return c.flags }

func TestFailOnUnknownReadsBothSpellings(t *testing.T) {
	assert.False(t, failOnUnknown(flagSet(t, nil)),
		"the default is to skip, which is what the flag's own help text promises")
	assert.True(t, failOnUnknown(flagSet(t, map[string]bool{"fail-on-unknown": true})))
	assert.True(t, failOnUnknown(flagSet(t, map[string]bool{"strict": true})),
		"--strict is documented as an alias and has to behave as one")
}

// TestFailOnUnknownOnACommandWithoutTheFlags: the flow commands did not carry
// either flag until this change, and a Command that has never heard of them
// must not be read as asking for strictness.
func TestFailOnUnknownOnACommandWithoutTheFlags(t *testing.T) {
	fs := pflag.NewFlagSet("bare", pflag.ContinueOnError)
	assert.False(t, failOnUnknown(&testCommand{flags: fs}),
		"an absent flag means the default, and the default is to skip")
}

func TestShortListKeepsAWarningReadable(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, shortList([]string{"a", "b"}, 3))
	assert.Equal(t, []string{"a", "b", "c"}, shortList([]string{"a", "b", "c"}, 3))
	assert.Equal(t, []string{"a", "b", "c", "and 2 more"},
		shortList([]string{"a", "b", "c", "d", "e"}, 3))

	// The source slice must survive: it is the skip list the caller still owns.
	src := []string{"a", "b", "c", "d"}
	_ = shortList(src, 2)
	assert.Equal(t, []string{"a", "b", "c", "d"}, src, "shortList must not scribble on its input")
}

// TestSkipSentinelIsDistinguishable: the batch runner tells a skip from a
// failure with errors.Is, so the sentinel has to survive being wrapped with the
// file name and the underlying cause.
func TestSkipSentinelIsDistinguishable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.h")
	require.NoError(t, os.WriteFile(path, []byte("int main(void){return 0;}\n"), 0o644))

	wrapped := wrapSkip(path, assertErr("no format found for extension \".h\""))
	require.ErrorIs(t, wrapped, errSkippedFile)
	assert.Contains(t, wrapped.Error(), "x.h", "the warning has to name the file it passed over")
	assert.Contains(t, wrapped.Error(), "no format found",
		"and why, or a skip is indistinguishable from a file that was simply not there")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
