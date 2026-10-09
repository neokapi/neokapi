package output

import (
	"fmt"
	"io"
)

// Execution-trust standings, as `kapi trust list` reports them. Each names
// how the decision recorded for a path stands against what is at that path
// now.
const (
	// TrustStatusCurrent: the recipe still runs what was approved, so the next
	// run honours the decision silently.
	TrustStatusCurrent = "current"
	// TrustStatusChanged: the recipe no longer runs what was approved, so the
	// next run asks again.
	TrustStatusChanged = "changed"
	// TrustStatusMissing: nothing is at the path any more.
	TrustStatusMissing = "missing"
	// TrustStatusUnreadable: the recipe at the path does not load.
	TrustStatusUnreadable = "unreadable"
	// TrustStatusUnverified: the decision is about a formatter a comment edit
	// runs, whose digest covers the formatter's executable and configuration.
	// Only `kapi apply` recomputes it, at the edit.
	TrustStatusUnverified = "unverified"
)

// Execution-trust standings, as `kapi trust show` reports them for one recipe.
const (
	// TrustShowAllowed: an allow is recorded for what the recipe runs now.
	TrustShowAllowed = "allowed"
	// TrustShowDeclined: a deny is recorded for what the recipe runs now.
	TrustShowDeclined = "declined"
	// TrustShowUndecided: nothing is recorded for the recipe.
	TrustShowUndecided = "undecided"
	// TrustShowChanged: a decision is recorded, for something else than the
	// recipe runs now.
	TrustShowChanged = "changed"
	// TrustShowNothingToDecide: the recipe runs no commands.
	TrustShowNothingToDecide = "nothing_to_decide"
)

// TrustSite is one place a recipe arms code execution.
type TrustSite struct {
	// Where locates the site in the recipe's own vocabulary.
	Where string `json:"where"`
	// Kind is "tool", "format" or "formatter".
	Kind string `json:"kind"`
	// Name is the tool ID or format name.
	Name string `json:"name"`
	// Detail summarises what would run.
	Detail string `json:"detail,omitempty"`
}

// TrustListEntry is one recorded execution-trust decision.
type TrustListEntry struct {
	// Path is the record's key: the absolute recipe path, or the configuration
	// file that selected a formatter.
	Path string `json:"path"`
	// Kind is "recipe" or "formatter".
	Kind string `json:"kind"`
	// Decision is "allow" or "deny".
	Decision string `json:"decision"`
	// RecordedAt is when the answer was given, in RFC 3339.
	RecordedAt string `json:"recorded_at"`
	// Digest fingerprints what was approved or declined.
	Digest string `json:"digest"`
	// Status is one of the TrustStatus* values.
	Status string `json:"status"`
	// Detail says more about the status where there is more to say.
	Detail string `json:"detail,omitempty"`
}

// TrustListOutput is the result of `kapi trust list`.
type TrustListOutput struct {
	// Record is the file the decisions are kept in.
	Record  string           `json:"record"`
	Entries []TrustListEntry `json:"entries"`
}

func (o TrustListOutput) FormatText(w io.Writer) error {
	if len(o.Entries) == 0 {
		fmt.Fprintln(w, "No execution-trust decisions recorded.")
		fmt.Fprintf(w, "Record: %s\n", o.Record)
		return nil
	}
	t := NewTable(w).Accent(0).Headers("PATH", "DECISION", "RECORDED", "STATUS")
	s := t.Styles()
	for _, e := range o.Entries {
		decision := s.Success.Render(e.Decision)
		if e.Decision != "allow" {
			decision = s.Error.Render(e.Decision)
		}
		status := e.Status
		switch e.Status {
		case TrustStatusCurrent:
			status = s.Success.Render(status)
		case TrustStatusUnverified:
			status = s.Dim(status)
		default:
			status = s.Warn.Render(status)
		}
		t.Row(e.Path, decision, s.Dim(e.RecordedAt), status)
	}
	t.Render()
	for _, e := range o.Entries {
		if e.Detail != "" {
			fmt.Fprintf(w, "  %s: %s\n", e.Path, e.Detail)
		}
	}
	fmt.Fprintf(w, "Record: %s\n", o.Record)
	return nil
}

// TrustRecorded is the decision the record holds for a path, whether or not
// it still applies.
type TrustRecorded struct {
	Decision   string `json:"decision"`
	RecordedAt string `json:"recorded_at"`
	Digest     string `json:"digest"`
}

// TrustShowOutput is the result of `kapi trust show`.
type TrustShowOutput struct {
	// Path is the absolute recipe path.
	Path string `json:"path"`
	// Sites is what the recipe would run.
	Sites []TrustSite `json:"sites"`
	// Digest fingerprints Sites; empty when there are none.
	Digest string `json:"digest,omitempty"`
	// Status is one of the TrustShow* values.
	Status string `json:"status"`
	// Recorded is the record's entry for the path, if any.
	Recorded *TrustRecorded `json:"recorded,omitempty"`
	// EnvGranted is set when KAPI_TRUST_EXEC grants trust to this process,
	// whatever the record says.
	EnvGranted bool `json:"env_granted,omitempty"`
	// Record is the file the decision is kept in.
	Record string `json:"record"`
}

func (o TrustShowOutput) FormatText(w io.Writer) error {
	if len(o.Sites) == 0 {
		fmt.Fprintf(w, "%s runs no commands; there is nothing to decide.\n", o.Path)
	} else {
		fmt.Fprintf(w, "%s runs commands chosen by the recipe:\n\n", o.Path)
		writeTrustSites(w, o.Sites)
		fmt.Fprintln(w)
	}
	s := Theme(w)
	switch o.Status {
	case TrustShowAllowed:
		fmt.Fprintf(w, "Decision: %s (recorded %s)\n", s.Success.Render("allowed"), o.Recorded.RecordedAt)
	case TrustShowDeclined:
		fmt.Fprintf(w, "Decision: %s (recorded %s); kapi trust revoke %s answers again\n", s.Error.Render("declined"), o.Recorded.RecordedAt, o.Path)
	case TrustShowChanged:
		fmt.Fprintf(w, "Decision: %s; the recipe changed what it runs since the %s recorded %s, so the next run asks again\n",
			s.Warn.Render("changed"), o.Recorded.Decision, o.Recorded.RecordedAt)
	case TrustShowUndecided:
		fmt.Fprintf(w, "Decision: %s; the next run asks, or kapi trust allow -p %s approves it now\n", s.Warn.Render("undecided"), o.Path)
	}
	if o.EnvGranted {
		fmt.Fprintln(w, "KAPI_TRUST_EXEC grants trust to this process; the record is not consulted.")
	}
	fmt.Fprintf(w, "Record: %s\n", o.Record)
	return nil
}

// TrustRevokeOutput is the result of `kapi trust revoke`.
type TrustRevokeOutput struct {
	Path string `json:"path"`
	// Removed is false when nothing was recorded for the path.
	Removed bool `json:"removed"`
	// Decision is the decision that was withdrawn.
	Decision string `json:"decision,omitempty"`
	Record   string `json:"record"`
}

func (o TrustRevokeOutput) FormatText(w io.Writer) error {
	if !o.Removed {
		fmt.Fprintf(w, "No decision recorded for %s.\n", o.Path)
		return nil
	}
	fmt.Fprintf(w, "Withdrew the %s recorded for %s; the next run asks again.\n", o.Decision, o.Path)
	return nil
}

// TrustAllowOutput is the result of `kapi trust allow`.
type TrustAllowOutput struct {
	Path  string      `json:"path"`
	Sites []TrustSite `json:"sites"`
	// Digest fingerprints Sites.
	Digest string `json:"digest,omitempty"`
	// Recorded is set when an allow was written.
	Recorded bool `json:"recorded"`
	// Reason says why nothing was recorded.
	Reason string `json:"reason,omitempty"`
	Record string `json:"record"`
}

func (o TrustAllowOutput) FormatText(w io.Writer) error {
	if !o.Recorded {
		fmt.Fprintf(w, "Nothing recorded for %s: %s\n", o.Path, o.Reason)
		return nil
	}
	fmt.Fprintf(w, "Recorded: %s may run the commands shown. kapi trust revoke %s withdraws it.\n", o.Path, o.Path)
	return nil
}

// writeTrustSites renders the surface as one indented line per site.
func writeTrustSites(w io.Writer, sites []TrustSite) {
	for _, s := range sites {
		fmt.Fprintf(w, "  %s  %s", s.Where, s.Name)
		if s.Detail != "" {
			fmt.Fprintf(w, "  %s", s.Detail)
		}
		fmt.Fprintln(w)
	}
}
