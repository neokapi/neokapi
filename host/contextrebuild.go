package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// ContextRebuild reports what a rebuild of a project's stores replayed.
type ContextRebuild struct {
	// Operations counts the operations replayed, by kind.
	Operations map[string]int `json:"operations"`
	// Failed lists the operations the stores refused, as they refused them
	// when they were first written.
	Failed []string `json:"failed,omitempty"`
	// Seconds is how long the rebuild took.
	Seconds float64 `json:"seconds"`
	// From is the last operation of the checkpoint the rebuild started from,
	// empty when it replayed the whole log.
	From string `json:"from,omitempty"`
	// Checkpoint is the last operation of the checkpoint written afterwards,
	// when one was asked for.
	Checkpoint string `json:"checkpoint,omitempty"`
}

// FormatText renders the rebuild for a reader.
func (r ContextRebuild) FormatText(w io.Writer) error {
	total := 0
	kinds := make([]string, 0, len(r.Operations))
	for kind, n := range r.Operations {
		total += n
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	from := ""
	if r.From != "" {
		from = " after checkpoint " + workspace.ShortOpID(r.From)
	}
	if _, err := fmt.Fprintf(w, "Rebuilt the project's stores from %s%s in %.1fs.\n",
		pluralUnit(total, "operation", "operations"), from, r.Seconds); err != nil {
		return err
	}
	for _, kind := range kinds {
		if _, err := fmt.Fprintf(w, "  %-13s %d\n", kind, r.Operations[kind]); err != nil {
			return err
		}
	}
	for _, f := range r.Failed {
		if _, err := fmt.Fprintf(w, "  refused: %s\n", f); err != nil {
			return err
		}
	}
	if r.Checkpoint != "" {
		if _, err := fmt.Fprintf(w, "Wrote a checkpoint through %s.\n", workspace.ShortOpID(r.Checkpoint)); err != nil {
			return err
		}
	}
	return nil
}

// RebuildProjectContext empties the project's terms store, content memory and
// voice profiles, and the rules it widened to the workspace, and replays the
// workspace's log into them (core/projector). The stores it leaves are the
// ones the log's writes produced, which is what makes them disposable: a store
// that was damaged, or that a merged log moved past, is rebuilt rather than
// repaired.
//
// With checkpoint set it writes a checkpoint afterwards (core/projector), so the
// next rebuild starts from the stores as they stand now and replays only what
// comes after.
func (a *App) RebuildProjectContext(ctx context.Context, projectPath string, checkpoint bool) (ContextRebuild, error) {
	var res ContextRebuild
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	w, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return res, err
	}
	if !w.Logged() {
		return res, errors.New("this project's store has no workspace log to rebuild from")
	}
	start := time.Now()
	report, err := w.Rebuild(ctx)
	res.Operations, res.Failed, res.From = report.Operations, report.Failed, report.Checkpoint
	res.Seconds = time.Since(start).Seconds()
	if err != nil || !checkpoint {
		return res, err
	}
	cp, err := w.Checkpoint(ctx)
	res.Checkpoint = cp.Through
	return res, err
}
