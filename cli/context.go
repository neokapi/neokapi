package cli

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// The context retrieval surface (AD-037), CLI half.
//
// `kapi context search` and the `context_search` MCP tool are two wrappers over
// one host function. That is deliberate: the surfaces drifted before — six CLI
// retrieval verbs against three MCP tools, with no rule for which got exposed —
// and the agent skill drives the CLI, so a CLI-only capability teaches an
// assistant a surface that MCP does not have.

// NewContextCmd creates the context retrieval command group.
func NewContextCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Short:   "Ask what this project's content context says",
		GroupID: "work",
		Long: `Retrieve the content context this project goes by — the terms, the
voice and the rules that hold — without needing to know which store holds the
answer.

Communication is contextual: a legal notice is not a help article. Ask what the
project says about a word or a phrase before you write, rather than learning it
from a failing check afterwards.`,
	}
	cmd.AddCommand(newContextSearchCmd(a))
	return cmd
}

func newContextSearchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Ask what the project's context says about a word or phrase",
		Long: `Ask one question and get the answer from every store the project
binds: what this is called and whether it is discouraged, and wording the
project has already approved.

Results are grouped by kind rather than merged into one ranked list, because a
terminology match and a wording match are not scored on comparable things.`,
		Example: "  kapi context search widget\n" +
			"  kapi context search \"sign in\" --locale en\n" +
			"  kapi context search widget --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			locale, _ := cmd.Flags().GetString("locale")
			limit, _ := cmd.Flags().GetInt("limit")

			src := host.ContextSearchSources{Scope: host.ScopeProject}

			// A store the project has not bound is not an error: a project with
			// terminology but no content memory is ordinary. SearchContext
			// reports the gap in its notes so an empty answer is never ambiguous
			// between "no answer" and "nowhere to look".
			if tb, _, err := a.OpenTermsSQLite(cmd); err == nil {
				defer tb.Close()
				src.Terms = tb
			}
			if tm, _, err := a.OpenMemorySQLite(cmd); err == nil {
				defer tm.Close()
				src.Memory = tm
			}

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
	// The same store-resolution flags every other store verb carries. Without
	// them this command would resolve stores differently from `terms` and
	// `memory` — a parity break inside the CLI, before MCP even enters it.
	AddResourceFlags(cmd)
	return cmd
}
