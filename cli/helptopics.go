package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/skills"
)

// SetupHelpTopics replaces cobra's help command with one that also serves the
// reference topics of the kapi skill.
//
// `kapi init` writes a short skill into a project and points it here for the
// rest, so an agent reads the guidance of the binary it is running. `kapi
// help` prints the command overview and then the topics; `kapi help <topic>`
// prints one topic; any other argument is a command, as it always was.
//
// Four topics share a name with a command (check, context, translate, voice).
// The topic is what `kapi help <name>` prints, followed by a line naming the
// command's own `--help`, because a person asking for help on a word wants the
// guidance and the flags are one command away.
func SetupHelpTopics(root *cobra.Command) {
	help := &cobra.Command{
		Use:   "help [topic | command]",
		Short: "Help about any command, and guides by topic",
		Long: `Help about any command, and guides by topic.

With no argument, kapi help prints the command overview and then the topics.
Name a topic to print its guide; name a command to print its help.`,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var out []cobra.Completion
			for _, t := range skills.Topics() {
				if strings.HasPrefix(t.Name, toComplete) {
					out = append(out, cobra.CompletionWithDesc(t.Name, t.Title))
				}
			}
			for _, c := range root.Commands() {
				if c.IsAvailableCommand() && strings.HasPrefix(c.Name(), toComplete) {
					out = append(out, cobra.CompletionWithDesc(c.Name(), c.Short))
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if len(args) == 0 {
				if err := root.Help(); err != nil {
					return err
				}
				return writeTopicList(w)
			}
			if len(args) == 1 {
				if text, ok := skills.TopicText(args[0]); ok {
					if _, err := io.WriteString(w, strings.TrimRight(text, "\n")+"\n"); err != nil {
						return err
					}
					if c, _, err := root.Find(args); err == nil && c != root {
						_, err := fmt.Fprintf(w, "\nFlags and subcommands: kapi %s --help\n", c.Name())
						return err
					}
					return nil
				}
			}
			target, _, err := root.Find(args)
			if err != nil || target == nil || target == root {
				return fmt.Errorf("no help topic or command %q; 'kapi help' lists them", strings.Join(args, " "))
			}
			target.InitDefaultHelpFlag()
			target.InitDefaultVersionFlag()
			return target.Help()
		},
	}
	root.SetHelpCommand(help)
}

// writeTopicList prints every topic with its title, under the command
// overview.
func writeTopicList(w io.Writer) error {
	topics := skills.Topics()
	width := 0
	for _, t := range topics {
		width = max(width, len(t.Name))
	}
	var b strings.Builder
	b.WriteString("\nTopics:\n")
	for _, t := range topics {
		fmt.Fprintf(&b, "  %-*s %s\n", width, t.Name, t.Title)
	}
	b.WriteString("\nUse \"kapi help <topic>\" to read a topic.\n")
	_, err := io.WriteString(w, b.String())
	return err
}
