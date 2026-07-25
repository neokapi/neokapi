package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The verb is read back out of cobra's own error text, so pin the format we
// parse against cobra itself rather than a hand-written string.
func TestUnknownCommandVerb_MatchesCobrasRealError(t *testing.T) {
	root := &cobra.Command{Use: "kapi", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "extract", RunE: func(*cobra.Command, []string) error { return nil }})
	root.SetArgs([]string{"push"})
	root.SetOut(&nopWriter{})
	root.SetErr(&nopWriter{})

	err := root.Execute()
	require.Error(t, err)

	verb, ok := unknownCommandVerb(err)
	require.True(t, ok, "cobra's unknown-command error must be recognized: %q", err)
	assert.Equal(t, "push", verb)
}

func TestUnknownCommandVerb_IgnoresOtherErrors(t *testing.T) {
	for _, err := range []error{
		nil,
		errors.New("load project: no kapi project found"),
		errors.New(`unknown flag: --nope`),
		errors.New("unknown command"),
		errors.New(`unknown command for "kapi"`),
		fmt.Errorf(`unknown command "" for %q`, "kapi"),
	} {
		_, ok := unknownCommandVerb(err)
		assert.False(t, ok, "must not treat %v as an unknown command", err)
	}
}

func TestArgsAfterVerb(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		verb string
		want []string
	}{
		{"bare verb", []string{"push"}, "push", []string{}},
		{"verb flags", []string{"push", "--force", "src/"}, "push", []string{"--force", "src/"}},
		{"global flags first", []string{"--quiet", "push", "--dry-run"}, "push", []string{"--dry-run"}},
		{"verb absent", []string{"--quiet"}, "push", nil},
		{"repeated token takes the first", []string{"push", "push"}, "push", []string{"push"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, argsAfterVerb(tt.argv, tt.verb))
		})
	}
}

func TestSetUnknownCommandHandler_DefaultsToUnset(t *testing.T) {
	saved := unknownCommandHandler
	t.Cleanup(func() { unknownCommandHandler = saved })

	unknownCommandHandler = nil
	assert.Nil(t, unknownCommandHandler, "binaries that never register a handler keep cobra's behavior")

	called := false
	SetUnknownCommandHandler(func(*cobra.Command, string, []string) (bool, error) { called = true; return false, nil })
	require.NotNil(t, unknownCommandHandler)
	handled, err := unknownCommandHandler(nil, "push", nil)
	assert.True(t, called)
	assert.False(t, handled)
	assert.NoError(t, err)
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
