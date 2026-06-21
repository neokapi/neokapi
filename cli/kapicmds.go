package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/config"
	clii18n "github.com/neokapi/neokapi/cli/i18n"
	"github.com/neokapi/neokapi/core/i18n"
)

// KapiRootShort / KapiRootLong are the canonical root help strings for the
// kapi CLI. They live here (rather than in kapi/cmd/kapi) so the cli/i18n
// generator — which cannot import the kapi module — emits them into
// commands.json under the cli.commands.kapi.* scopes.
const (
	KapiRootShort = "A format-aware content toolkit — parse, edit, check, and localize any format"
	KapiRootLong  = `kapi parses any format into one unified content model, edits the content
inside it, checks it, and writes it back byte-for-byte. At heart, both a
localization engine and the tool that keeps your source content on brand:
convert formats, translate with AI, and run quality checks across a wide range
of file types.`
)

// KapiCommandSet constructs the full built-in command set of the kapi CLI,
// in registration order. kapi/cmd/kapi attaches these to its root command;
// the cli/i18n generator walks the same set to extract help strings, so the
// two can never drift. Plugin-contributed commands are not included — they
// attach via the command factories and the plugin host.
func (a *App) KapiCommandSet() []*cobra.Command {
	var cmds []*cobra.Command

	// Primary commands.
	runCmd := a.NewRunCmd(RunCmdOptions{})
	runCmd.GroupID = "processing"
	cmds = append(cmds, runCmd)
	cmds = append(cmds, a.NewExtractCmd(ExtractCmdOptions{}))
	cmds = append(cmds, a.NewMergeCmd(MergeCmdOptions{}))

	// .klz project snapshot hand-off (AD-025 §5): pack the working state
	// into a portable .klz and rehydrate it elsewhere.
	cmds = append(cmds, a.NewPackCmd(), a.NewUnpackCmd(), a.NewInfoCmd())

	// Toolbox: format-aware cat / grep / sed, registered as hidden proxies
	// for the kcat / kgrep / ksed multi-call binaries.
	cmds = append(cmds, a.NewToolboxProxies()...)
	// rewrite is the AI-driven sibling of ksed: edit the content inside a file
	// with a plain-language instruction, faithfully, with a reviewable --diff.
	cmds = append(cmds, a.newRewriteCmd())
	cmds = append(cmds, a.NewInspectCmd())
	cmds = append(cmds,
		a.NewVerifyCmd(),
		a.NewCheckCmd(),
		a.NewHookCmd(),
		a.NewInitCmd(),
		a.NewAddCmd(),
		a.NewRmCmd(),
		a.NewLsCmd(),
	)

	// Management commands.
	cmds = append(cmds,
		a.NewFlowsCmd(FlowCmdOptions{}),
		a.NewToolsCmd(),
		a.NewFormatsCmd(),
		a.NewPluginCmd(),
		a.NewModelsCmd(),
		a.NewRegistryCmd(),
		a.NewPresetsCmd(),
		a.NewTermbaseCmd(),
		a.NewTMCmd(),
		a.NewBrandCmd(),
		a.NewSkillsCmd(),
		a.NewCredentialsCmd(),
		a.NewConfigCmd(),
		a.NewVersionCmd("kapi"),
		a.NewUpdateCmd(),
		a.NewCompletionCmd(),
	)

	// Top-level tool commands (declarative opt-in via the tool registry).
	cmds = append(cmds, a.NewToolCommands()...)

	mcpCmd := a.NewMCPCmd("kapi")
	mcpCmd.GroupID = "processing"
	cmds = append(cmds, mcpCmd)

	return cmds
}

// HelpTranslator resolves the Translator used to localize command help at
// construction time. Cobra renders --help before any hook runs, so the
// --lang flag cannot participate here: help honors KAPI_LANG, the config
// file's `language` key, and the POSIX env chain, while --lang still
// localizes metadata in command output (tools/formats listings). The config
// load is best-effort — a missing or unreadable config file simply leaves
// the env chain in charge.
func HelpTranslator() i18n.Translator {
	cfg := config.NewAppConfig()
	_ = cfg.Load()
	return clii18n.Resolve(i18n.ResolveOptions{ConfigLanguage: cfg.Language()})
}

// LocalizeCommandHelp rewrites Short/Long/Example on every command in the
// tree rooted at root through t, using the scopes the cli/i18n generator
// emits: cli.commands.<full.command.path>.{short,long,example}. The first
// path segment is always "kapi" regardless of the root command's name, so
// catalogs stay valid across binaries built on the shared CLI base. Misses
// fall back to the English source (gettext semantics), so commands the
// catalog doesn't know — plugin commands, new built-ins — stay English.
func LocalizeCommandHelp(root *cobra.Command, t i18n.Translator) {
	if root == nil || t == nil || t.Locale() == "en" {
		return
	}
	localizeCommand(root, "kapi", t)
}

func localizeCommand(c *cobra.Command, path string, t i18n.Translator) {
	scope := "cli.commands." + path
	if c.Short != "" {
		c.Short = t.T(i18n.Scope(scope+".short"), c.Short)
	}
	if c.Long != "" {
		c.Long = helpText(t, i18n.Scope(scope+".long"), c.Long)
	}
	if c.Example != "" {
		c.Example = helpText(t, i18n.Scope(scope+".example"), c.Example)
	}
	for _, sub := range c.Commands() {
		name := sub.Name()
		if name == "" || strings.ContainsAny(name, ". ") {
			// A command name with a path-separator character would corrupt
			// the scope; leave such commands (none exist today) untouched.
			continue
		}
		localizeCommand(sub, path+"."+name, t)
	}
}

// helpText translates a multi-line help string (Long / Example), guarding
// against translations that lost their line structure. The project TM's
// plain-text fast path normalizes whitespace on stored variants
// (sievepen.NormalizeText), so a TM-leveraged catalog can carry a Long whose
// line breaks were collapsed to spaces — rendering as one unreadable
// paragraph. Until the TM round-trips line structure, prefer the English
// source over a structurally damaged translation.
func helpText(t i18n.Translator, scope i18n.Scope, source string) string {
	tr := t.T(scope, source)
	if strings.Contains(source, "\n") && !strings.Contains(tr, "\n") {
		return source
	}
	return tr
}
