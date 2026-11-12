package cli

import (
	"github.com/neokapi/neokapi/host"
	"github.com/spf13/cobra"
)

// newMemoryAnnotateCmd returns `kapi memory annotate`, which marks a text as a
// named entity in the content memory's entries (host.RunMemoryAnnotate).
func newMemoryAnnotateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "annotate <text> --type <entity-type>",
		Short: "Mark a text as a named entity in content-memory entries",
		Long: `Mark a text as a named entity in the content memory's entries.

Each occurrence of the text, in every language of an entry, becomes an entity
placeholder of the given type, and the entry records the entity once with its
wording in each language. A lookup can then match the entry for content that
names a different entity of the same type in the same place.

With no --entry, every entry whose text contains the text is annotated.
Inside a project, each new entity is linked to the concept in the project's
terms store whose term it matches.

The entity types are person, organization, product, location, date, time,
currency, measurement and other.`,
		Example: `  kapi memory annotate "Acme" --type organization
  kapi memory annotate "Paris" --type location --case-sensitive
  kapi memory annotate "Kapi Pro" --type product --entry 3f2a9c`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, _ := cmd.Flags().GetString("type")
			caseSensitive, _ := cmd.Flags().GetBool("case-sensitive")
			entries, _ := cmd.Flags().GetStringSlice("entry")
			return a.RunMemoryAnnotate(cmd, host.MemoryAnnotateOptions{
				Text: args[0], Type: typ, CaseSensitive: caseSensitive, Entries: entries,
			})
		},
	}
	cmd.Flags().String("type", "", "entity type: person, organization, product, location, date, time, currency, measurement or other")
	cmd.Flags().Bool("case-sensitive", false, "match the text in its own case only")
	cmd.Flags().StringSlice("entry", nil, "annotate only these entry ids (repeatable, or comma-separated)")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
