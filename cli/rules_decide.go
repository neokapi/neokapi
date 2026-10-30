package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
)

// walkRules asks about each suggestion in turn and records each decision as it
// is made, so quitting part way keeps what was decided. It leaves the "since
// you last looked" marker where it is: the marker belongs to the review of
// what changed, and rules asks only about suggestions, which wait until they
// are decided.
func walkRules(cmd *cobra.Command, a *App, projectPath string, view *host.RulesDecideView, note string) error {
	w := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	var tally rulesTally
	for i, d := range view.Decisions {
		fmt.Fprintf(w, "\n[%d/%d] ", i+1, len(view.Decisions))
		if err := askRule(w, in, a, cmd, projectPath, view, d, note, &tally); err != nil {
			if err == errRulesQuit {
				break
			}
			return err
		}
	}
	fmt.Fprintf(w, "\n%s\n", tally.line())
	return nil
}

// askRule asks one question until it is answered. Picking a number changes
// the option a keep takes and says again what keeping it does.
func askRule(w io.Writer, in *bufio.Reader, a *App, cmd *cobra.Command, projectPath string,
	view *host.RulesDecideView, d host.RuleDecision, note string, tally *rulesTally,
) error {
	chosen, other := d.Chosen, ""
	if err := d.Write(w, chosen); err != nil {
		return err
	}
	options := len(d.Options)
	if d.Kind == host.RuleDecisionName {
		options++ // Other…
	}
	prompt := "k keep, n not a rule, l later, q quit"
	if options > 0 {
		prompt = fmt.Sprintf("k keep, n not a rule, l later, 1-%d pick another, q quit", options)
	}
	record := func(req host.ContextReviewRequest) bool {
		req.Project, req.Note = projectPath, note
		res, err := a.DecideContextReview(cmd.Context(), req)
		if err != nil {
			fmt.Fprintf(w, "  not recorded: %v\n", err)
			return false
		}
		tally.kept += len(res.Kept)
		tally.dropped += len(res.Dropped)
		if res.Chosen != nil {
			tally.kept++
			tally.dropped += len(res.Chosen.SetAside)
		}
		return true
	}
	for {
		answer := ask(w, in, prompt)
		switch answer {
		case "q", "":
			return errRulesQuit
		case "l":
			tally.later++
			return nil
		case "k":
			record(d.KeepRequest(chosen, other))
			return nil
		case "n":
			record(d.DropRequest())
			return nil
		}
		n, err := strconv.Atoi(answer)
		switch {
		case err != nil || n < 1 || n > options:
			fmt.Fprintln(w, "  answer with one of the letters or numbers shown")
		case n <= len(d.Options):
			chosen, other = n-1, ""
			fmt.Fprintln(w, "  "+d.Options[chosen].Consequence)
		default:
			form := askForm(w, in)
			if form == "" {
				continue
			}
			chosen, other = -1, form
			fmt.Fprintln(w, "  "+view.Consequence(d, form))
		}
	}
}

// askForm reads the form a person types for Other…, keeping its case.
func askForm(w io.Writer, in *bufio.Reader) string {
	fmt.Fprint(w, "Form to write: ")
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(w)
		return ""
	}
	return strings.TrimSpace(line)
}
