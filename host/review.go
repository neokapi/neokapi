package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// reviewQueueOutput is the structured result of `kapi status --review`: every
// unit awaiting human review, the derived counterpart of the convergence loop's
// "parked" outcome.
type reviewQueueOutput struct {
	Project string       `json:"project,omitempty"`
	Pending []ReviewItem `json:"pending"`
}

// FormatText renders the review queue.
func (o reviewQueueOutput) FormatText(w io.Writer) error {
	if len(o.Pending) == 0 {
		fmt.Fprintln(w, "Review queue empty: every translated unit is reviewed (or nothing is translated yet).")
		return nil
	}
	fmt.Fprintf(w, "%d unit(s) awaiting review:\n\n", len(o.Pending))
	t := output.NewTable(w).Accent(1).Headers("locale", "unit", "source", "ai")
	s := t.Styles()
	for _, it := range o.Pending {
		ai := ""
		if it.AIScore != nil {
			ai = fmt.Sprintf("ai %d", *it.AIScore)
			if it.AIModel != "" {
				ai += " (" + it.AIModel + ")"
			}
		}
		t.Row(it.Locale, it.File+":"+it.Key, it.Source, s.Dim(ai))
	}
	t.Render()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Approve a unit with `kapi apply` (a `review` change-set, addressed by its file/id/locale) — the state record lands in the project store and the unit then counts as reviewed.")
	return nil
}

// computeReviewQueue lists the translated units that are not yet approved — the
// review queue. It is derived (recomputed from content + the project state store),
// never tracked.
func (a *App) computeReviewQueue(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit) ([]ReviewItem, error) {
	reviewed, err := a.loadReviewedCorrections(ctx, proj, root)
	if err != nil {
		return nil, err
	}
	docs := a.documentIndexOrEmpty(ctx, root)
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
		loc := model.LocaleID(u.Locale)
		scope := docs.Scope(root, u.SourcePath)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// Only a translated unit can await review; an absent target is
			// upstream of review, and a decided pair is out of the queue —
			// approved is done, rejected is back in the work queue (draft)
			// until its translation changes.
			if unitState(b, u.Locale) != string(model.TargetStatusTranslated) {
				continue
			}
			if reviewed.decided(scope, b, u.Locale) {
				continue
			}
			item := ReviewItem{
				Locale:       u.Locale,
				File:         u.DisplayPath,
				Key:          blockKey(b),
				Collection:   u.Collection,
				SourceLocale: string(proj.Defaults.SourceLanguage),
				Source:       preview(b.SourceText()),
				Target:       preview(b.TargetText(loc)),
			}
			// Surface a fresh AI pre-review annotation (score + model) so the
			// queue can show it — read from the state store, never a provider
			// call.
			if rev, ok := reviewed.aiReviewFor(scope, b, u.Locale); ok {
				score := rev.score
				item.AIScore = &score
				item.AIModel = rev.model
			}
			items = append(items, item)
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
