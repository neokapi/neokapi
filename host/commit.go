package host

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
)

// RunCommit writes staged decisions into the project's committed record.
//
// The counterpart to `kapi status`: status shows what is staged, commit makes it
// part of the record a reviewer reads and git tracks. Recording a decision and
// publishing it are separate acts on purpose — a run of automated approvals
// should not land in the tracked record before anyone has looked at it.
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
	if res.Reseeded {
		writeRecordReseeded(out, res.Carried)
	}
	if res.Committed == 0 {
		fmt.Fprintln(out, "Nothing staged. The committed record is up to date.")
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

// writeRecordReseeded reports a committed record that moved under the working
// set, which is what a branch switch does to it, and how many staged decisions
// crossed. Callers print it only when the set was rebuilt.
//
// The count is the part a person acts on: those decisions were made against
// another record and are about to be written into this one.
func writeRecordReseeded(out io.Writer, carried int) {
	const moved = "The committed record changed since the working set was seeded from it; " +
		"the working set was rebuilt from the record this checkout holds"
	if carried == 0 {
		fmt.Fprintln(out, moved+".")
		return
	}
	fmt.Fprintf(out, "%s, carrying %s across.\n", moved, pluralStagedDecisions(carried))
}

func pluralStagedDecisions(n int) string {
	if n == 1 {
		return "1 staged decision"
	}
	return fmt.Sprintf("%d staged decisions", n)
}

func pluralUnitStateChanges(n int) string {
	if n == 1 {
		return "1 unit-state change"
	}
	return fmt.Sprintf("%d unit-state changes", n)
}
