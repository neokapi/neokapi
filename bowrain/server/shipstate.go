package server

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// applyShipStates enriches dashboard stats in place with failing-check counts
// and the derived per-locale ship state (store.DeriveShipState), on both the
// project-wide locale stats and each collection rollup.
//
// The QA pass is bounded the same way the convergence derive path bounds it
// (deriveFunc): checks run only for locales that reach full coverage in at
// least one scope (project-wide or a collection). Below full coverage a locale
// is pending regardless of check results, so the expensive full-block read and
// check run are skipped entirely. When checks do run, only blocks that carry a
// target for the locale are evaluated — a fully covered scope contains no
// target-less blocks, and under-covered scopes stay pending on coverage alone.
func applyShipStates(ctx context.Context, cs store.ContentStore, projectID, stream string, stats *store.TranslationDashboardStats) error {
	fullyCovered := func(ls store.LocaleTranslationStats) bool {
		return ls.TotalBlocks > 0 && ls.TranslatedBlocks >= ls.TotalBlocks
	}
	candidates := map[string]bool{}
	for _, ls := range stats.LocaleStats {
		if fullyCovered(ls) {
			candidates[ls.Locale] = true
		}
	}
	for _, coll := range stats.CollectionStats {
		for _, ls := range coll.Locales {
			if fullyCovered(ls) {
				candidates[ls.Locale] = true
			}
		}
	}

	// Error-severity failing block counts among the locale's translated blocks,
	// project-wide and attributed per collection.
	failing := map[string]int{}
	failingByColl := map[string]map[string]int{}

	if len(candidates) > 0 {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: stream})
		if err != nil {
			return fmt.Errorf("load blocks for ship-state checks: %w", err)
		}
		collByItem := make(map[string]string, len(stats.ItemStats))
		for _, it := range stats.ItemStats {
			collByItem[it.ItemName] = it.CollectionID
		}
		for localeStr := range candidates {
			loc := model.LocaleID(localeStr)
			for _, sb := range blocks {
				if sb.Block == nil || !sb.Block.Translatable || sb.Block.Target(loc) == nil {
					continue
				}
				blockFails := false
				for _, issue := range runQAOnBlock(ctx, sb.Block, loc) {
					if issue.Severity == "error" {
						blockFails = true
						break
					}
				}
				if !blockFails {
					continue
				}
				failing[localeStr]++
				cid := collByItem[sb.ItemName]
				if failingByColl[cid] == nil {
					failingByColl[cid] = map[string]int{}
				}
				failingByColl[cid][localeStr]++
			}
		}
	}

	for i := range stats.LocaleStats {
		ls := &stats.LocaleStats[i]
		ls.FailingChecks = failing[ls.Locale]
		ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks)
	}
	for i := range stats.CollectionStats {
		coll := &stats.CollectionStats[i]
		for j := range coll.Locales {
			ls := &coll.Locales[j]
			ls.FailingChecks = failingByColl[coll.CollectionID][ls.Locale]
			ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks)
		}
	}
	return nil
}
