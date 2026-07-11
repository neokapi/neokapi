package cli

import "github.com/spf13/cobra"

// NewHookCmd creates the hidden `kapi hook` group: integration glue for AI
// coding assistants. The commands read an assistant's hook payload on stdin and
// emit the assistant's expected response on stdout; they are not part of a
// human workflow, so the group is hidden from help.
func NewHookCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "hook",
		Short:   "Integration hooks for AI coding assistants (Claude Code)",
		GroupID: "advanced",
		Hidden:  true,
		Long: `Glue commands for AI coding assistants. Each subcommand reads the
assistant's hook payload on stdin and writes the assistant's expected response
on stdout. These are wired up by the assistant's plugin, not run by hand.`,
	}
	cmd.AddCommand(newHookStopCmd(a))
	cmd.AddCommand(newHookPreEditCmd(a))
	return cmd
}

// newHookStopCmd implements the Claude Code `Stop` hook: it runs the current
// project's `kapi check --ship` gates and, when they fail, tells Claude to keep
// working (with the findings) instead of finishing. This turns the verify gate
// into a hard guardrail — the assistant cannot end a turn with a project that
// is off-brand, off-terminology, or with broken placeholders.
func newHookStopCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Claude Code Stop hook: keep working until the project's kapi check --ship gates pass",
		Long: `Claude Code Stop hook. Reads the Stop-event JSON on stdin, runs the
verify gates for the project in the session's working directory, and:

  - emits nothing (exit 0) when the project passes, when there is no .kapi
    project, or when verify cannot run — Claude is free to finish; or
  - emits {"decision":"block","reason":"…findings…"} (exit 0) when a gate
    fails, so Claude keeps working and fixes the findings before stopping.

Wire it up via the kapi Claude Code plugin (hooks/hooks.json). It fails open:
anything other than a clean gate failure lets Claude stop, so a missing project
or a verify error never traps the assistant. Claude Code caps consecutive
blocks, so an unfixable finding cannot loop forever.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunHookStop(cmd)
		},
	}
}

// newHookPreEditCmd implements the Claude Code `PreToolUse` hook for Edit/Write:
// it denies a write to a file that the current project generates as a
// translation target (a `kapi merge` output), steering Claude back through the
// extract → translate → merge round-trip. Hand-editing a target is discarded on
// the next merge and skips the terminology, placeholder, and brand-voice gates,
// so the guard turns the skill's "don't hand-translate files" guidance into a
// hard rule.
func newHookPreEditCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "pre-edit",
		Short: "Claude Code PreToolUse hook: block direct edits to kapi-generated translation targets",
		Long: `Claude Code PreToolUse hook for Edit/Write/MultiEdit. Reads the
PreToolUse-event JSON on stdin, resolves the .kapi project in the session's
working directory, and:

  - emits {"hookSpecificOutput":{"permissionDecision":"deny",…}} (exit 0) when
    the file being written is a project content target (a path that
    "kapi merge" generates), so Claude routes the change through
    extract → translate → merge instead; or
  - emits nothing (exit 0) otherwise — outside a project, on source files, on
    files the project does not generate, or when the project cannot be loaded —
    leaving the normal permission flow untouched.

Wire it up via the kapi Claude Code plugin (hooks/hooks.json) with a matcher of
Edit|Write|MultiEdit. It fails open: anything other than a confirmed target
match lets the edit proceed, so the guard never traps the assistant on an
unrelated file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunHookPreEdit(cmd)
		},
	}
}
