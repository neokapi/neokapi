package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/host/config"
	clii18n "github.com/neokapi/neokapi/host/i18n"
)

// KapiRootShort / KapiRootLong are the canonical root help strings for the
// kapi CLI. They live here (rather than in kapi/cmd/kapi) so the cli/i18n
// generator — which cannot import the kapi module — emits them into
// commands.json under the cli.commands.kapi.* scopes.
const (
	KapiRootShort = "A format-aware content engine that knows your project's context"
	KapiRootLong  = `kapi parses any format into one unified content model, edits the content
inside it, checks it, and writes it back byte-for-byte. It holds a project's
content context (the terms, the voice and the rules it goes by) so you can
convert formats, translate with AI, and run quality checks against what
actually applies.`
)

// KapiCommandSet constructs the full built-in command set of the kapi CLI,
// in registration order. kapi/cmd/kapi attaches these to its root command;
// the cli/i18n generator walks the same set to extract help strings, so the
// two can never drift. Plugin-contributed commands are not included — they
// attach via the command factories and the plugin host.
func KapiCommandSet(a *App) []*cobra.Command {
	var cmds []*cobra.Command

	// Porcelain (#1078 C1): `kapi up` brings the project up to date against its
	// ship gates; `kapi translate` / `kapi pseudo-translate` run the guardrailed
	// built-in flows over ad-hoc files; the bare `kapi run` remains as the
	// custom-flow surface.
	cmds = append(cmds, NewUpCmd(a))
	cmds = append(cmds, NewTranslateCmd(a), NewPseudoTranslateCmd(a))

	// Primary commands.
	runCmd := NewRunCmd(a, RunCmdOptions{})
	runCmd.GroupID = "advanced"
	cmds = append(cmds, runCmd)
	cmds = append(cmds, NewExtractCmd(a, ExtractCmdOptions{}))
	cmds = append(cmds, NewMergeCmd(a, MergeCmdOptions{}))

	// .kpz project snapshot hand-off (AD-025 §5): pack the working state
	// into a portable .kpz and rehydrate it elsewhere.
	cmds = append(cmds, NewPackCmd(a), NewUnpackCmd(a), NewInfoCmd(a))

	// Toolbox: format-aware cat / grep / sed, registered as hidden proxies
	// for the kcat / kgrep / ksed multi-call binaries.
	cmds = append(cmds, NewToolboxProxies(a)...)
	cmds = append(cmds, NewInspectCmd(a))
	// apply is the write sibling of inspect: land a typed change-set (content +
	// asset edits) through one reviewed write path.
	cmds = append(cmds, NewApplyCmd(a))
	cmds = append(cmds, NewStatsCmd(a))
	cmds = append(cmds,
		NewStatusCmd(a),
		NewCommitCmd(a),
		NewCheckCmd(a),
		NewHookCmd(a),
		NewInitCmd(a),
		NewAddCmd(a),
		NewRmCmd(a),
		NewLsCmd(a),
	)

	// Management commands.
	cmds = append(cmds,
		NewFlowsCmd(a, FlowCmdOptions{}),
		NewToolsCmd(a),
		NewFormatsCmd(a),
		NewPluginCmd(a),
		NewModelsCmd(a),
		NewContextCmd(a),
		NewTermsCmd(a),
		NewMemoryCmd(a),
		NewVoiceCmd(a),
		NewCredentialsCmd(a),
		NewConfigCmd(a),
		NewTelemetryCmd(a),
		NewVersionCmd(a, "kapi"),
		NewUpdateCmd(a),
		NewCompletionCmd(a),
	)

	// Top-level tool commands (declarative opt-in via the tool registry).
	cmds = append(cmds, NewToolCommands(a)...)

	mcpCmd := NewMCPCmd(a, "kapi")
	mcpCmd.GroupID = "advanced"
	cmds = append(cmds, mcpCmd)

	// The engine gRPC server is machine plumbing (the desktop and embedders
	// spawn it; humans don't type it) — routable but out of --help.
	engineCmd := NewEngineCmd(a)
	engineCmd.Hidden = true
	cmds = append(cmds, engineCmd)

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
// against translations that lost their line structure. The project content memory's
// plain-text fast path normalizes whitespace on stored variants
// (memory.NormalizeText), so a content memory-leveraged catalog can carry a Long whose
// line breaks were collapsed to spaces — rendering as one unreadable
// paragraph. Until the content memory round-trips line structure, prefer the English
// source over a structurally damaged translation.
func helpText(t i18n.Translator, scope i18n.Scope, source string) string {
	tr := t.T(scope, source)
	if strings.Contains(source, "\n") && !strings.Contains(tr, "\n") {
		return source
	}
	return tr
}
