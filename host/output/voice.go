package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/profile"
)

// VoiceGuideOutput is the result of `kapi voice guide`.
type VoiceGuideOutput struct {
	Profile string `json:"profile"`
	Guide   string `json:"guide"`
}

// FormatText prints the rendered voice guide verbatim.
func (o VoiceGuideOutput) FormatText(w io.Writer) error {
	_, err := io.WriteString(w, o.Guide)
	return err
}

// VoiceCheckOutput is the result of `kapi voice check` — a voice compliance
// score plus the findings that produced it.
type VoiceCheckOutput struct {
	Profile    string                   `json:"profile"`
	Score      int                      `json:"score"`
	Passed     bool                     `json:"passed"`
	MinScore   *int                     `json:"min_score,omitempty"`
	AIChecked  bool                     `json:"ai_checked"`
	Dimensions []profile.DimensionScore `json:"dimensions"`
	Findings   []profile.VoiceFinding   `json:"findings"`
}

// FormatText renders a compact human-readable scorecard.
func (o VoiceCheckOutput) FormatText(w io.Writer) error {
	s := Theme(w)
	status := s.Success.Render("PASS")
	if !o.Passed {
		status = s.Error.Render("FAIL")
	}
	fmt.Fprintf(w, "Voice profile score: %d/100  [%s]", o.Score, status)
	if o.Profile != "" {
		fmt.Fprintf(w, "  %s", s.Muted.Render("(profile: "+o.Profile+")"))
	}
	fmt.Fprintln(w)

	t := NewTable(w).Accent(0).Headers("DIMENSION", "SCORE", "ISSUES")
	for _, d := range o.Dimensions {
		if d.Issues == 0 {
			continue
		}
		t.Rowf(string(d.Dimension), d.Score, t.Styles().Warn.Render(strconv.Itoa(d.Issues)))
	}
	t.Render()

	if len(o.Findings) == 0 {
		fmt.Fprintln(w, "  No findings. Reads on brand.")
		return nil
	}
	fmt.Fprintf(w, "\n%d finding(s):\n", len(o.Findings))
	for _, f := range o.Findings {
		fmt.Fprintf(w, "  [%s/%s] %s", string(f.Severity), f.Category, f.Message)
		if f.Suggestion != "" {
			fmt.Fprintf(w, " (%s)", f.Suggestion)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// VoiceChange records a single term substitution made by `kapi voice rewrite`.
type VoiceChange struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// VoiceRewriteOutput is the result of `kapi voice rewrite`: the text with the
// profile's substitutions applied, the substitutions, and the vocabulary rules
// that matched and were left in place. Skipped is what tells a caller apart
// "nothing to fix" from "violations found and not fixed": both leave the text
// unchanged, and only the second needs a hand edit.
type VoiceRewriteOutput struct {
	Profile   string                `json:"profile"`
	Original  string                `json:"original"`
	Rewritten string                `json:"rewritten"`
	Changes   []VoiceChange         `json:"changes"`
	Skipped   []profile.RewriteSkip `json:"skipped"`
}

// FormatText prints the rewritten text, a change summary, and the rules that
// matched and could not be substituted.
func (o VoiceRewriteOutput) FormatText(w io.Writer) error {
	fmt.Fprintln(w, o.Rewritten)
	if len(o.Changes) > 0 {
		fmt.Fprintf(w, "\n%d change(s):\n", len(o.Changes))
		for _, c := range o.Changes {
			fmt.Fprintf(w, "  %q → %q", c.From, c.To)
			if c.Count > 1 {
				fmt.Fprintf(w, " (×%d)", c.Count)
			}
			fmt.Fprintln(w)
		}
	}
	if len(o.Skipped) > 0 {
		total := 0
		terms := make([]string, 0, len(o.Skipped))
		for _, s := range o.Skipped {
			total += s.Count
			terms = append(terms, s.Term)
		}
		fmt.Fprintf(w, "\n%d violation(s) found, not rewritten: %s\n", total, strings.Join(terms, ", "))
		for _, s := range o.Skipped {
			fmt.Fprintf(w, "  %q [%s/%s]: %s", s.Term, s.Severity, s.List, rewriteSkipReason(s))
			if s.Count > 1 {
				fmt.Fprintf(w, " (×%d)", s.Count)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "Rewrite these by hand and verify with 'kapi voice check'.")
	}
	return nil
}

// rewriteSkipReason words one skipped rule's reason for a reader.
func rewriteSkipReason(s profile.RewriteSkip) string {
	switch s.Reason {
	case profile.RewriteSkipInflectedForm:
		quoted := make([]string, 0, len(s.Matched))
		for _, m := range s.Matched {
			quoted = append(quoted, strconv.Quote(m))
		}
		return fmt.Sprintf("matched as %s; the replacement %q is worded for %q",
			strings.Join(quoted, ", "), s.Replacement, s.Term)
	default:
		if s.Note != "" {
			return "no replacement in the profile (" + s.Note + ")"
		}
		return "no replacement in the profile"
	}
}

// VoiceProfileSummary is one row in `kapi voice profiles`.
type VoiceProfileSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // "store", "pack"
}

// VoiceProfilesOutput is the result of `kapi voice profiles`.
type VoiceProfilesOutput struct {
	Profiles []VoiceProfileSummary `json:"profiles"`
	Total    int                   `json:"total"`
}

// FormatText prints a profile table.
func (o VoiceProfilesOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No voice profiles. Install a starter pack with: kapi voice pack <name>")
		return nil
	}
	t := NewTable(w).Accent(0).Headers("ID", "SOURCE", "NAME")
	s := t.Styles()
	for _, p := range o.Profiles {
		t.Row(p.ID, s.Dim(p.Source), strings.TrimSpace(p.Name))
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, "%d profile(s)", o.Total)
	return nil
}

// VoiceValidateOutput is the result of `kapi voice validate` — whether a brand
// voice profile YAML is structurally sound, plus the problems that made it
// invalid. Errors is always present (an empty list when valid) so the JSON
// shape is stable for CI: {valid, errors:[]}.
type VoiceValidateOutput struct {
	Valid   bool                     `json:"valid"`
	Source  string                   `json:"source,omitempty"`
	Profile string                   `json:"profile,omitempty"`
	Errors  []profile.ProfileProblem `json:"errors"`
}

// FormatText renders the validation verdict and any problems.
func (o VoiceValidateOutput) FormatText(w io.Writer) error {
	src := o.Source
	if src == "" {
		src = "profile"
	}
	var blocking, advisory []profile.ProfileProblem
	for _, p := range o.Errors {
		if p.Warning {
			advisory = append(advisory, p)
			continue
		}
		blocking = append(blocking, p)
	}

	switch {
	case len(blocking) > 0:
		fmt.Fprintf(w, "INVALID  %s: %d problem(s):\n", src, len(blocking))
		for _, p := range blocking {
			writeProblem(w, p)
		}
	case o.Profile != "":
		fmt.Fprintf(w, "VALID  %s (%s)\n", src, o.Profile)
	default:
		fmt.Fprintf(w, "VALID  %s\n", src)
	}

	// Advisories after the verdict, never instead of it. A profile with a tone
	// this does not recognise is usable, and saying so is the whole point of
	// the distinction.
	if len(advisory) > 0 {
		fmt.Fprintf(w, "\n%d note(s):\n", len(advisory))
		for _, p := range advisory {
			writeProblem(w, p)
		}
	}
	return nil
}

func writeProblem(w io.Writer, p profile.ProfileProblem) {
	if p.Field != "" {
		fmt.Fprintf(w, "  %s: %s\n", p.Field, p.Message)
		return
	}
	fmt.Fprintf(w, "  %s\n", p.Message)
}

// VoiceImportOutput is the result of `kapi voice import` / `kapi voice pack`.
type VoiceImportOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"` // "created" | "updated"
	Path   string `json:"path,omitempty"`
}

// FormatText confirms the import.
func (o VoiceImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "%s voice profile %q (id: %s)\n", o.Action, o.Name, o.ID)
	return nil
}

// VoicePointerOutput is the result of `kapi voice pointer`: what happened to
// the section in the project's assistant file.
type VoicePointerOutput struct {
	// File is the assistant file the section lives in; empty when nothing
	// was written.
	File string `json:"file,omitempty"`
	// Created is true when File was created by this run.
	Created bool `json:"created,omitempty"`
	// Action is created, updated, unchanged, removed or none.
	Action string `json:"action"`
	// Voice is the name the section carries.
	Voice string `json:"voice,omitempty"`
	// Warning says why the voice could not be named.
	Warning string `json:"warning,omitempty"`
}

// FormatText prints one line saying what was done, plus the import hint when
// a fresh AGENTS.md was created and a warning when the voice has no name.
func (o VoicePointerOutput) FormatText(w io.Writer) error {
	var line string
	switch o.Action {
	case "created":
		line = "Wrote the voice pointer to " + o.File
	case "updated":
		line = "Wrote the voice pointer into " + o.File
	case "unchanged":
		line = fmt.Sprintf("The voice pointer in %s is current", o.File)
	case "removed":
		line = fmt.Sprintf("Removed the voice pointer from %s: the project binds no voice", o.File)
	default:
		line = "No voice is bound; nothing written"
	}
	if o.Voice != "" && o.Action != "removed" {
		line += " (voice: " + o.Voice + ")"
	}
	fmt.Fprintln(w, line)
	if o.Created && strings.HasSuffix(o.File, AssistantFileHint) {
		fmt.Fprintln(w, "An assistant that reads CLAUDE.md picks it up through an import line there: @AGENTS.md")
	}
	if o.Warning != "" {
		fmt.Fprintf(w, "warning: could not name the voice: %s\n", o.Warning)
	}
	return nil
}

// AssistantFileHint is the file whose creation earns the import hint.
const AssistantFileHint = "AGENTS.md"
