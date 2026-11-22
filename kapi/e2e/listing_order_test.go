//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every listing answers with the same bytes on every run, text and --json
// alike: a person comparing two runs, or a script diffing them, sees no
// churn that is only map order.
func TestListingsAreTheSameOnEveryRun(t *testing.T) {
	for _, args := range [][]string{
		{"tools"}, {"tools", "--json"},
		{"formats"}, {"formats", "--json"},
		{"flows"}, {"flows", "--json"},
		{"plugins", "list"}, {"plugins", "list", "--json"},
		{"models"}, {"models", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			run := func() string {
				cmd := exec.Command(kapiBin, args...)
				cmd.Env = append(os.Environ(), isoEnv...)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", out)
				return string(out)
			}
			first := run()
			require.NotEmpty(t, first)
			for range 19 {
				assert.Equal(t, first, run())
			}
		})
	}
}
