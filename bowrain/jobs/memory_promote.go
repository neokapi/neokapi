package jobs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
)

// The corpus follows decisions (backlog 017, decision 2026-08-03): the content
// memory has ONE door in — wording a human (or their agent) approved. Transport
// and machine output stopped writing to it; this file is the door. An approval
// promotes its (source, target) pair, a rejection evicts it, and both are
// idempotent — the entry ID is content-derived, so re-promoting the same
// wording upserts the same row and re-evicting a gone row is a no-op.
//
// Shared by the two places a decision enters a server: the sync-push ingest
// (worker) and the editor's review endpoint (server). One implementation, so
// the two entry points cannot disagree about what an approval admits.

// PromoteDecisionsToMemory applies the decisions' verdicts to the workspace
// content memory: approved/signed-off pairs are promoted, rejected pairs are
// evicted. A decision is applied only when its unit resolves to a stored block
// whose current translation the decision still blesses (TargetHash) — a stale
// decision moves no wording, in either direction. Returns how many entries
// were promoted and evicted.
func PromoteDecisionsToMemory(
	ctx context.Context,
	cs store.ContentStore,
	tm memory.Store,
	projectID, stream string,
	sourceLocale model.LocaleID,
	decisions []venue.UnitDecision,
) (promoted, evicted int) {
	if tm == nil || cs == nil || sourceLocale.IsEmpty() || len(decisions) == 0 {
		return 0, 0
	}

	// One block load per item, not per decision.
	byItem := map[string][]venue.UnitDecision{}
	for _, d := range decisions {
		if d.ItemName == "" || d.Unit == "" || d.Variant == "" {
			continue
		}
		switch d.ReviewState {
		case "approved", "signed-off", "rejected":
		default:
			continue // parking, assignment, un-review: not a corpus verdict
		}
		byItem[d.ItemName] = append(byItem[d.ItemName], d)
	}

	now := time.Now()
	for itemName, ds := range byItem {
		rows, err := cs.GetBlocks(ctx, store.BlockQuery{
			ProjectID: projectID, Stream: stream, ItemName: itemName,
		})
		if err != nil {
			slog.WarnContext(ctx, "memory promotion: item load failed",
				"project", projectID, "item", itemName, "error", err)
			continue
		}
		bySource := map[string]*venue.StoredBlock{}
		for _, sb := range rows {
			if sb.SourceID != "" {
				bySource[sb.SourceID] = sb
			}
		}

		for _, d := range ds {
			sb := bySource[d.Unit]
			if sb == nil || sb.Block == nil || !sb.Block.Translatable {
				continue
			}
			var variant model.VariantKey
			if err := variant.UnmarshalText([]byte(d.Variant)); err != nil || variant.Locale == "" {
				continue
			}
			targetRuns := localeTargetRuns(sb.Block, variant.Locale)
			if len(targetRuns) == 0 {
				continue
			}
			targetText := model.RunsText(targetRuns)
			if strings.TrimSpace(targetText) == "" {
				continue
			}
			// The decision blesses (or condemns) a SPECIFIC translation. If the
			// stored target has moved on, the verdict is about text that is no
			// longer there — promoting would admit unapproved wording, evicting
			// would remove wording nobody rejected.
			if d.TargetHash != "" && state.TargetHash(targetText) != d.TargetHash {
				continue
			}
			sourceRuns := sb.Block.Source
			if len(sourceRuns) == 0 || model.RunsText(sourceRuns) == "" {
				continue
			}

			entryID := memoryEntryID(sourceLocale, variant.Locale, sourceRuns, targetRuns)
			if d.ReviewState == "rejected" {
				if err := tm.Delete(ctx, entryID); err == nil {
					evicted++
				}
				continue
			}
			entry := memory.Entry{
				ID: entryID,
				Variants: map[model.LocaleID][]model.Run{
					sourceLocale:   cloneRunsForMemory(sourceRuns),
					variant.Locale: cloneRunsForMemory(targetRuns),
				},
				HintSrcLang: sourceLocale,
				ProjectID:   projectID,
				Origins: []memory.Origin{{
					Source:    "approval",
					Key:       d.Unit,
					Reference: d.DecidedAt,
					AddedAt:   now,
					AddedBy:   d.DecidedBy,
					// The context the approval was made under, so the corpus
					// carries for a promoted answer what the local absorber
					// carries for an absorbed one.
					ContextFingerprint: d.GoverningFingerprint,
				}},
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tm.Add(ctx, entry); err != nil {
				slog.WarnContext(ctx, "memory promotion failed",
					"project", projectID, "unit", d.Unit, "error", err)
				continue
			}
			promoted++
		}
	}
	return promoted, evicted
}
