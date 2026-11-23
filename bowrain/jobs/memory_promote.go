package jobs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
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

// PromotionMemory is the content memory as decision promotion writes it. The
// promotion is one writer among several (a person's edit, an import, a merge),
// so it changes nothing it did not put there: it creates an entry only when
// the memory holds none for the pair, adds its origin to an entry that holds
// the pair, and on a rejection removes its own origin, which removes the entry
// only when no other origin is left.
type PromotionMemory interface {
	GetEntry(ctx context.Context, id string) (memory.Entry, bool, error)
	AddIfAbsent(ctx context.Context, entry memory.Entry) (bool, error)
	AddOrigin(ctx context.Context, id string, origin memory.Origin, properties map[string]string) (bool, error)
	RemoveOrigin(ctx context.Context, id, source, key string) (bool, error)
}

// promotionOrigin is the origin source an approval records on the entry that
// holds the pair it approved; the origin's key is the decision's unit.
const promotionOrigin = "approval"

// PromoteDecisionsToMemory applies the decisions' verdicts to the workspace
// content memory: approved pairs are promoted, rejected pairs are
// evicted. A decision is applied only when its unit resolves to a stored block
// whose current translation the decision still blesses (Revision) — a stale
// decision moves no wording, in either direction. Returns how many entries
// were promoted and evicted.
//
// An approval of a pair the memory already holds adds its origin and leaves
// the text alone. An entry under the pair's id whose text a person has since
// changed is that person's, and the approval leaves it as it is. A rejection
// removes the approval's origin; an entry another origin still holds, a
// person's edit among them, stays.
func PromoteDecisionsToMemory(
	ctx context.Context,
	cs store.ContentStore,
	tm PromotionMemory,
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
		case "approved", "rejected":
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
			var variant model.EditionKey
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
			if d.Revision != store.VariantRevision(d.Variant, targetRuns) {
				continue
			}
			// A memory pair runs from the edition every translation is made
			// from to the translation.
			src, _ := sb.Block.Edition(sb.Block.Authoritative(model.AuthorityPolicy{}))
			sourceRuns := src.Runs
			if len(sourceRuns) == 0 || model.RunsText(sourceRuns) == "" {
				continue
			}

			entryID := memoryEntryID(sourceLocale, variant.Locale, sourceRuns, targetRuns)
			if d.ReviewState == "rejected" {
				removed, err := tm.RemoveOrigin(ctx, entryID, promotionOrigin, d.Unit)
				if err != nil {
					slog.WarnContext(ctx, "memory eviction failed",
						"project", projectID, "unit", d.Unit, "error", err)
					continue
				}
				if removed {
					evicted++
				}
				continue
			}
			entry := memory.Entry{
				ID: entryID,
				Variants: map[model.LocaleID][]model.Run{
					sourceLocale:   model.CloneRuns(sourceRuns),
					variant.Locale: model.CloneRuns(targetRuns),
				},
				HintSrcLang: sourceLocale,
				ProjectID:   projectID,
				Origins: []memory.Origin{{
					Source:    promotionOrigin,
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
			ok, err := promote(ctx, tm, entry)
			if err != nil {
				slog.WarnContext(ctx, "memory promotion failed",
					"project", projectID, "unit", d.Unit, "error", err)
				continue
			}
			if ok {
				promoted++
			}
		}
	}
	return promoted, evicted
}

// promote stores entry, or adds its origin to the entry the memory already
// holds under its id when that entry still holds the same pair. It reports
// whether the memory now records the approval.
func promote(ctx context.Context, tm PromotionMemory, entry memory.Entry) (bool, error) {
	created, err := tm.AddIfAbsent(ctx, entry)
	if err != nil || created {
		return created, err
	}
	existing, ok, err := tm.GetEntry(ctx, entry.ID)
	if err != nil || !ok {
		return false, err
	}
	for loc, runs := range entry.Variants {
		if existing.VariantText(loc) != model.RunsText(runs) {
			// A person changed the text after the pair was promoted; the
			// entry is theirs now.
			return false, nil
		}
	}
	return tm.AddOrigin(ctx, entry.ID, entry.Origins[0], nil)
}
