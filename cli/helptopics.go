package cli

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host/skills"
)

// SetupHelpTopics replaces cobra's help command with one that also serves the
// reference topics of the kapi skill, so an agent reads the guidance of the
// binary it is running. With no argument, help prints the command overview
// and then the topics. A topic name prints that topic, and any other argument
// is a command.
//
// The topics follow the tiers of the command groups (helptiers.go): the
// translate and i18n topics appear when the Languages group does. With --all,
// help prints every group, every global flag and every topic. The argument
// languages prints the Languages group after a lead, then the language
// topics.
//
// Four topics share a name with a command (check, context, translate, voice).
// For those, help prints the topic and then a line naming the command's
// --help, since a person asking about a word wants the guidance first.
func SetupHelpTopics(root *cobra.Command) {
	help := &cobra.Command{
		Use:   "help [topic | command]",
		Short: "Help about any command, and guides by topic",
		Long: `Help about any command, and guides by topic.

With no argument, kapi help prints the command overview and then the topics.
Name a topic to print its guide; name a command to print its help.

kapi help --all lists every command, every global flag and every topic. kapi
help languages lists the commands and topics for content in other languages.`,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var out []cobra.Completion
			if strings.HasPrefix(languagesGroupID, toComplete) {
				out = append(out, cobra.CompletionWithDesc(languagesGroupID, "The commands for content in other languages"))
			}
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
				// cobra adds --help and --version to the command it executes,
				// which here is help, so the root's Flags block needs them added.
				root.InitDefaultHelpFlag()
				root.InitDefaultVersionFlag()
				all, _ := cmd.Flags().GetBool("all")
				if all {
					writeStyledHelp(root, w, helpView{all: true})
				} else if err := root.Help(); err != nil {
					return err
				}
				return writeTopicList(w, scopeFor(root, helpView{all: all}))
			}
			if len(args) == 1 && args[0] == languagesGroupID {
				return writeLanguagesHelp(root, w)
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
	help.Flags().Bool("all", false, "with no topic or command, list every command, global flag and topic")
	root.SetHelpCommand(help)
}

// languagesLead opens `kapi help languages`: how a project gets the Languages
// group, and where the bilingual exchange verbs are listed.
const languagesLead = `To work in another language, add it to the recipe under
defaults.target_languages and give each collection a target path. kapi --help
then lists these commands, and kapi up translates the content. kapi extract
and kapi merge, listed by kapi help --all, exchange bilingual files with a
translator.`

// writeLanguagesHelp prints `kapi help languages`: the lead, the root's
// Languages group laid out as the root help lays out a group, and the
// language topics. A topic named languages, should the skill gain one, prints
// first.
func writeLanguagesHelp(root *cobra.Command, w io.Writer) error {
	if text, ok := skills.TopicText(languagesGroupID); ok {
		if _, err := io.WriteString(w, strings.TrimRight(text, "\n")+"\n\n"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, languagesLead+"\n\n"); err != nil {
		return err
	}
	st := themeFor(root, w)
	t := template.Must(template.New("languages").Funcs(usageFuncs(st)).Parse(languagesGroupTemplate))
	if err := t.Execute(w, root); err != nil {
		return err
	}
	var topics []skills.Topic
	for _, topic := range skills.Topics() {
		if topicTier(topic.Name) == tierLanguages {
			topics = append(topics, topic)
		}
	}
	return writeTopics(w, topics, `Use "kapi help <topic>" to read a topic.`)
}

// languagesGroupTemplate renders the root's Languages group with the root
// help's heading, name column and footer.
const languagesGroupTemplate = `{{range $group := .Groups}}{{if eq $group.ID "` + languagesGroupID + `"}}{{heading $group.Title}}{{range $.Commands}}{{if (and (eq .GroupID $group.ID) .IsAvailableCommand)}}
  {{cmdname (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{end}}

{{muted (printf "Use \"%s [command] --help\" for more information about a command." .CommandPath)}}
`

// writeTopicList prints the topics the scope shows, under the command
// overview. The topics are tiered as the command groups are: the language
// topics appear when the Languages group does, and `kapi help languages` lists
// them otherwise.
func writeTopicList(w io.Writer, scope usageScope) error {
	var topics []skills.Topic
	for _, t := range skills.Topics() {
		if scope.topicShown(t.Name) {
			topics = append(topics, t)
		}
	}
	hints := []string{`Use "kapi help <topic>" to read a topic.`}
	if !scope.tierShown(tierLanguages) {
		hints = append(hints, `Use "kapi help languages" to list the topics for other languages.`)
	}
	return writeTopics(w, topics, hints...)
}

// writeTopics prints topics with their titles under a Topics heading, then
// the hints.
func writeTopics(w io.Writer, topics []skills.Topic, hints ...string) error {
	width := 0
	for _, t := range topics {
		width = max(width, len(t.Name))
	}
	var b strings.Builder
	b.WriteString("\nTopics:\n")
	for _, t := range topics {
		fmt.Fprintf(&b, "  %-*s %s\n", width, t.Name, t.Title)
	}
	b.WriteString("\n")
	for _, h := range hints {
		b.WriteString(h + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
