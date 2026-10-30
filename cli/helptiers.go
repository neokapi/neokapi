package cli

import (
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/project"
)

// helpTier says which audience a root command group serves. The root help
// shows the person tier always, the languages tier when the project declares
// a target language, and the engine tier only in `kapi help --all`.
//
// The tier is a property of the renderer. No command is marked Hidden for it:
// a hidden command leaves shell completion, the published CLI reference and
// the help-string catalog, and every command here stays in all three.
type helpTier int

const (
	tierPerson helpTier = iota
	tierLanguages
	tierEngine
)

// groupTiers maps each root group ID (AddCommandGroups) to its tier. A group
// absent from the map renders at the person tier, so a new group is shown
// until someone assigns it.
var groupTiers = map[string]helpTier{
	"work":      tierPerson,
	"assets":    tierPerson,
	"languages": tierLanguages,
	"advanced":  tierEngine,
}

// languagesGroupID is the root group `kapi help languages` prints.
const languagesGroupID = "languages"

// rootDefaultFlags are the global flags the default root help lists. The
// others work on every command; `kapi help --all` lists them on the root, and
// each command's own --help lists them under "Global Flags:".
var rootDefaultFlags = map[string]bool{
	"help":    true,
	"version": true,
	"json":    true,
	"quiet":   true,
	"verbose": true,
	"yes":     true,
	"color":   true,
}

// leadCommands are listed first in their group on the root help, in this
// order. The rest of each group follows in cobra's alphabetical order.
var leadCommands = []string{"look"}

// ordered returns the root's commands with leadCommands first. A subcommand's
// help lists its commands as cobra sorts them.
func (s usageScope) ordered(cmds []*cobra.Command) []*cobra.Command {
	if !s.tiered {
		return cmds
	}
	out := make([]*cobra.Command, 0, len(cmds))
	for _, name := range leadCommands {
		for _, c := range cmds {
			if c.Name() == name {
				out = append(out, c)
			}
		}
	}
	for _, c := range cmds {
		if !slices.Contains(leadCommands, c.Name()) {
			out = append(out, c)
		}
	}
	return out
}

// helpView selects how much of the root a help render shows.
type helpView struct {
	// all shows every tier and every global flag (`kapi help --all`).
	all bool
}

// usageScope is a helpView resolved against the command being rendered. Only
// the root is tiered; a subcommand's help lists everything it has.
type usageScope struct {
	tiered    bool
	all       bool
	languages bool
}

func scopeFor(cmd *cobra.Command, view helpView) usageScope {
	if cmd.HasParent() {
		return usageScope{}
	}
	s := usageScope{tiered: true, all: view.all}
	s.languages = view.all || commandDeclaresTargetLanguages(cmd)
	return s
}

// tierShown reports whether a tier renders in this scope.
func (s usageScope) tierShown(tier helpTier) bool {
	if !s.tiered || s.all {
		return true
	}
	switch tier {
	case tierLanguages:
		return s.languages
	case tierEngine:
		return false
	default:
		return true
	}
}

// groupShown reports whether the root group id renders in this scope.
func (s usageScope) groupShown(id string) bool {
	return s.tierShown(groupTiers[id])
}

// topicTier says which tier a help topic of the kapi skill serves. The
// translate topic and the i18n topics (the framework playbooks under the
// skill's references/i18n) are language work. Every other topic is the person
// tier, so a new topic is shown until someone assigns it.
func topicTier(name string) helpTier {
	if name == "translate" || name == "i18n" || strings.HasPrefix(name, "i18n-") {
		return tierLanguages
	}
	return tierPerson
}

// topicShown reports whether the help topic renders in this scope.
func (s usageScope) topicShown(name string) bool {
	return s.tierShown(topicTier(name))
}

// localFlagUsages renders c's Flags block. The default root view lists only
// rootDefaultFlags; every other view lists every flag. The filter builds a
// separate set for display, so no flag is marked hidden and completion still
// offers all of them.
func (s usageScope) localFlagUsages(c *cobra.Command) string {
	flags := c.LocalFlags()
	if !s.tiered || s.all {
		return flags.FlagUsages()
	}
	shown := pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
	flags.VisitAll(func(f *pflag.Flag) {
		if rootDefaultFlags[f.Name] {
			shown.AddFlag(f)
		}
	})
	return shown.FlagUsages()
}

// footerHints are the footer lines the default root help adds under cobra's
// own: where the commands it leaves out are listed. None for every other view.
func (s usageScope) footerHints(cmd *cobra.Command) []string {
	if !s.tiered || s.all {
		return nil
	}
	name := cmd.CommandPath()
	hints := []string{`Use "` + name + ` help --all" to list every command.`}
	if !s.languages {
		hints = append(hints, `Use "`+name+` help languages" to list the commands for other languages.`)
	}
	return hints
}

// commandDeclaresTargetLanguages reports whether the project cmd resolves to
// (-p, KAPI_PROJECT, discovery; KAPI_NO_PROJECT turns discovery off) names a
// target language. Any failure reads as no target language.
func commandDeclaresTargetLanguages(cmd *cobra.Command) bool {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return false
	}
	return recipeDeclaresTargetLanguages(path)
}

// recipeDeclaresTargetLanguages reads the recipe at path and reports
// project.KapiProject.DeclaresTargetLanguages. It decodes the YAML and stops
// there: project.Load would validate and report near-miss keys through
// project.OnKeyWarnings, which prints to stderr once host.App.Init has run,
// and help must never warn. Nothing else is read, so no content is scanned
// and no store is opened. Any failure reads as false.
func recipeDeclaresTargetLanguages(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var proj project.KapiProject
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return false
	}
	return proj.DeclaresTargetLanguages()
}
