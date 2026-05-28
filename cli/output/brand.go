package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/brand"
)

// BrandGuideOutput is the result of `kapi brand guide`.
type BrandGuideOutput struct {
	Profile string `json:"profile"`
	Guide   string `json:"guide"`
}

// FormatText prints the rendered voice guide verbatim.
func (o BrandGuideOutput) FormatText(w io.Writer) error {
	_, err := io.WriteString(w, o.Guide)
	return err
}

// BrandCheckOutput is the result of `kapi brand check` — a brand compliance
// score plus the findings that produced it.
type BrandCheckOutput struct {
	Profile    string                    `json:"profile"`
	Score      int                       `json:"score"`
	Passed     bool                      `json:"passed"`
	MinScore   *int                      `json:"min_score,omitempty"`
	AIChecked  bool                      `json:"ai_checked"`
	Dimensions []brand.DimensionScore    `json:"dimensions"`
	Findings   []brand.BrandVoiceFinding `json:"findings"`
}

// FormatText renders a compact human-readable scorecard.
func (o BrandCheckOutput) FormatText(w io.Writer) error {
	status := "PASS"
	if !o.Passed {
		status = "FAIL"
	}
	fmt.Fprintf(w, "Brand voice score: %d/100  [%s]", o.Score, status)
	if o.Profile != "" {
		fmt.Fprintf(w, "  (profile: %s)", o.Profile)
	}
	fmt.Fprintln(w)
	for _, d := range o.Dimensions {
		if d.Issues == 0 {
			continue
		}
		fmt.Fprintf(w, "  %-16s %3d  (%d issue(s))\n", string(d.Dimension), d.Score, d.Issues)
	}
	if len(o.Findings) == 0 {
		fmt.Fprintln(w, "  No findings — on brand.")
		return nil
	}
	fmt.Fprintf(w, "\n%d finding(s):\n", len(o.Findings))
	for _, f := range o.Findings {
		fmt.Fprintf(w, "  [%s/%s] %s", string(f.Severity), f.Category, f.Message)
		if f.Suggestion != "" {
			fmt.Fprintf(w, " — %s", f.Suggestion)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// BrandChange records a single term substitution made by `kapi brand rewrite`.
type BrandChange struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// BrandRewriteOutput is the result of `kapi brand rewrite`.
type BrandRewriteOutput struct {
	Profile   string        `json:"profile"`
	AIRewrite bool          `json:"ai_rewrite"`
	Original  string        `json:"original"`
	Rewritten string        `json:"rewritten"`
	Changes   []BrandChange `json:"changes"`
}

// FormatText prints the rewritten text and a change summary.
func (o BrandRewriteOutput) FormatText(w io.Writer) error {
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
	return nil
}

// BrandProfileSummary is one row in `kapi brand profiles`.
type BrandProfileSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // "store", "pack"
}

// BrandProfilesOutput is the result of `kapi brand profiles`.
type BrandProfilesOutput struct {
	Profiles []BrandProfileSummary `json:"profiles"`
	Total    int                   `json:"total"`
}

// FormatText prints a profile table.
func (o BrandProfilesOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No brand voice profiles. Install a starter pack with: kapi brand pack <name>")
		return nil
	}
	for _, p := range o.Profiles {
		fmt.Fprintf(w, "%-22s %-8s %s\n", p.ID, p.Source, strings.TrimSpace(p.Name))
	}
	fmt.Fprintf(w, "\n%d profile(s)\n", o.Total)
	return nil
}

// BrandImportOutput is the result of `kapi brand import` / `kapi brand pack`.
type BrandImportOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"` // "created" | "updated"
	Path   string `json:"path,omitempty"`
}

// FormatText confirms the import.
func (o BrandImportOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "%s brand voice profile %q (id: %s)\n", o.Action, o.Name, o.ID)
	return nil
}
