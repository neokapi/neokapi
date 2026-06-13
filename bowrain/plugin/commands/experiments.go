package commands

import (
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

// newExperimentsCmd builds the `kapi experiments` command tree (change-sets of
// the workspace brand knowledge graph). Flags are bound to locals so a fresh
// tree carries no state from a previous build.
func newExperimentsCmd() *cobra.Command {
	experimentsCmd := &cobra.Command{
		Use:   "experiments",
		Short: "Inspect brand knowledge-graph change-sets",
		Long: `Inspect change-sets — reviewable drafts of edits to the workspace brand
knowledge graph and vocabulary.

A change-set moves through draft → in_review → approved → merged (or abandoned).
Before it merges you can preview its blast radius: how many stored blocks it
would newly flag or resolve across projects. These commands read the change-sets
through the bowrain server; the project must be claimed into a workspace and you
must be authenticated.`,
	}

	var status string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List change-sets in the workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			changesets, err := client.ListChangesets(cmd.Context(), status)
			if err != nil {
				return err
			}

			out := output.ExperimentListOutput{}
			for _, cs := range changesets {
				out.Experiments = append(out.Experiments, output.ExperimentEntry{
					ID:          cs.ID,
					Name:        cs.Name,
					Status:      cs.Status,
					CreatedBy:   cs.CreatedBy,
					CreatedAt:   cs.CreatedAt,
					SubmittedAt: cs.SubmittedAt,
					MergedAt:    cs.MergedAt,
				})
			}
			return output.Print(cmd, out)
		},
	}
	listCmd.Flags().StringVar(&status, "status", "", "filter by change-set status (draft, in_review, approved, merged, abandoned)")

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a change-set's operations, reviews, and pilots",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			detail, err := client.GetChangeset(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			out := output.ExperimentShowOutput{
				ID:          detail.ID,
				Name:        detail.Name,
				Description: detail.Description,
				Status:      detail.Status,
				Governed:    detail.Governed,
				CreatedBy:   detail.CreatedBy,
				CreatedAt:   detail.CreatedAt,
				SubmittedAt: detail.SubmittedAt,
				MergedAt:    detail.MergedAt,
			}
			for _, op := range detail.Ops {
				out.Ops = append(out.Ops, output.ExperimentOp{Seq: op.Seq, Op: op.Op})
			}
			for _, r := range detail.Reviews {
				out.Reviews = append(out.Reviews, output.ExperimentReview{
					Reviewer: r.Reviewer,
					Verdict:  r.Verdict,
					Comment:  r.Comment,
				})
			}
			for _, p := range detail.Pilots {
				out.Pilots = append(out.Pilots, output.ExperimentPilot{
					ProjectID: p.ProjectID,
					Stream:    p.Stream,
				})
			}
			return output.Print(cmd, out)
		},
	}

	blastRadiusCmd := &cobra.Command{
		Use:   "blast-radius <id>",
		Short: "Preview a change-set's blast radius over stored content",
		Long: `Preview a change-set's blast radius without persisting anything.

Reports how many stored (block, locale) rows the draft would newly flag or
resolve, with a source word count as a re-translation effort proxy, broken down
per project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := knowledgeClient()
			if err != nil {
				return err
			}

			impact, err := client.GetChangesetBlastRadius(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			out := output.ExperimentBlastRadiusOutput{
				ChangesetID:    args[0],
				TotalBlocks:    impact.TotalBlocks,
				AffectedBlocks: impact.AffectedBlocks,
				NewViolations:  impact.NewViolations,
				Resolved:       impact.Resolved,
				Words:          impact.Words,
			}
			for _, p := range impact.Projects {
				out.Projects = append(out.Projects, output.BlastRadiusProject{
					ProjectID:      p.ProjectID,
					ProjectName:    p.ProjectName,
					AffectedBlocks: p.AffectedBlocks,
					NewViolations:  p.NewViolations,
					Resolved:       p.Resolved,
					Words:          p.Words,
				})
			}
			return output.Print(cmd, out)
		},
	}

	experimentsCmd.AddCommand(listCmd, showCmd, blastRadiusCmd)
	return experimentsCmd
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(newExperimentsCmd()) })
}
