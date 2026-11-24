package cli

import "github.com/spf13/cobra"

// AnnotationProject is the cobra annotation by which a command declares
// whether it concerns the project it is run in. The root command's prerun
// reads the declaration (ProjectDeclaration): it resolves a project, and gives
// the one-time notices about it, only for a command that concerns one.
const AnnotationProject = "kapi.project"

// The values of AnnotationProject.
const (
	projectUses = "uses"
	projectNone = "none"
)

// UsesProject declares that cmd concerns the project it is run in although it
// takes no -p: it finds the project from the working directory. It returns
// cmd.
func UsesProject(cmd *cobra.Command) *cobra.Command {
	return declareProject(cmd, projectUses)
}

// NoProject declares that cmd, and each subcommand that declares nothing
// itself, resolves no project. It returns cmd.
func NoProject(cmd *cobra.Command) *cobra.Command {
	return declareProject(cmd, projectNone)
}

func declareProject(cmd *cobra.Command, value string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AnnotationProject] = value
	return cmd
}

// ProjectDeclaration reports whether cmd concerns a project, and whether it
// declares either way. The first of these answers:
//
//  1. cmd's own AnnotationProject.
//  2. cmd takes -p (AddProjectFlag, or a parent's persistent -p): it concerns
//     a project.
//  3. cmd is served by a plugin: it resolves no project in kapi, because the
//     plugin resolves its project in its own process.
//  4. the AnnotationProject of cmd's nearest parent that has one. The root's
//     declaration covers bare `kapi` only.
//
// cobra's shell completion request command (`__complete`, also called as
// `__completeNoDesc`) resolves no project. cobra builds it inside Execute and
// adds it to the root only when it is the command being run, after every hook
// that could annotate it, so it is answered here by name.
//
// A command that declares nothing concerns no project, so the prerun resolves
// none for it. The command tree test fails on such a command.
func ProjectDeclaration(cmd *cobra.Command) (uses, declared bool) {
	if cmd == nil {
		return false, false
	}
	if v, ok := cmd.Annotations[AnnotationProject]; ok {
		return v == projectUses, true
	}
	if cmd.Name() == cobra.ShellCompRequestCmd && cmd.HasParent() && !cmd.Parent().HasParent() {
		return false, true
	}
	if cmd.Flags().Lookup(projectFlagName) != nil || cmd.InheritedFlags().Lookup(projectFlagName) != nil {
		return true, true
	}
	if cmd.Annotations[pluginAnnotation] != "" {
		return false, true
	}
	for p := cmd.Parent(); p != nil && p.HasParent(); p = p.Parent() {
		if v, ok := p.Annotations[AnnotationProject]; ok {
			return v == projectUses, true
		}
	}
	return false, false
}

// projectFlagName is the long name of the flag AddProjectFlag registers.
const projectFlagName = "project"

// pluginAnnotation names the plugin that serves a command (cli/pluginattach).
const pluginAnnotation = "plugin"
