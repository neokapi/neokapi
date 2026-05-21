package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/cli/skills"
	"github.com/spf13/cobra"
)

// NewSkillsCmd creates the `kapi skills` command group — install, list, and
// uninstall the bundled Agent Skills that wrap kapi/bowrain for AI coding
// assistants. Skills are embedded in the binary (the single source of truth),
// so install works offline and is byte-identical across distribution paths.
func (a *App) NewSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skills",
		Short:   "Install Agent Skills that plug kapi into your AI coding assistant",
		GroupID: "management",
		Long: `Manage the bundled Agent Skills.

These SKILL.md definitions teach an AI coding assistant (Claude Code, etc.) how
to use kapi to keep generated content on-brand, terminologically consistent, and
to publish multilingually. The kapi-* skills drive the local CLI; the bowrain-*
skills drive the governed Bowrain platform.`,
	}
	cmd.AddCommand(a.newSkillsListCmd(), a.newSkillsInstallCmd(), a.newSkillsUninstallCmd(), a.newSkillsExportCmd())
	return cmd
}

func (a *App) newSkillsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the bundled Agent Skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := skills.List()
			if err != nil {
				return err
			}
			out := output.SkillsListOutput{}
			for _, s := range all {
				out.Skills = append(out.Skills, output.SkillEntry{
					Name: s.Name, Family: s.Family, Description: s.Description,
				})
			}
			out.Total = len(out.Skills)
			return output.Print(cmd, out)
		},
	}
	output.AddFlags(cmd)
	return cmd
}

func (a *App) newSkillsInstallCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "install [names...]",
		Short: "Install skills into .claude/skills (project or user scope)",
		Long: `Install the bundled skills into a .claude/skills directory.

  --target project  (default)  → ./.claude/skills/<name>/SKILL.md
  --target user                → ~/.claude/skills/<name>/SKILL.md

Pass skill names to install a subset; omit to install all.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := skillsBaseDir(target)
			if err != nil {
				return err
			}
			written, err := skills.InstallTo(base, args)
			if err != nil {
				return err
			}
			out := output.SkillsInstallOutput{Target: target, Dir: base}
			for _, p := range written {
				out.Installed = append(out.Installed, p)
			}
			out.Total = len(written)
			return output.Print(cmd, out)
		},
	}
	cmd.Flags().StringVar(&target, "target", "project", "install scope: project or user")
	output.AddFlags(cmd)
	return cmd
}

func (a *App) newSkillsUninstallCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "uninstall [names...]",
		Short: "Remove installed skills from .claude/skills",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := skillsBaseDir(target)
			if err != nil {
				return err
			}
			var removed []string
			for _, name := range args {
				dir := filepath.Join(base, name)
				if err := os.RemoveAll(dir); err != nil {
					return fmt.Errorf("remove %s: %w", dir, err)
				}
				removed = append(removed, dir)
			}
			return output.Print(cmd, output.SkillsInstallOutput{
				Target: target, Dir: base, Installed: removed, Total: len(removed),
			})
		},
	}
	cmd.Flags().StringVar(&target, "target", "project", "scope: project or user")
	output.AddFlags(cmd)
	return cmd
}

// newSkillsExportCmd writes every skill into a directory (used by the plugin
// bundle build target so all distribution paths are byte-identical).
func (a *App) newSkillsExportCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:    "export",
		Short:  "Export all skills to a directory (for the plugin bundle)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return fmt.Errorf("--dir is required")
			}
			written, err := skills.InstallTo(dir, nil)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.SkillsInstallOutput{
				Target: "export", Dir: dir, Installed: written, Total: len(written),
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "output directory")
	output.AddFlags(cmd)
	return cmd
}

// skillsBaseDir resolves the .claude/skills directory for the given target.
func skillsBaseDir(target string) (string, error) {
	switch target {
	case "", "project":
		return filepath.Join(".claude", "skills"), nil
	case "user":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, ".claude", "skills"), nil
	default:
		return "", fmt.Errorf("invalid --target %q (use project or user)", target)
	}
}
