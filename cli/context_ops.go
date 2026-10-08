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

// The recording half of the context surface (AD C-11).
//
// A project's context grows out of ordinary work. Something is noticed, or a
// person changes wording, and it is recorded as a suggestion; a person decides
// about it in review. note and log are what an assistant reaches for mid-task,
// and they exist here as well as over MCP (context_note,
// context_session_summary) because the skill drives the command line. A habit
// an assistant can only keep on one of the two surfaces is a habit half the
// assistants do not have.
//
// A suggestion advises from the moment it is recorded. A check reports it and
// no check fails on it. A person's signal establishes it: keeping it in review,
// a change toward it, or the change reaching the default branch (`kapi context
// sync --merged`). Establishing writes the rule into the project's terms
// store, where every check reads it.

func newContextNoteCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note [what you noticed]",
		Short: "Record what you noticed, or what a person changed",
		Long: `Record one thing this project's files do every time: a product,
feature or plan name as they write it, the spelling variety they keep to, a
word they use where writers often use another, who the text addresses. Record
it whether or not your own text uses it. Leave alone a word the files write
more than one way (record nothing about it, not even a note), an interface
label, and wording taken from your task.

For a name or word, pass --term with the form the files use and --instead-of
with the form they avoid: the split form of a one-word name, the other
spelling, or the other word. kapi derives the other forms to avoid: the
spaced, hyphenated and closed spellings of a compound, each part
capitalised. "--term Quickcast --instead-of 'Quick cast'" avoids Quick cast,
Quick-cast and QuickCast. Without --term a note is a fact in prose.

When a person changes wording, record it with --from (what was there), --to
(what replaced it) and --seen-in. A change is evidence about how this project
writes. A person's change toward a rule somebody already suggested is that
person's backing for it, so the rule is established, and fails a check, unless
something recorded argues against it. With --suggest it also records the rule
the change implies, so the next use of the old wording is reported. A person's
change that reverses a rule in force contests the rule, so their own edit
never fails their build.

Every note is a suggestion. Checks report it, no check fails on it, and a
person decides about it with "kapi context review". Say where you saw it with
--seen-in and --quote. Take back a note of your own that was wrong with
--withdraw <id>, in the session that recorded it.`,
		Example: "  kapi context note \"the docs address the reader as you\"\n" +
			"  kapi context note --term Quickcast --instead-of \"Quick cast\" \\\n" +
			"    --seen-in README.md --quote \"Quickcast forecasts the next hour.\"\n" +
			"  kapi context note --from \"sign in\" --to \"log in\" --seen-in web/src/auth.tsx --suggest\n" +
			"  kapi context note --withdraw 0n794e2gk7 --why \"recorded the wrong way round\"",
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
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			withdraw, _ := cmd.Flags().GetString("withdraw")
			why, _ := cmd.Flags().GetString("why")
			seenIn, _ := cmd.Flags().GetStringSlice("seen-in")
			quote, _ := cmd.Flags().GetString("quote")
			suggest, _ := cmd.Flags().GetBool("suggest")
			advisory, _ := cmd.Flags().GetBool("advisory")

			switch {
			case withdraw != "":
				if text != "" || term != "" || from != "" || to != "" {
					return errors.New("--withdraw takes back an earlier note: record nothing else with it")
				}
				res, err := a.WithdrawContextOperation(cmd.Context(), host.ContextWithdrawRequest{
					Project: projectPath, ID: withdraw, Note: why,
				})
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			case from != "" || to != "":
				if from == "" || to == "" {
					return errors.New("a change needs both wordings: --from (what was there) and --to (what replaced it)")
				}
				if text != "" || term != "" {
					return errors.New("record a change with --from and --to alone; note what you noticed separately")
				}
				res, err := a.RecordContextCorrection(cmd.Context(), host.ContextCorrectRequest{
					Project:  projectPath,
					From:     from,
					To:       to,
					Evidence: evidenceFrom(seenIn, quote),
					Suggest:  suggest,
					Advisory: advisory,
					Note:     why,
				})
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			if text == "" && term == "" {
				return errors.New("say what you noticed, name the form the project uses with --term, or give --from and --to for a change")
			}
			if suggest || advisory {
				return errors.New("--suggest and --advisory go with a change (--from and --to); a note about a term is a suggestion already")
			}
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
	cmd.Flags().String("from", "", "for a change a person made: the wording that was there")
	cmd.Flags().String("to", "", "for a change a person made: the wording that replaced it")
	cmd.Flags().Bool("suggest", false, "with --from and --to: also record the rule the change implies")
	cmd.Flags().Bool("advisory", false, "with --suggest: make that rule report without failing a check once kept")
	cmd.Flags().StringSlice("seen-in", nil, "a file you saw it in, or the change was made in (repeatable)")
	cmd.Flags().String("quote", "", "the wording you saw")
	cmd.Flags().String("withdraw", "", "take back a note of your own by its id")
	cmd.Flags().String("why", "", "what the person said about the change, or what was wrong with the note withdrawn")
	AddProjectFlag(cmd)
	return cmd
}

func newContextLogCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List what was recorded, or what this session recorded",
		Long: `List what has been noted, imported and decided about this project's
context, newest first.

Each line carries an id, when it happened, who did it, what it was about and
where it stands: suggested and waiting for a person, established and in force,
contested by another rule it names, or withdrawn, dropped or reset.

"--session this" is an assistant reading back what its own run recorded, which
is how it ends a task report. Narrow the list with --status to see what is
waiting for a decision, or with --session to see what one run did.`,
		Example: "  kapi context log --session this\n" +
			"  kapi context log --status suggested\n" +
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
	cmd.Flags().String("session", "", "only what one agent session recorded, or \"this\" for the session this run records under")
	cmd.Flags().String("status", "", "only operations at one status: suggested, established, contested, withdrawn, dropped or reset")
	cmd.Flags().String("actor", "", "only one actor, by name or by kind")
	cmd.Flags().String("since", "", "only what happened after a date (2006-01-02) or instant")
	cmd.Flags().Bool("subjects", false, "only what was recorded, leaving out the decisions about it")
	cmd.Flags().Int("limit", 0, "keep the most recent N (0 keeps them all)")
	AddProjectFlag(cmd)
	return cmd
}

func newContextResetCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset --before <session | date | id>",
		Short: "Go back to how the context stood at an earlier point",
		Long: `Go back to how this project's context stood at an earlier point: before an
assistant's session started, before a date or a moment, or before one entry in
"kapi context log".

Everything recorded from that point on is set aside: it stops answering, and
the project's terms, approved wording and voice are rebuilt as they stood.
Nothing is erased. What was set aside stays in the log, and the reset is
recorded too, so a later reset to before it brings everything back.

To change your mind about one rule, decide again in "kapi context review"
instead. --dry-run says what a reset would set aside without doing it.`,
		Example: "  kapi context reset --before s0ab4e399\n" +
			"  kapi context reset --before 2026-10-01 --dry-run\n" +
			"  kapi context reset --before 0n794e2gk7 --note \"back to before the import\"",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			before, _ := cmd.Flags().GetString("before")
			note, _ := cmd.Flags().GetString("note")
			req := host.ContextResetRequest{Project: projectPath, Before: before, Note: note}
			if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
				res, err := a.ContextResetScope(cmd.Context(), req)
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			res, err := a.ResetContext(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("before", "", "the point to go back to: an agent session, a date (2006-01-02) or instant, or an id from kapi context log")
	_ = cmd.MarkFlagRequired("before")
	cmd.Flags().Bool("dry-run", false, "say what the reset would set aside without recording it")
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

// confirmAdvisory is the --advisory edit a decision asks for: nil when the flag
// was not given, so the rule keeps what it says.
func confirmAdvisory(cmd *cobra.Command) *bool {
	if !cmd.Flags().Changed("advisory") {
		return nil
	}
	v, _ := cmd.Flags().GetBool("advisory")
	return &v
}
