package commands

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

var streamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Manage streams",
	Long:  `Create, list, merge, diff, and archive streams for content branching.`,
}

var streamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List project streams",
	RunE:  runStreamList,
}

var streamCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new stream",
	Args:  cobra.ExactArgs(1),
	RunE:  runStreamCreate,
}

var streamDiffCmd = &cobra.Command{
	Use:   "diff <stream>",
	Short: "Show differences between stream and parent",
	Args:  cobra.ExactArgs(1),
	RunE:  runStreamDiff,
}

var streamMergeCmd = &cobra.Command{
	Use:   "merge <stream>",
	Short: "Merge stream into its parent",
	Args:  cobra.ExactArgs(1),
	RunE:  runStreamMerge,
}

var streamArchiveCmd = &cobra.Command{
	Use:   "archive <stream>",
	Short: "Archive a stream",
	Args:  cobra.ExactArgs(1),
	RunE:  runStreamArchive,
}

func init() {
	streamListCmd.Flags().Bool("all", false, "Include archived streams")
	streamCreateCmd.Flags().String("parent", "main", "Parent stream to branch from")
	streamCreateCmd.Flags().String("visibility", "public", "Stream visibility (public, private, shared)")
	streamCreateCmd.Flags().String("description", "", "Stream description")
	streamMergeCmd.Flags().Bool("dry-run", false, "Show what would be merged without merging")

	streamCmd.AddCommand(streamListCmd)
	streamCmd.AddCommand(streamCreateCmd)
	streamCmd.AddCommand(streamDiffCmd)
	streamCmd.AddCommand(streamMergeCmd)
	streamCmd.AddCommand(streamArchiveCmd)
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(streamCmd) })
}

func runStreamList(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
	}
	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	includeArchived, _ := cmd.Flags().GetBool("all")
	streams, err := conn.Client().ListStreams(cmd.Context(), includeArchived)
	if err != nil {
		return err
	}

	out := output.StreamListOutput{Streams: make([]output.StreamEntry, 0, len(streams)+1)}
	// Always include "main" as implicit first entry.
	out.Streams = append(out.Streams, output.StreamEntry{
		Name:       "main",
		Visibility: "public",
		Active:     conn.Stream() == "main" || conn.Stream() == "",
	})
	for _, s := range streams {
		out.Streams = append(out.Streams, output.StreamEntry{
			Name:        s.Name,
			Parent:      s.Parent,
			Visibility:  s.Visibility,
			Description: s.Description,
			Archived:    s.Archived,
			Active:      conn.Stream() == s.Name,
		})
	}
	return output.Print(cmd, out)
}

func runStreamCreate(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
	}
	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	parent, _ := cmd.Flags().GetString("parent")
	visibility, _ := cmd.Flags().GetString("visibility")
	description, _ := cmd.Flags().GetString("description")

	stream, err := conn.Client().CreateStream(cmd.Context(), client.CreateStreamRequest{
		Name:        args[0],
		Parent:      parent,
		Visibility:  visibility,
		Description: description,
	})
	if err != nil {
		return err
	}

	out := output.StreamCreateOutput{
		Name:        stream.Name,
		Parent:      stream.Parent,
		Visibility:  stream.Visibility,
		Description: stream.Description,
	}
	return output.Print(cmd, out)
}

func runStreamDiff(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
	}
	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	diff, err := conn.Client().DiffStream(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	changes := make([]output.DiffChangeEntry, len(diff.Changes))
	for i, c := range diff.Changes {
		changes[i] = output.DiffChangeEntry{
			BlockID:    c.BlockID,
			ChangeType: c.ChangeType,
		}
	}

	out := output.StreamDiffOutput{
		Stream:  diff.StreamName,
		Parent:  diff.ParentName,
		Changes: changes,
	}
	return output.Print(cmd, out)
}

func runStreamMerge(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
	}
	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	result, err := conn.Client().MergeStream(cmd.Context(), args[0], dryRun)
	if err != nil {
		return err
	}

	out := output.StreamMergeOutput{
		Stream:         args[0],
		MergedBlocks:   result.MergedBlocks,
		AddedBlocks:    result.AddedBlocks,
		ModifiedBlocks: result.ModifiedBlocks,
		RemovedBlocks:  result.RemovedBlocks,
		DryRun:         dryRun,
	}
	return output.Print(cmd, out)
}

func runStreamArchive(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
	}
	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.Client().ArchiveStream(cmd.Context(), args[0]); err != nil {
		return err
	}

	out := output.StreamArchiveOutput{Stream: args[0]}
	return output.Print(cmd, out)
}
