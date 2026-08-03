package host

import (
	"context"
	"fmt"
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

	n, err := CommitProjectState(root)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if n == 0 {
		fmt.Fprintln(out, "Nothing staged — the committed record is up to date.")
		return nil
	}

	layout := project.Layout{StateDir: filepath.Join(root, project.StateDirName)}
	rel, relErr := filepath.Rel(root, layout.UnitsDir())
	if relErr != nil {
		rel = layout.UnitsDir()
	}
	fmt.Fprintf(out, "Committed %s to %s\n", pluralDecisions(n), rel)
	return nil
}

func pluralDecisions(n int) string {
	if n == 1 {
		return "1 decision"
	}
	return fmt.Sprintf("%d decisions", n)
}
