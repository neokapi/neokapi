package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpRoot builds a root with one command that shares a name with a topic and
// one that does not, and the topic-aware help command.
func helpRoot() *cobra.Command {
	root := &cobra.Command{Use: "kapi", Short: "kapi root"}
	root.AddCommand(&cobra.Command{Use: "check", Short: "Verify content", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "inspect", Short: "Parse any format", Run: func(*cobra.Command, []string) {}})
	SetupHelpTopics(root)
	return root
}

func runHelp(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := helpRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"help"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestHelp_ListsTopics(t *testing.T) {
	out, err := runHelp(t)
	require.NoError(t, err)
	assert.Contains(t, out, "inspect", "the command overview comes first")
	assert.Contains(t, out, "Topics:")
	assert.Contains(t, out, "growing-context")
	assert.Contains(t, out, `kapi help <topic>`)
}

func TestHelp_PrintsATopic(t *testing.T) {
	out, err := runHelp(t, "growing-context")
	require.NoError(t, err)
	assert.Contains(t, out, "# Growing a project's context")
	assert.NotContains(t, out, "Flags and subcommands", "no command shares this topic's name")
}

// A topic that shares its name with a command prints the guide and then names
// the command's own help.
func TestHelp_TopicSharingACommandName(t *testing.T) {
	out, err := runHelp(t, "check")
	require.NoError(t, err)
	assert.Contains(t, out, "# Check what you changed")
	assert.Contains(t, out, "Flags and subcommands: kapi check --help")
}

func TestHelp_CommandAndUnknown(t *testing.T) {
	out, err := runHelp(t, "inspect")
	require.NoError(t, err)
	assert.Contains(t, out, "Parse any format")

	_, err = runHelp(t, "no-such-thing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'kapi help' lists them")
}
