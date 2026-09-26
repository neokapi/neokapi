package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// The decision half of the context surface (AD C-11).
//
// A project's context grows out of ordinary work. Something is noticed and
// recorded as a suggestion, a person decides about it, and both statements are
// recorded so either can be looked at again or taken back. These verbs are that
// loop from the command line: observe, correct, log, keep, drop, withdraw,
// revert, widen.
//
// observe and correct are what an agent reaches for mid-task, and they exist
// here as well as over MCP because the skill drives the command line. A habit an
// assistant can only keep on one of the two surfaces is a habit half the
// assistants do not have.
//
// A suggestion advises from the moment it is recorded. A check reports it and
// no check fails on it. A person's signal establishes it: keeping it, a
// correction toward it, or the change reaching the default branch (settle).
// Establishing writes the rule into the project's terms store, where every
// check reads it.

func newContextObserveCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observe [what you noticed]",
		Short: "Record something you noticed about how this project writes",
		Long: `Record something you noticed about how this project writes: a
product name as it spells it, a spelling it is consistent about, who its text
addresses, the register it keeps.

For a name or spelling, pass --term with the form the project uses and
--instead-of with a form it avoids. kapi derives the other forms to avoid: the
spaced, hyphenated and closed spellings of a compound, each part capitalised,
and the lower-case spelling of a capitalised term. "--term Quickcast
--instead-of 'Quick cast'" avoids Quick cast, Quick-cast, QuickCast and
quickcast. The parts of a closed word come from an --instead-of form that
breaks it, so a word with none varies only its case.

Everything recorded is a suggestion. Checks report it wherever the word
appears, no check fails on it, and it is established when a person keeps it
with "kapi context keep". Without --term an observation is a fact in prose.

Say where you saw it with --seen-in and --quote.`,
		Example: "  kapi context observe \"the docs address the reader as you\"\n" +
			"  kapi context observe --term Quickcast --instead-of \"Quick cast\" \\\n" +
			"    --seen-in README.md --quote \"Quickcast forecasts the next hour.\"\n" +
			"  kapi context observe \"we write use, never utilise\" --term use --instead-of utilise --instead-of utilize",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			text := ""
			if len(args) == 1 {
				text = args[0]
			}
			term, _ := cmd.Flags().GetString("term")
			insteadOf, _ := cmd.Flags().GetStringArray("instead-of")
			if text == "" && term == "" {
				return errors.New("say what you noticed, or name the form the project uses with --term")
			}
			seenIn, _ := cmd.Flags().GetStringSlice("seen-in")
			quote, _ := cmd.Flags().GetString("quote")
			res, err := a.RecordContextObservation(cmd.Context(), host.ContextObserveRequest{
				Project:   projectPath,
				Text:      text,
				Term:      term,
				InsteadOf: insteadOf,
				Evidence:  evidenceFrom(seenIn, quote),
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("term", "", "the form the project uses for a name or word")
	cmd.Flags().StringArray("instead-of", nil, "a form the project avoids for --term (repeatable)")
	cmd.Flags().StringSlice("seen-in", nil, "a file you saw it in (repeatable)")
	cmd.Flags().String("quote", "", "the wording you saw")
	AddProjectFlag(cmd)
	return cmd
}

func newContextCorrectCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "correct <from> <to>",
		Short: "Record that wording was changed at a place",
		Long: `Record that somebody changed wording: what was there, what replaced
it, and where.

A correction is evidence about how this project writes, and the cheapest there
is, because the judgement has already been made. On its own it records the
change and nothing else.

With --suggest it also records the rule the change implies, so the next use of
the old wording is reported. That rule is a suggestion: checks report it and
none of them fails on it until a person keeps it.

A person's correction that reverses an established rule contests the rule: it
reports instead of failing until a person keeps it again or drops the
correction, so your own edit never fails your build.`,
		Example: "  kapi context correct \"sign in\" \"log in\" --seen-in web/src/auth.tsx\n" +
			"  kapi context correct utilise use --seen-in docs/guide.md --suggest --advisory",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			seenIn, _ := cmd.Flags().GetStringSlice("seen-in")
			quote, _ := cmd.Flags().GetString("quote")
			suggest, _ := cmd.Flags().GetBool("suggest")
			advisory, _ := cmd.Flags().GetBool("advisory")
			note, _ := cmd.Flags().GetString("note")
			res, err := a.RecordContextCorrection(cmd.Context(), host.ContextCorrectRequest{
				Project:  projectPath,
				From:     args[0],
				To:       args[1],
				Evidence: evidenceFrom(seenIn, quote),
				Suggest:  suggest,
				Advisory: advisory,
				Note:     note,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().StringSlice("seen-in", nil, "a file the change was made in (repeatable)")
	cmd.Flags().String("quote", "", "the sentence the change was made in")
	cmd.Flags().Bool("suggest", false, "also record the rule the change implies, as a suggestion")
	cmd.Flags().Bool("advisory", false, "make that rule report without failing a check once kept")
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

func newContextLogCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show how this project's context came to be",
		Long: `List what has been observed, corrected, imported and decided about
this project's context, newest first.

Each line carries an operation's id, when it happened, who did it, what it was
about and where it stands: suggested and waiting for a person, established and
in force, contested by another rule it names, or withdrawn, dropped or
reverted.

The id is what the other verbs take: the short form a line shows, or any start
of it that names one operation. Narrow the list with --status to see what is
waiting for a decision, or with --session to see what one agent run did.
"--session this" is that run reading back its own work.`,
		Example: "  kapi context log\n" +
			"  kapi context log --status suggested\n" +
			"  kapi context log --status contested\n" +
			"  kapi context log --session 0ab4e399 --json\n" +
			"  kapi context log --session this\n" +
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
	cmd.Flags().String("session", "", "only what one agent session recorded, or \"this\" for the session this run records under")
	cmd.Flags().String("status", "", "only operations at one status: suggested, established, contested, withdrawn, dropped or reverted")
	cmd.Flags().String("actor", "", "only one actor, by name or by kind")
	cmd.Flags().String("since", "", "only what happened after a date (2006-01-02) or instant")
	cmd.Flags().Bool("subjects", false, "only what was recorded, leaving out the decisions about it")
	cmd.Flags().Int("limit", 0, "keep the most recent N (0 keeps them all)")
	AddProjectFlag(cmd)
	return cmd
}

func newContextKeepCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keep [id...]",
		Short: "Establish suggestions as rules",
		Long: `Keep suggestions, which establishes each rule and writes it where the rest
of kapi reads it: the project's terms or its content memory. An established rule
fails a check unless it is marked advisory.

Name one or more operations by id, or keep everything one agent session
suggested with --session. A contested suggestion disagrees with another rule,
and waits until you choose: a session keep leaves it and says so, and naming it
is refused with the other side named. Drop the side you do not want, then keep
the other.

Change the rule as you keep it with --use and --advisory, and widen it past the
point its evidence was seen at with --widen-to.`,
		Example: "  kapi context keep 0n794e2gk7\n" +
			"  kapi context keep 0n794e2gk7 0n79gkq853\n" +
			"  kapi context keep --session s0ab4e399\n" +
			"  kapi context keep 0n794e2gk7 --use \"content memory\"\n" +
			"  kapi context keep 0n794e2gk7 --advisory=false\n" +
			"  kapi context keep 0n794e2gk7 --widen-to workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			session, _ := cmd.Flags().GetString("session")
			use, _ := cmd.Flags().GetString("use")
			widenTo, _ := cmd.Flags().GetString("widen-to")
			note, _ := cmd.Flags().GetString("note")
			res, err := a.KeepContextOperations(cmd.Context(), host.ContextKeepRequest{
				Project:     projectPath,
				IDs:         args,
				Session:     session,
				Replacement: use,
				Advisory:    confirmAdvisory(cmd),
				WidenTo:     widenTo,
				Note:        note,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("session", "", "keep everything one agent session suggested that nothing disagrees with")
	cmd.Flags().String("use", "", "change what the rule says to write instead (one id)")
	cmd.Flags().Bool("advisory", false, "make the rule report without failing a check (--advisory=false makes it fail)")
	cmd.Flags().String("widen-to", "", "widen as you keep: \"workspace\", or the name of an axis the rule should stop being specific about")
	cmd.Flags().String("note", "", "why")
	AddProjectFlag(cmd)
	return cmd
}

func newContextDropCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drop <id>",
		Short: "Set a suggestion aside",
		Long: `Drop a suggestion. It stops being reported at once, and the record
of it stays in the log so the same suggestion can be recognised next time.

Dropping one side of a disagreement settles it: the other side is no longer
contested. An established rule is reverted rather than dropped.`,
		Example: "  kapi context drop 0n794e2gk7\n" +
			"  kapi context drop 0n794e2gk7 --note \"we say it both ways on purpose\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			note, _ := cmd.Flags().GetString("note")
			res, err := a.DropContextOperation(cmd.Context(), host.ContextDropRequest{
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

func newContextWithdrawCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "withdraw <id>",
		Short: "Take back a suggestion you recorded",
		Long: `Withdraw a suggestion you recorded, such as a correction entered
backwards. Only its author can withdraw a suggestion, and an agent only in the
session that recorded it. It stops being reported at once, and the log keeps
the record of it.`,
		Example: "  kapi context withdraw 0n794e2gk7\n" +
			"  kapi context withdraw 0n794e2gk7 --note \"recorded the wrong way round\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			note, _ := cmd.Flags().GetString("note")
			res, err := a.WithdrawContextOperation(cmd.Context(), host.ContextWithdrawRequest{
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
	cmd.Flags().String("note", "", "what was wrong with it")
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
		Example: "  kapi context revert 0n794e2gk7\n" +
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
		Short: "Put an established rule in force somewhere broader",
		Long: `Widen an established rule past the point its evidence was seen at.

A rule learned in one place holds in that place. Widening says it holds more
widely: "--to workspace" puts it in force in every project you work on here, and
naming an axis drops that axis from the rule's point so it stops being specific
about it.

A project that has its own decision about the word keeps it. The more specific
answer always wins.`,
		Example: "  kapi context widen 0n794e2gk7 --to workspace\n" +
			"  kapi context widen 0n794e2gk7 --to mode",
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

// confirmAdvisory is the --advisory edit a confirmation asks for: nil when the
// flag was not given, so the rule keeps what it says.
func confirmAdvisory(cmd *cobra.Command) *bool {
	if !cmd.Flags().Changed("advisory") {
		return nil
	}
	v, _ := cmd.Flags().GetBool("advisory")
	return &v
}

func newContextSettleCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settle",
		Short: "Establish the suggestions a person's signal backs",
		Long: `Establish every suggestion that a person's signal backs and nothing
open contradicts.

A person's signal is keeping the suggestion, a correction toward it, or the
change reaching the default branch. Another session recording the same rule and
the project's content writing the preferred form add to a suggestion's standing
without establishing it. A correction away from it, its withdrawal, or content
moving to a rejected form leave it contested for a person to decide.

With --merged, settle first reads the change a range of commits made and
records it as evidence for every suggestion whose preferred wording the change
added or whose rejected wording it removed. Run it in CI after a merge or a push
to the default branch. The same range records the same evidence, so running it
twice changes nothing.`,
		Example: "  kapi context settle\n" +
			"  kapi context settle --merged HEAD~1..HEAD\n" +
			"  kapi context settle --merged \"$BEFORE..$AFTER\" --pr 412 --merger asgeir",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			merged, _ := cmd.Flags().GetString("merged")
			pr, _ := cmd.Flags().GetInt("pr")
			merger, _ := cmd.Flags().GetString("merger")
			if (pr != 0 || merger != "") && merged == "" {
				return errors.New("--pr and --merger describe a merge: name it with --merged")
			}
			res, err := a.SettleContext(cmd.Context(), host.ContextSettleRequest{
				Project: projectPath,
				Merged:  merged,
				PR:      pr,
				Merger:  merger,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("merged", "", "a range of commits that reached the default branch, recorded as evidence")
	cmd.Flags().Int("pr", 0, "the pull request that merged the range (read from the commit subject when unset)")
	cmd.Flags().String("merger", "", "who merged it (the last commit's committer when unset)")
	AddProjectFlag(cmd)
	return cmd
}
