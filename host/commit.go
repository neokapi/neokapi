package host

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
)

// RunCommit writes this checkout's decision record to the committed shards.
//
// The record it writes is, for every unit the checkout holds, the ledger entry
// that applies to the unit's current pairing. Every decision in it is already
// durable, so the write serializes the record into the form git tracks and a
// reviewer reads in a diff.
func (a *App) RunCommit(cmd Command, _ []string) error {
	a.InitRegistries()
	if cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}

	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	// The recipe is loaded rather than used: it is what proves this is a real
	// project before the command touches its state directory.
	if _, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true}); err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	res, err := a.CommitProjectStateReport(cmd.Context(), root, dryRun)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if res.Committed == 0 {
		fmt.Fprintln(out, "The committed record is up to date.")
		return nil
	}

	layout := project.Layout{StateDir: filepath.Join(root, project.StateDirName)}
	rel, relErr := filepath.Rel(root, layout.UnitStateDir())
	if relErr != nil {
		rel = layout.UnitStateDir()
	}
	if dryRun {
		fmt.Fprintf(out, "Would commit %s to %s\n", pluralUnitStateChanges(res.Committed), rel)
		return nil
	}
	fmt.Fprintf(out, "Committed %s to %s\n", pluralUnitStateChanges(res.Committed), rel)
	return nil
}

func pluralUnitStateChanges(n int) string {
	if n == 1 {
		return "1 unit-state change"
	}
	return fmt.Sprintf("%d unit-state changes", n)
}
