package cli

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// The context surface (AD C-11), CLI half.
//
// `kapi context` is split by audience. A person meets what applies to a file
// (`kapi context <path>`), what the project says about a word (search), the
// suggestions waiting for a decision (review), sharing (sync) and going back
// to an earlier state (reset). An assistant records what it noticed (note) and
// reads back what its session did (log), listed apart under "For assistants".
// The maintenance verbs live under `kapi store`.
//
// Retrieval is addressed by LOCATION (`kapi context <path>`, the `context://`
// resources) or by CONTENT (`kapi context search`, the `context_search` tool),
// never by store. Each half is two wrappers over one host function, and note
// is the CLI twin of the context_note tool, because the agent skill drives the
// CLI: a capability on one surface only teaches an assistant a surface the
// other does not have.

// Help groups of `kapi context`.
const (
	contextGroupPerson    = "context-person"
	contextGroupAssistant = "context-assistant"
)

// NewContextCmd creates the context command group, which is also the
// by-location verb: `kapi context <path>` answers what applies at a place.
func NewContextCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context [path]",
		Short:   "Ask what applies before you write a file",
		GroupID: "work",
		Long: `Ask what this project keeps to before you write a file: the voice, the
words to use and to avoid, and what has been suggested but not yet decided.

  kapi context <path>          what applies to that file
  kapi context <path> --comments
                               what applies to a comment written in it
  kapi context --profile <n>   the same answer for a named profile
  kapi context search <word>   what the project says about one word

The answer is the text an assistant reads too. --explain adds how it was
reached; --json gives all of it as data.

Rules grow out of ordinary work. An assistant notes what it sees and what you
change, and each note arrives as a suggestion: checks report it and none fails
on it until you keep it. Decide about suggestions with review, share the
context with the rest of the team with sync, and go back to an earlier state
with reset.`,
		Example: "  kapi context docs/guide.md\n" +
			"  kapi context docs/guide.md --explain\n" +
			"  kapi context cmd/main.go --comments\n" +
			"  kapi context search widget\n" +
			"  kapi context review\n" +
			"  kapi context sync",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
				if err := host.RetiredContextVerbError(path); err != nil {
					return err
				}
			}
			profile, _ := cmd.Flags().GetString("profile")
			if path == "" && profile == "" {
				return cmd.Help()
			}

			locale, _ := cmd.Flags().GetString("locale")
			limit, _ := cmd.Flags().GetInt("limit")
			comments, _ := cmd.Flags().GetBool("comments")
			req := host.ContextPointRequest{
				Path:     path,
				Profile:  profile,
				Locale:   model.LocaleID(locale),
				Limit:    limit,
				Comments: comments,
			}

			// One assembly for both this verb and the `context://` MCP
			// resources (host.ContextSourcesAt).
			src, cleanup := a.ContextSourcesAt(cmd, req)
			defer cleanup()

			res, err := host.ResolveContextAt(cmd.Context(), src, req)
			if err != nil {
				return err
			}
			if explain, _ := cmd.Flags().GetBool("explain"); explain {
				res.Explain()
			}
			// The answer renders itself (host.ContextAnswer implements
			// output.TextFormatter), so the markdown a reader sees here is the
			// body the MCP resource serves, and --json is the same document the
			// resource's application/json rendering carries.
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("profile", "", "answer for a named profile instead of a location")
	cmd.Flags().Bool("comments", false, "answer for the comments in the file rather than its content")
	cmd.Flags().StringP("locale", "l", "", "narrow the reported terms to one language")
	cmd.Flags().Int("limit", host.DefaultContextTermsLimit, "max word rules to render")
	cmd.Flags().Bool("explain", false, "add how the answer was reached: point, binding, project, revision and notes")
	// The same project- and store-resolution flags every other project verb
	// carries, so this command resolves the same way `terms` and `memory` do.
	// The output format axis (--json among them) is persistent on the root.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)

	cmd.AddGroup(
		&cobra.Group{ID: contextGroupPerson, Title: "Commands:"},
		&cobra.Group{ID: contextGroupAssistant, Title: "For assistants:"},
	)
	for _, sub := range []*cobra.Command{
		newContextSearchCmd(a),
		newContextReviewCmd(a),
		newContextResetCmd(a),
		newContextSyncCmd(a),
	} {
		sub.GroupID = contextGroupPerson
		cmd.AddCommand(sub)
	}
	for _, sub := range []*cobra.Command{
		newContextNoteCmd(a),
		newContextLogCmd(a),
	} {
		sub.GroupID = contextGroupAssistant
		cmd.AddCommand(sub)
	}
	return cmd
}

func newContextSearchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Ask what the project says about a word or phrase",
		Long: `Ask one question and get the answer from everything the project keeps:
what this is called and whether it is discouraged, and wording the project has
already approved.

Results are grouped by kind rather than merged into one ranked list, because a
match on a term and a match on approved wording are not scored on comparable
things.

Each term also reports how often the project's content uses it. The count is
as of the last "kapi up" rather than of the working tree: a term added since
then shows no uses until the next run.`,
		Example: "  kapi context search widget\n" +
			"  kapi context search \"sign in\" --locale en\n" +
			"  kapi context search widget --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			locale, _ := cmd.Flags().GetString("locale")
			limit, _ := cmd.Flags().GetInt("limit")

			// One assembly for both this verb and the context_search MCP tool
			// (host.ContextSearchSourcesFor). The CLI passes no standalone paths,
			// so it takes the store the --name/--file/--local flags resolve to.
			src, cleanup := a.ContextSearchSourcesFor(cmd, "", "")
			defer cleanup()

			res, err := host.SearchContext(cmd.Context(), src, host.ContextSearchRequest{
				Query:  args[0],
				Locale: model.LocaleID(locale),
				Limit:  limit,
			})
			if err != nil {
				return err
			}
			// The result renders itself (host.ContextSearchResult implements
			// output.TextFormatter), so --json emits exactly the shape the MCP
			// tool returns and the text form is defined in one place.
			return output.Print(cmd, res)
		},
	}

	cmd.Flags().StringP("locale", "l", "", "narrow results to one language")
	cmd.Flags().Int("limit", host.DefaultContextSearchLimit, "max results per group")
	// The same project- and store-resolution flags every other project verb
	// carries. Without them this command would resolve stores differently from
	// `terms` and `memory`, and it could only ever be pointed at a project by
	// standing in it, which no run under the isolation contract can do.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
	return cmd
}
