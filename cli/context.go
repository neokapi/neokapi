package cli

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// The context retrieval surface (AD-037), CLI half.
//
// Retrieval is addressed by LOCATION (`kapi context <path>`, the `context://`
// resources) or by CONTENT (`kapi context search`, the `context_search` tool),
// never by store. Each half is two wrappers over one host function. That is
// deliberate: the surfaces drifted before — six CLI retrieval verbs against
// three MCP tools, with no rule for which got exposed — and the agent skill
// drives the CLI, so a CLI-only capability teaches an assistant a surface that
// MCP does not have.

// NewContextCmd creates the context retrieval command group, which is also the
// by-location verb: `kapi context <path>` answers what applies at a place.
func NewContextCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context [path]",
		Short:   "Ask what this project's content context says",
		GroupID: "work",
		Long: `Retrieve the content context this project goes by: the terms, the
voice and the rules that hold, without needing to know which store holds the
answer.

Two questions, and every asset-shaped lookup is one of them:

  kapi context <path>          what applies HERE: the voice in force at that
                               location, its guidance, the terms bound there
  kapi context --profile <n>   the same answer for a named profile, when you
                               have no file in hand
  kapi context search <query>  what we know about THIS: terms, prior wording

Communication is contextual: a legal notice is not a help article. Ask what the
project says before you write, rather than learning it from a failing check
afterwards.`,
		Example: "  kapi context docs/guide.md\n" +
			"  kapi context docs/guide.md --json\n" +
			"  kapi context --profile marketing\n" +
			"  kapi context search widget",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			profile, _ := cmd.Flags().GetString("profile")
			if path == "" && profile == "" {
				return cmd.Help()
			}

			locale, _ := cmd.Flags().GetString("locale")
			limit, _ := cmd.Flags().GetInt("limit")
			req := host.ContextPointRequest{
				Path:    path,
				Profile: profile,
				Locale:  model.LocaleID(locale),
				Limit:   limit,
			}

			// One assembly for both this verb and the `context://` MCP
			// resources (host.ContextSourcesAt).
			src, cleanup := a.ContextSourcesAt(cmd, req)
			defer cleanup()

			res, err := host.ResolveContextAt(cmd.Context(), src, req)
			if err != nil {
				return err
			}
			// The answer renders itself (host.ContextAnswer implements
			// output.TextFormatter), so the markdown a reader sees here is the
			// body the MCP resource serves, and --json is the same document the
			// resource's application/json rendering carries.
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("profile", "", "answer for a named profile instead of a location")
	cmd.Flags().StringP("locale", "l", "", "narrow the reported terms to one language")
	cmd.Flags().Int("limit", host.DefaultContextTermsLimit, "max terms to render")
	// The same project- and store-resolution flags every other project verb
	// carries, so this command resolves the same way `terms` and `memory` do.
	// The output format axis (--json among them) is persistent on the root.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)

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
terminology match and a wording match are not scored on comparable things.

Each term also reports how often the project's extracted content uses it. The
count is read from the context graph that extraction writes, so it is as of the
last extraction (the last "kapi up") rather than of the working tree: a term
added to the store since then shows no uses until the next run.`,
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
	// `terms` and `memory` — a parity break inside the CLI, before MCP even
	// enters it — and it could only ever be pointed at a project by standing in
	// it, which no run under the isolation contract can do.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
	return cmd
}
