package host

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInPlaceFlagSet builds a flag set carrying just -i, the way ksed and
// `kapi apply` register it.
func newInPlaceFlagSet() (*pflag.FlagSet, *InPlaceFlag) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	v := RegisterInPlace(fs, "edit files in place; append a backup SUFFIX if given (e.g. -i.bak)")
	return fs, v
}

// TestInPlaceFlagForms covers every spelling of -i that reaches the parser,
// including the attached-suffix form that NormalizeSedInPlaceArgs rewrites.
func TestInPlaceFlagForms(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSet    bool
		wantSuffix string
	}{
		{name: "absent", args: []string{"file.md"}},
		{name: "bare short", args: []string{"-i", "file.md"}, wantSet: true},
		{name: "bare long", args: []string{"--in-place", "file.md"}, wantSet: true},
		{name: "attached suffix", args: []string{"-i.bak", "file.md"}, wantSet: true, wantSuffix: ".bak"},
		{name: "long with suffix", args: []string{"--in-place=.orig", "file.md"}, wantSet: true, wantSuffix: ".orig"},
		{name: "short with equals", args: []string{"-i=.bak", "file.md"}, wantSet: true, wantSuffix: ".bak"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, v := newInPlaceFlagSet()
			require.NoError(t, fs.Parse(NormalizeSedInPlaceArgs(tc.args)))
			assert.Equal(t, tc.wantSet, fs.Changed("in-place"), "the flag being set is what turns editing on")
			assert.Equal(t, tc.wantSuffix, v.Suffix)
			assert.Equal(t, []string{"file.md"}, fs.Args(), "a bare -i must not swallow the filename")
		})
	}
}

// TestInPlaceFlagUsageIsPlainText is the regression: pflag prints NoOptDefVal
// verbatim into the flag list, so the NUL-prefixed sentinel this flag used to
// carry put a raw NUL byte into --help. The usage text registered as binary,
// which is how `ksed --help | grep` came back empty.
func TestInPlaceFlagUsageIsPlainText(t *testing.T) {
	fs, _ := newInPlaceFlagSet()
	usage := fs.FlagUsages()

	assert.NotContains(t, usage, "\x00", "no unprintable byte may reach --help")
	for _, r := range usage {
		require.False(t, r < ' ' && r != '\n' && r != '\t',
			"control character %q in the flag list: %q", r, usage)
	}
	// Rendered as a plain switch, with the optional suffix left to the
	// description — the way GNU sed documents -i.
	assert.Contains(t, usage, "-i, --in-place ")
	assert.NotContains(t, usage, "[=", "the NoOptDefVal marker is an implementation detail, not documentation")
}

// TestInPlaceFlagBareMarkerNeverBecomesASuffix pins the one seam of the
// "bool"-typed rendering: the marker pflag substitutes for a bare -i must never
// be mistaken for a backup suffix.
func TestInPlaceFlagBareMarkerNeverBecomesASuffix(t *testing.T) {
	fs, v := newInPlaceFlagSet()
	require.NoError(t, fs.Parse([]string{"-i", "file.md"}))
	assert.Empty(t, v.Suffix)
	assert.NotContains(t, v.String(), inPlaceBare)
	assert.False(t, strings.Contains(v.Suffix, "true"))
}
