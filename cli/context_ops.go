package cli

import (
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// The decision half of the context surface (AD C-11).
//
// A project's context grows out of ordinary work. Something is proposed, a
// person decides about it, and both statements are recorded so either can be
// looked at again or taken back. These six verbs are that loop from the command
// line: propose, log, confirm, discard, revert, widen.
//
// A proposal advises from the moment it is recorded. A check reports it and no
// check fails on it. Confirming is what makes it bind, and confirming writes
// the rule into the committed source the recipe binds, so the change shows up
// in `git diff` like any other.

func newContextProposeCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose <term>",
		Short: "Propose a rule about a word, for someone to confirm",
		Long: `Propose a rule about a word: write this instead of that.

A proposal takes effect at once as advice. Checks report it wherever the word
appears, and no check fails on it, so a proposal never stops anyone's work. It
starts binding when someone confirms it with "kapi context confirm".

Say where you saw the word with --seen-in. A rule with evidence behind it can be
argued with later; a rule without any is a preference somebody typed.

Propose a rule for the voice profile's vocabulary with --list, and one for the
project's terms without it.`,
		Example: "  kapi context propose utilise --use use\n" +
			"  kapi context propose utilise --use use --seen-in docs/guide.md\n" +
			"  kapi context propose Ripgrep --use ripgrep --list forbidden --severity major",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			use, _ := cmd.Flags().GetString("use")
			severity, _ := cmd.Flags().GetString("severity")
			note, _ := cmd.Flags().GetString("note")
			list, _ := cmd.Flags().GetString("list")
			seenIn, _ := cmd.Flags().GetStringSlice("seen-in")
			quote, _ := cmd.Flags().GetString("quote")

			rule := coreprofile.TermRule{Term: args[0], Replacement: use, Severity: severity, Note: note}
			req := host.ContextProposeRequest{
				Project:  projectPath,
				Evidence: evidenceFrom(seenIn, quote),
				Note:     note,
			}
			if list != "" {
				req.Voice = &contextop.VoiceRule{List: list, Rule: rule}
			} else {
				req.Term = &rule
			}
			res, err := a.ProposeContextRule(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("use", "", "what to write instead")
	cmd.Flags().String("severity", "", "how hard the rule bites once confirmed: minor or neutral report, anything else fails a check")
	cmd.Flags().String("list", "", "propose a voice-profile rule in this list instead of a project term: forbidden, competitor or preferred")
	cmd.Flags().String("note", "", "why")
	cmd.Flags().StringSlice("seen-in", nil, "a file the word was seen in (repeatable)")
	cmd.Flags().String("quote", "", "the wording it was seen in")
	AddProjectFlag(cmd)
	return cmd
}

func newContextLogCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show how this project's context came to be",
		Long: `List what has been proposed, corrected and decided about this
project's context, newest first.

Each line carries an operation's id, when it happened, who did it, what it was
about and where it stands: a candidate nobody has decided on, a confirmed rule
in force, or one that was discarded or reverted.

The id is what the other verbs take. Narrow the list with --status to see what
is waiting for a decision, or with --session to see what one agent run did.`,
		Example: "  kapi context log\n" +
			"  kapi context log --status candidate\n" +
			"  kapi context log --session 0ab4e399 --json\n" +
			"  kapi context log --actor agent --since 2026-09-01",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			session, _ := cmd.Flags().GetString("session")
			status, _ := cmd.Flags().GetString("status")
			actor, _ := cmd.Flags().GetString("actor")
			since, _ := cmd.Flags().GetString("since")
			subjects, _ := cmd.Flags().GetBool("subjects")
			limit, _ := cmd.Flags().GetInt("limit")

			req := host.ContextLogRequest{
				Project:  projectPath,
				Session:  session,
				Status:   contextop.Status(status),
				Actor:    actor,
				Subjects: subjects,
				Limit:    limit,
			}
			if status != "" && !req.Status.Valid() {
				return fmt.Errorf("--status takes one of %v, not %q", contextop.Statuses, status)
			}
			if since != "" {
				at, perr := parseSince(since)
				if perr != nil {
					return perr
				}
				req.Since = at
			}
			res, err := a.ContextOperations(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("session", "", "only what one agent session recorded")
	cmd.Flags().String("status", "", "only operations at one status: candidate, confirmed, discarded or reverted")
	cmd.Flags().String("actor", "", "only one actor, by name or by kind")
	cmd.Flags().String("since", "", "only what happened after a date (2006-01-02) or instant")
	cmd.Flags().Bool("subjects", false, "only what was proposed, leaving out the decisions about it")
	cmd.Flags().Int("limit", 0, "keep the most recent N (0 keeps them all)")
	AddProjectFlag(cmd)
	return cmd
}

func newContextConfirmCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "confirm <id>",
		Short: "Make a proposed rule binding",
		Long: `Confirm a proposal, which makes its rule binding at the severity it
carries and writes it where the rest of kapi reads it: the project's terms, its
voice profile, or its content memory.

The write goes into the committed file the recipe binds, so "git diff" shows the
decision like any other change.

Edit the rule as you confirm it with --use and --severity, and widen it past the
point its evidence was seen at with --widen-to.`,
		Example: "  kapi context confirm 7\n" +
			"  kapi context confirm 7 --use \"content memory\"\n" +
			"  kapi context confirm 7 --severity critical\n" +
			"  kapi context confirm 7 --widen-to workspace",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			use, _ := cmd.Flags().GetString("use")
			severity, _ := cmd.Flags().GetString("severity")
			widenTo, _ := cmd.Flags().GetString("widen-to")
			note, _ := cmd.Flags().GetString("note")
			res, err := a.ConfirmContextOperation(cmd.Context(), host.ContextConfirmRequest{
				Project:     projectPath,
				ID:          args[0],
				Replacement: use,
				Severity:    severity,
				WidenTo:     widenTo,
				Note:        note,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("use", "", "change what the rule says to write instead")
	cmd.Flags().String("severity", "", "change how hard the rule bites: minor or neutral report, anything else fails a check")
	cmd.Flags().String("widen-to", "", "widen as you confirm: \"workspace\", or the name of an axis the rule should stop being specific about")
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

func newContextDiscardCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discard <id>",
		Short: "Reject a proposed rule",
		Long: `Discard a proposal. It stops being reported at once, and the record
of it stays in the log so the same suggestion can be recognised next time.

Discarding a rule that was already confirmed takes it back out of the project's
terms, voice profile or content memory, and out of the committed file it was
written to.`,
		Example: "  kapi context discard 7\n" +
			"  kapi context discard 7 --note \"we say it both ways on purpose\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			note, _ := cmd.Flags().GetString("note")
			res, err := a.DiscardContextOperation(cmd.Context(), host.ContextDiscardRequest{
				Project: projectPath,
				ID:      args[0],
				Note:    note,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

func newContextRevertCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revert [id]",
		Short: "Undo one operation, or everything one session did",
		Long: `Undo an operation. Whatever it put in force stops answering, and a
rule it got written into the project's terms, voice profile or content memory is
taken back out.

With --session it undoes everything one agent run recorded, which puts the
project's answers back where they were before that run started.

Nothing is erased. The undone operations stay in the log, marked reverted.`,
		Example: "  kapi context revert 7\n" +
			"  kapi context revert --session 0ab4e399",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			session, _ := cmd.Flags().GetString("session")
			note, _ := cmd.Flags().GetString("note")
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			res, err := a.RevertContextOperations(cmd.Context(), host.ContextRevertRequest{
				Project: projectPath,
				ID:      id,
				Session: session,
				Note:    note,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("session", "", "undo everything one agent session recorded")
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

func newContextWidenCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "widen <id>",
		Short: "Put a confirmed rule in force somewhere broader",
		Long: `Widen a confirmed rule past the point its evidence was seen at.

A rule learned in one place holds in that place. Widening says it holds more
widely: "--to workspace" puts it in force in every project you work on here, and
naming an axis drops that axis from the rule's point so it stops being specific
about it.

A project that has its own decision about the word keeps it. The more specific
answer always wins.`,
		Example: "  kapi context widen 7 --to workspace\n" +
			"  kapi context widen 7 --to mode",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			to, _ := cmd.Flags().GetString("to")
			if to == "" {
				return fmt.Errorf("widen needs --to: %q, or the name of an axis to widen past", host.WidenToWorkspace)
			}
			note, _ := cmd.Flags().GetString("note")
			res, err := a.WidenContextOperation(cmd.Context(), host.ContextWidenRequest{
				Project: projectPath,
				ID:      args[0],
				To:      to,
				Note:    note,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("to", "", "\"workspace\", or the name of an axis the rule should stop being specific about")
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

// evidenceFrom turns the --seen-in paths and the --quote into evidence. A quote
// with no path is evidence of wording nobody can find again, so it rides on the
// first path when there is one and stands alone when there is not.
func evidenceFrom(paths []string, quote string) []contextop.Evidence {
	if len(paths) == 0 {
		if quote == "" {
			return nil
		}
		return []contextop.Evidence{{Quote: quote}}
	}
	out := make([]contextop.Evidence, 0, len(paths))
	for i, path := range paths {
		e := contextop.Evidence{Path: path}
		if i == 0 {
			e.Quote = quote
		}
		out = append(out, e)
	}
	return out
}

// parseSince reads a --since value as a bare date or a full instant. A bare
// date is midnight UTC, the same reading a recipe's validity bounds take.
func parseSince(value string) (time.Time, error) {
	if at, err := time.Parse("2006-01-02", value); err == nil {
		return at, nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since takes a date (2006-01-02) or an instant (2006-01-02T15:04:05Z), not %q", value)
	}
	return at, nil
}
