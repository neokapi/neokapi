package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// reviewQueueOutput is the structured result of `kapi status --review`: every
// unit awaiting human review, the derived counterpart of the convergence loop's
// "parked" outcome. One queue holds every language, the source language among
// them, and Languages says how many units each language has waiting.
type reviewQueueOutput struct {
	Project   string            `json:"project,omitempty"`
	Pending   []ReviewQueueItem `json:"pending"`
	Languages []ReviewLanguage  `json:"languages,omitempty"`
}

// FormatText renders the review queue.
func (o reviewQueueOutput) FormatText(w io.Writer) error {
	if len(o.Pending) == 0 {
		fmt.Fprintln(w, "Review queue empty: no unit in any language is waiting for a person.")
		return nil
	}
	fmt.Fprintf(w, "%d unit(s) awaiting review:\n\n", len(o.Pending))
	t := output.NewTable(w).Accent(1).Headers("locale", "unit", "source", "ai")
	s := t.Styles()
	sources, held := 0, 0
	for _, it := range o.Pending {
		ai := ""
		if it.AIScore != nil {
			ai = fmt.Sprintf("ai %d", *it.AIScore)
			if it.AIModel != "" {
				ai += " (" + it.AIModel + ")"
			}
		}
		lang := it.LanguageTag()
		if it.IsSource {
			sources++
			// The tag stays a tag; the marker rides beside it, so a reader sees
			// which rows are the author's own wording.
			lang += " · source"
			if it.Held {
				held++
			}
		}
		t.Row(lang, it.File+":"+it.Key, it.Source, s.Dim(ai))
	}
	t.Render()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Approve a translated unit with `kapi apply` (a `review` change-set, addressed by its file/id/locale). The state record lands in the project store and the unit then counts as reviewed.")
	if sources > 0 {
		fmt.Fprintln(w, "Units marked `source` are the project's own source language. `kapi apply` records target-language decisions only; approve source wording in the Review page of Kapi Desktop.")
	}
	if held > 0 {
		fmt.Fprintf(w, "%d source unit(s) rank below the project's source gate, so the loop holds their translations.\n", held)
	}
	return nil
}

// relativeToRoot renders a source path relative to the project root, so a path
// filter written against the content ("web/**") can be matched against it. It
// falls back to the path as given: a path outside the root is still an honest
// answer, and an empty one would silently pass every filter.
func relativeToRoot(root, path string) string {
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.ToSlash(rel)
}

// computeReviewQueue lists the translated units that are not yet approved — the
// review queue. It is derived (recomputed from content + the project state store),
// never tracked.
func (a *App) computeReviewQueue(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit) ([]ReviewQueueItem, error) {
	reviewed, err := a.loadReviewedCorrections(ctx, proj, root)
	if err != nil {
		return nil, err
	}
	docs := a.documentIndexOrEmpty(ctx, root)
	var items []ReviewQueueItem
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
			item := ReviewQueueItem{
				Locale:       u.Locale,
				Language:     u.Locale,
				Status:       string(model.TargetStatusTranslated),
				File:         u.DisplayPath,
				Relative:     relativeToRoot(root, u.SourcePath),
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

// ReviewQueueOptions narrows a queue listing.
type ReviewQueueOptions struct {
	// Languages limits the listing to these language tags, the source language
	// among them. Empty lists every language.
	Languages []string
}

// wants reports whether a language belongs in the listing.
func (o ReviewQueueOptions) wants(lang string) bool {
	if len(o.Languages) == 0 {
		return true
	}
	for _, want := range o.Languages {
		if strings.EqualFold(strings.TrimSpace(want), lang) {
			return true
		}
	}
	return false
}

// ReviewQueue lists every unit awaiting a person, in one queue across the
// project's languages: the translated units not yet approved, and the source
// units the project's source gate or its `approved` rung is waiting on. Source
// units carry IsSource and sort first.
//
// The listing is unified; the storage is not. A source decision is recorded
// under the source locale variant and a target decision under the target's, so
// what a row means for the store is what its language says
// (host/sourcereview.go).
func (a *App) ReviewQueue(ctx context.Context, projectPath, sourceLang string, opts ReviewQueueOptions) (ReviewQueue, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)

	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return ReviewQueue{}, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return ReviewQueue{}, fmt.Errorf("resolve content: %w", err)
	}
	srcUnits, err := a.SourceUnitsFromProject(proj, root)
	if err != nil {
		return ReviewQueue{}, fmt.Errorf("resolve source content: %w", err)
	}

	var queue ReviewQueue
	cacheErr := a.withParseCache(root, func() error {
		q, qerr := a.computeUnifiedReviewQueue(ctx, proj, root, units, srcUnits, opts)
		queue = q
		return qerr
	})
	if cacheErr != nil {
		return ReviewQueue{}, cacheErr
	}
	return queue, nil
}

// computeUnifiedReviewQueue merges the target queue and the source queue into
// one listing. units are the (source, locale) pairs the target queue measures;
// srcUnits are the project's source files, which a monolingual project has even
// when it resolves no pair.
func (a *App) computeUnifiedReviewQueue(ctx context.Context, proj *project.KapiProject, root string, units, srcUnits []VerifyUnit, opts ReviewQueueOptions) (ReviewQueue, error) {
	targets, err := a.computeReviewQueue(ctx, proj, root, units)
	if err != nil {
		return ReviewQueue{}, err
	}
	sources, err := a.computeSourceQueue(ctx, proj, root, srcUnits)
	if err != nil {
		return ReviewQueue{}, err
	}
	sourceLang := string(proj.Defaults.SourceLanguage)

	all := make([]ReviewQueueItem, 0, len(targets)+len(sources))
	for _, it := range sources {
		all = append(all, sourceItemAsQueueItem(it, sourceLang))
	}
	all = append(all, targets...)
	convergence.SortReviewQueue(all)

	// The summary counts the whole queue, so a surface filtered to one language
	// still offers the others.
	languages := convergence.SummarizeReviewLanguages(all)

	pending := all
	if len(opts.Languages) > 0 {
		pending = make([]ReviewQueueItem, 0, len(all))
		for _, it := range all {
			if opts.wants(it.LanguageTag()) {
				pending = append(pending, it)
			}
		}
	}
	return ReviewQueue{Pending: pending, Languages: languages}, nil
}

// sourceItemAsQueueItem renders one source-queue row as a queue item. The
// source file is both halves of the address: for source content the file a
// reviewer reads and the file a path filter is written against are the same
// file.
func sourceItemAsQueueItem(it SourceQueueItem, sourceLang string) ReviewQueueItem {
	lang := it.SourceLocale
	if lang == "" {
		lang = sourceLang
	}
	return ReviewQueueItem{
		Locale:       lang,
		Language:     lang,
		IsSource:     true,
		File:         it.File,
		Relative:     it.Relative,
		Key:          it.Key,
		Collection:   it.Collection,
		SourceLocale: lang,
		Source:       it.Source,
		Status:       it.Status,
		Held:         it.Held,
	}
}
