package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/credentials"
)

// A missing key's advice names --api-key exactly for the commands that read
// it, and for no other.
func TestAdviseAPIKey(t *testing.T) {
	a := processOnlyApp(t)
	missing := fmt.Errorf("flow %q: tool %q: %w", "converge", "translate",
		&credentials.MissingCredentialError{Provider: "anthropic"})
	withFlag := credentials.MissingKeyMessage("anthropic", true)
	without := credentials.MissingKeyMessage("anthropic", false)

	var exec *cobra.Command
	for _, c := range NewToolCommands(a) {
		if c.Name() == "exec" {
			for _, sub := range c.Commands() {
				if sub.Name() == "translate" {
					exec = sub
				}
			}
		}
	}
	require.NotNil(t, exec, "kapi exec translate")

	for name, cmd := range map[string]*cobra.Command{
		"up":             NewUpCmd(a),
		"run":            NewRunCmd(a),
		"translate":      NewTranslateCmd(a),
		"exec translate": exec,
	} {
		t.Run(name, func(t *testing.T) {
			got := AdviseAPIKey(cmd, missing)
			assert.Contains(t, got.Error(), withFlag)
			var m *credentials.MissingCredentialError
			require.ErrorAs(t, got, &m)
		})
	}

	got := AdviseAPIKey(NewVersionCmd(a, "kapi"), missing)
	assert.Contains(t, got.Error(), without)
	assert.NotContains(t, got.Error(), "--api-key")

	other := errors.New("disk full")
	assert.Equal(t, other, AdviseAPIKey(NewUpCmd(a), other))
	assert.NoError(t, AdviseAPIKey(NewUpCmd(a), nil))
}
