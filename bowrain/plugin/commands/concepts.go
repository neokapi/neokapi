package commands

import (
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

// newConceptsCmd builds the `kapi concepts` command tree. Flags are bound to
// locals so the tree is self-contained — a fresh tree carries no state from a
// previous build (which keeps it testable).
func newConceptsCmd() *cobra.Command {
	conceptsCmd := &cobra.Command{
		Use:   "concepts",
		Short: "Browse the workspace brand knowledge graph",
		Long: `Read the governed concepts of the workspace brand knowledge graph.

Concepts are the language-neutral units of the graph: each carries terms across
locales with a lifecycle status (preferred, admitted, deprecated, forbidden, …),
typed relations to other concepts, and a timeline of how it came to be. These
commands read the graph through the bowrain server; the project must be claimed
into a workspace and you must be authenticated.`,
	}

	var (
		status string
		domain string
		market string
		query  string
		limit  int
	)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List concepts in the workspace graph",
		Long: `List concepts in the workspace brand knowledge graph.

Narrow the result with the term-status, domain, and market facets, or with a
free-text query against the term text.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			result, err := client.ListConcepts(cmd.Context(), apiclient.ListConceptsParams{
				Query:  query,
				Status: status,
				Domain: domain,
				Market: market,
				Limit:  limit,
			})
			if err != nil {
				return err
			}

			out := output.ConceptListOutput{
				Shown:      len(result.Concepts),
				TotalCount: result.TotalCount,
			}
			for _, c := range result.Concepts {
				out.Concepts = append(out.Concepts, output.ConceptListEntry{
					ID:         c.ID,
					Domain:     c.Domain,
					Definition: c.Definition,
					Terms:      conceptTerms(c.Terms),
				})
			}
			return output.Print(cmd, out)
		},
	}
	listCmd.Flags().StringVar(&status, "status", "", "filter by term lifecycle status (preferred, admitted, deprecated, forbidden, …)")
	listCmd.Flags().StringVar(&domain, "domain", "", "filter by subject-field domain")
	listCmd.Flags().StringVar(&market, "market", "", "filter by market validity tag")
	listCmd.Flags().StringVar(&query, "q", "", "free-text query against the term text")
	listCmd.Flags().IntVar(&limit, "limit", 50, "maximum number of concepts to return")

	showCmd := &cobra.Command{
		Use:   "show <concept-id>",
		Short: "Show a concept's terms, status, and relations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			concept, err := client.GetConcept(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			out := output.ConceptShowOutput{
				ID:         concept.ID,
				ProjectID:  concept.ProjectID,
				Domain:     concept.Domain,
				Definition: concept.Definition,
				Properties: concept.Properties,
				Terms:      conceptTerms(concept.Terms),
				CreatedAt:  concept.CreatedAt,
				UpdatedAt:  concept.UpdatedAt,
			}

			// Relations are a separate endpoint; surface them when present, but
			// don't fail the whole command if only the relation lookup errors.
			if relations, relErr := client.ListConceptRelations(cmd.Context(), args[0], "", ""); relErr == nil {
				for _, r := range relations {
					out.Relations = append(out.Relations, output.ConceptRelationEntry{
						Type:     r.RelationType,
						SourceID: r.SourceID,
						TargetID: r.TargetID,
						Note:     r.Note,
					})
				}
			}

			return output.Print(cmd, out)
		},
	}

	storyCmd := &cobra.Command{
		Use:   "story <concept-id>",
		Short: "Show a concept's timeline",
		Long: `Show a concept's merged chronological timeline (oldest first).

The story interleaves revisions, observations, comments, and the change-sets
that touched the concept, so you can see how the governed term came to be.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			story, err := client.GetConceptStory(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			out := output.ConceptStoryOutput{ConceptID: story.ConceptID}
			for _, e := range story.Entries {
				out.Entries = append(out.Entries, output.ConceptStoryItem{
					Kind:    e.Kind,
					At:      e.At,
					Actor:   e.Actor,
					Summary: e.Summary,
					Ref:     e.Ref,
				})
			}
			return output.Print(cmd, out)
		},
	}

	conceptsCmd.AddCommand(listCmd, showCmd, storyCmd)
	return conceptsCmd
}

// conceptTerms maps client term DTOs to the shared output term type.
func conceptTerms(terms []apiclient.TermInfo) []output.ConceptTerm {
	out := make([]output.ConceptTerm, 0, len(terms))
	for _, t := range terms {
		out = append(out, output.ConceptTerm{
			Text:   t.Text,
			Locale: t.Locale,
			Status: t.Status,
		})
	}
	return out
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(newConceptsCmd()) })
}
