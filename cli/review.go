package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// ReviewQueueOutput is the structured result of `kapi status --review`: every
// unit awaiting human review, the derived counterpart of the convergence loop's
// "parked" outcome.
type ReviewQueueOutput struct {
	Project string       `json:"project,omitempty"`
	Pending []ReviewItem `json:"pending"`
}

// FormatText renders the review queue.
func (o ReviewQueueOutput) FormatText(w io.Writer) error {
	if len(o.Pending) == 0 {
		fmt.Fprintln(w, "Review queue empty: every translated unit is reviewed (or nothing is translated yet).")
		return nil
	}
	fmt.Fprintf(w, "%d unit(s) awaiting review:\n\n", len(o.Pending))
	for _, it := range o.Pending {
		fmt.Fprintf(w, "  %-8s %s:%s\n", it.Locale, it.File, it.Key)
		fmt.Fprintf(w, "           %s\n", it.Source)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Approve a unit with `kapi apply` (a `review` change-set, addressed by its file/id/locale) — the decision lands in the project state store and the unit then counts as reviewed.")
	return nil
}

// computeReviewQueue lists the translated units that are not yet approved — the
// review queue. It is derived (recomputed from content + the project state store),
// never tracked.
func (a *App) computeReviewQueue(ctx context.Context, proj *project.KapiProject, root string, units []verifyUnit) ([]ReviewItem, error) {
	reviewed, err := a.loadReviewedCorrections(proj, root)
	if err != nil {
		return nil, err
	}
	var items []ReviewItem
	for _, u := range units {
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // can't read the target (e.g. a compiled .mo) — not reviewable per unit
			}
			return nil, berr
		}
		if missing {
			continue // nothing translated for this locale yet
		}
		loc := model.LocaleID(u.locale)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// Only a translated unit can await review; an absent target is
			// upstream of review, and an already-reviewed pair is done.
			if unitState(b, u.locale) != string(model.TargetStatusTranslated) {
				continue
			}
			if reviewed.reviewed(b, u.locale) {
				continue
			}
			items = append(items, ReviewItem{
				Locale: u.locale,
				File:   u.displayPath,
				Key:    blockKey(b),
				Source: preview(b.SourceText()),
				Target: preview(b.TargetText(loc)),
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Locale != items[j].Locale {
			return items[i].Locale < items[j].Locale
		}
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		return items[i].Key < items[j].Key
	})
	return items, nil
}
