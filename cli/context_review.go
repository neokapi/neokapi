package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// `kapi context review`: the decisions a person makes about what was
// suggested. In a terminal it walks the suggestions one at a time; with
// --keep, --drop or --choose it records those decisions and asks nothing; and
// anywhere else it lists what is waiting. Every decision goes through
// host.DecideContextReview, so the prompt and the flags record the same
// operations.

func newContextReviewCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Decide about the suggestions waiting for you",
		Long: `Walk through what has been suggested about how this project writes, and
decide about each one: keep it as a rule, drop it, choose a side where two
suggestions disagree, or apply a rule more widely than where it was seen.

A suggestion nobody answers keeps advising, so nothing here has to be cleared.
A rule you keep is in force from then on: checks fail on it unless it is
advisory. Dropping a rule already in force takes it back out.

In a terminal, review asks about each suggestion in turn. Elsewhere it lists
what is waiting, with the id each decision takes:

  --keep <id>        keep a suggestion (repeatable); --use changes what it says
  --drop <id>        drop a suggestion or a rule (repeatable)
  --choose <id>      choose one side where two suggestions disagree
  --widen-to <where> with --keep: apply it more widely: project, workspace,
                     or a part of where it was seen, such as mode

--session narrows the review to what one assistant session suggested, and
"--session <id> --keep all" keeps everything it suggested that nothing
disagrees with.

Reviewing moves your "since you last looked" marker, kept on this machine and
never in the project. --peek leaves it where it is.`,
		Example: "  kapi context review\n" +
			"  kapi context review --session s0ab4e399\n" +
			"  kapi context review --keep 0n794e2gk7 --keep 0n79gkq853\n" +
			"  kapi context review --keep 0n794e2gk7 --use \"content memory\"\n" +
			"  kapi context review --keep 0n794e2gk7 --widen-to workspace\n" +
			"  kapi context review --drop 0n794e2gk7 --note \"we say it both ways on purpose\"\n" +
			"  kapi context review --choose 0n794e2gk7\n" +
			"  kapi context review --session s0ab4e399 --keep all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			keep, _ := cmd.Flags().GetStringArray("keep")
			drop, _ := cmd.Flags().GetStringArray("drop")
			choose, _ := cmd.Flags().GetString("choose")
			widenTo, _ := cmd.Flags().GetString("widen-to")
			use, _ := cmd.Flags().GetString("use")
			note, _ := cmd.Flags().GetString("note")
			session, _ := cmd.Flags().GetString("session")
			decide := host.ContextReviewRequest{
				Project:     projectPath,
				Keep:        keep,
				Drop:        drop,
				Choose:      choose,
				Session:     session,
				WidenTo:     widenTo,
				Replacement: use,
				Advisory:    confirmAdvisory(cmd),
				Note:        note,
			}
			if !decide.Empty() {
				res, err := a.DecideContextReview(cmd.Context(), decide)
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			if widenTo != "" || use != "" || cmd.Flags().Changed("advisory") {
				return errors.New("--widen-to, --use and --advisory change what --keep names: name it with --keep")
			}

			peek, _ := cmd.Flags().GetBool("peek")
			since, _ := cmd.Flags().GetString("since")
			req := host.ContextDigestRequest{Project: projectPath}
			if since != "" {
				at, perr := parseSince(since)
				if perr != nil {
					return perr
				}
				req.Since = at
			}
			digest, err := a.ContextDigest(cmd.Context(), req)
			if err != nil {
				return err
			}
			digest = digest.OnlySession(session)
			if reviewInteractive(cmd) {
				if err := walkContextReview(cmd, a, projectPath, digest, note); err != nil {
					return err
				}
			} else if err := output.Print(cmd, digest); err != nil {
				return err
			}
			if peek || since != "" || session != "" {
				return nil
			}
			return a.NoteContextDigestRead(digest)
		},
	}
	cmd.Flags().StringArray("keep", nil, "keep a suggestion by id (repeatable); \"all\" with --session keeps what the session suggested")
	cmd.Flags().StringArray("drop", nil, "drop a suggestion or a rule by id (repeatable)")
	cmd.Flags().String("choose", "", "choose this side where two suggestions disagree: the other side is dropped and this one kept")
	cmd.Flags().String("widen-to", "", "with --keep: apply it more widely: \"project\", \"workspace\", or a part of where it was seen")
	cmd.Flags().String("use", "", "with one --keep: change what the rule says to write instead")
	cmd.Flags().Bool("advisory", false, "with --keep: make the rule report without failing a check (--advisory=false makes it fail)")
	cmd.Flags().String("session", "", "only what one assistant session suggested")
	cmd.Flags().String("note", "", "why")
	cmd.Flags().Bool("peek", false, "leave the \"since you last looked\" marker where it is")
	cmd.Flags().String("since", "", "treat what came after a date (2006-01-02) or instant as new, in place of the marker")
	AddProjectFlag(cmd)
	return cmd
}

// reviewInteractive reports whether review can ask: a person at a terminal on
// both ends, and text output. A test sets it.
var reviewInteractive = func(cmd *cobra.Command) bool {
	if output.ResolveFormat(cmd) != output.FormatText {
		return false
	}
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok || !isatty.IsTerminal(in.Fd()) {
		return false
	}
	out, ok := cmd.OutOrStdout().(*os.File)
	return ok && isatty.IsTerminal(out.Fd())
}

// reviewStep is one thing review asks about: a disagreement (several sides), a
// suggestion, or a rule that came into force since the person last looked.
type reviewStep struct {
	sides    []host.DigestItem
	evidence bool // a rule contested by evidence alone
	reason   string
	item     host.DigestItem
	rule     bool // a rule in force
}

// reviewSteps orders what review asks about: disagreements first, then every
// suggestion still waiting, then the rules new since the last look.
func reviewSteps(d host.ContextDigest) []reviewStep {
	var steps []reviewStep
	seen := map[string]bool{}
	for _, c := range d.Conflicts {
		steps = append(steps, reviewStep{sides: c.Sides, evidence: c.ByEvidence, reason: c.Reason})
		for _, s := range c.Sides {
			seen[s.ID] = true
		}
	}
	for _, t := range d.Suggested {
		for _, g := range t.Groups {
			for _, it := range g.Items {
				if !seen[it.ID] && (it.Keepable || it.Droppable) {
					steps = append(steps, reviewStep{item: it})
					seen[it.ID] = true
				}
			}
		}
	}
	for _, it := range d.Established {
		if it.New && !seen[it.ID] {
			steps = append(steps, reviewStep{item: it, rule: true})
			seen[it.ID] = true
		}
	}
	return steps
}

// walkContextReview asks about each step in turn and records each decision as
// it is made, so quitting part way keeps what was decided.
func walkContextReview(cmd *cobra.Command, a *App, projectPath string, d host.ContextDigest, note string) error {
	w := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	steps := reviewSteps(d)
	if len(steps) == 0 {
		fmt.Fprintln(w, "Nothing is waiting for you. Assistants' notes appear here as suggestions.")
		return nil
	}
	var kept, dropped, widened, skipped int
	decide := func(req host.ContextReviewRequest) bool {
		req.Project, req.Note = projectPath, note
		res, err := a.DecideContextReview(cmd.Context(), req)
		if err != nil {
			fmt.Fprintf(w, "  not recorded: %v\n", err)
			return false
		}
		kept += len(res.Kept)
		dropped += len(res.Dropped)
		widened += len(res.Widened)
		if res.Chosen != nil {
			kept++
			dropped += len(res.Chosen.SetAside)
		}
		return true
	}

walk:
	for i, step := range steps {
		fmt.Fprintf(w, "\n[%d/%d] ", i+1, len(steps))
		switch {
		case len(step.sides) > 1:
			fmt.Fprintln(w, step.reason)
			for n, side := range step.sides {
				fmt.Fprintf(w, "  %d) %s  %s\n", n+1, side.Short, side.Sentence)
				writeReviewFacts(w, side, "     ")
			}
			answer := ask(w, in, fmt.Sprintf("Choose a side (1-%d), d drop them all, s skip, q quit", len(step.sides)))
			switch {
			case answer == "q" || answer == "":
				break walk
			case answer == "s":
				skipped++
			case answer == "d":
				ids := make([]string, len(step.sides))
				for n, side := range step.sides {
					ids[n] = side.ID
				}
				decide(host.ContextReviewRequest{Drop: ids})
			default:
				n, err := strconv.Atoi(answer)
				if err != nil || n < 1 || n > len(step.sides) {
					fmt.Fprintln(w, "  skipped: that is not one of the sides")
					skipped++
					continue
				}
				decide(host.ContextReviewRequest{Choose: step.sides[n-1].ID})
			}
		default:
			it := step.item
			if len(step.sides) == 1 {
				it = step.sides[0]
				fmt.Fprintln(w, step.reason)
			}
			fmt.Fprintf(w, "%s  %s\n", it.Short, it.Sentence)
			writeReviewFacts(w, it, "  ")
			prompt := "k keep, w keep and apply more widely, d drop, s skip, q quit"
			if step.rule {
				prompt = "w apply more widely, d drop, s skip, q quit"
			}
			switch answer := ask(w, in, prompt); answer {
			case "q", "":
				break walk
			case "s":
				skipped++
			case "k":
				if step.rule {
					skipped++
					continue
				}
				decide(host.ContextReviewRequest{Keep: []string{it.ID}})
			case "d":
				decide(host.ContextReviewRequest{Drop: []string{it.ID}})
			case "w":
				where := ask(w, in, "Apply where? project, workspace, or a part of where it was seen")
				if where == "" || where == "q" {
					skipped++
					continue
				}
				decide(host.ContextReviewRequest{Keep: []string{it.ID}, WidenTo: where})
			default:
				fmt.Fprintln(w, "  skipped: answer with one of the letters shown")
				skipped++
			}
		}
	}
	fmt.Fprintf(w, "\nKept %d, dropped %d, applied more widely %d, skipped %d.\n", kept, dropped, widened, skipped)
	return nil
}

// ask prints a prompt and reads one answer, lower-cased. End of input reads as
// an empty answer, which quits.
func ask(w io.Writer, in *bufio.Reader, prompt string) string {
	fmt.Fprintf(w, "%s: ", prompt)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(w)
		return ""
	}
	return strings.ToLower(strings.TrimSpace(line))
}

// writeReviewFacts writes where an item was seen and who noticed it.
func writeReviewFacts(w io.Writer, it host.DigestItem, indent string) {
	if it.Quote != nil && (it.Quote.Quote != "" || it.Quote.Path != "") {
		line := it.Quote.Path
		if it.Quote.Quote != "" {
			line = strconv.Quote(it.Quote.Quote)
			if it.Quote.Path != "" {
				line += " in " + it.Quote.Path
			}
		}
		fmt.Fprintf(w, "%sseen %s\n", indent, line)
	}
	if it.Usage != nil {
		fmt.Fprintf(w, "%s%s\n", indent, it.Usage.Line)
	}
	if it.Standing != "" {
		fmt.Fprintf(w, "%s%s\n", indent, it.Standing)
	}
}
