package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// `kapi state` and `kapi hook` are listed in the reference and on neither page
// of the root help.
func TestCommandsListedInTheReferenceStayOffTheRootHelp(t *testing.T) {
	a := processOnlyApp(t)
	root := &cobra.Command{Use: "kapi", Short: "test"}
	AddCommandGroups(a, root)
	for _, c := range KapiCommandSet(a) {
		root.AddCommand(c)
	}
	shown, all := budgetedCommands(root)
	for _, name := range []string{"state", "hook"} {
		c, _, err := root.Find([]string{name})
		if assert.NoError(t, err) {
			assert.True(t, InReference(c), "%s is in the reference", name)
		}
		assert.NotContains(t, shown, name)
		assert.NotContains(t, all, name)
	}
	assert.False(t, InReference(&cobra.Command{Use: "plumbing", Hidden: true}), "a hidden command without the mark is not")
}
