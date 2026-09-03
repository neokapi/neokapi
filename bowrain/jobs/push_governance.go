package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/review"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// A push moves content. It does not decide.
//
// The payload carries both halves of a review verdict: a target at a rung
// above translated, and a decision record saying who approved it. Both are
// written by whoever ran `kapi push`, so storing them as sent would give
// anyone holding the permission to write files the permission to approve every
// unit in the project, from a laptop, in one command.
//
// So every rung above translated and every verdict a push carries is put to the
// same two questions the platform's own review surfaces ask: does this user
// hold review permission for that language in that project, and does the
// workspace separation-of-duties policy pass with them as the decider. The
// answer comes from bowrain/review, which the web routes ask as well.
//
// What fails the gate is demoted, not dropped: the content still lands, at the
// rung a translation with nobody's blessing sits on. A push is never failed for
// a rung, because the content is not in question and refusing to store it would
// lose work over a permission the pusher can be granted afterwards.

// pushGovernor holds one push's answer: the gate, the identities the answer is
// about, and the report of what it refused.
type pushGovernor struct {
	gate    *review.Gate
	permits *review.LanguagePermits
	actor   string

	// storedID maps a block as the payload names it (row id, or the durable
	// source id) to the row the venue holds, which is what authorship is keyed
	// on. A block the venue does not hold yet is absent: nobody has written
	// that translation here, so nobody can be conflicted about it.
	storedID map[string]string
	// unitID maps a decision's (item, unit) to the same row.
	unitID map[unitRef]string
	// priorStatus is the rung each stored target sits on now, for the audit
	// trail's before/after.
	priorStatus map[platstore.TargetRef]model.TargetStatus

	counts   map[refusalRef]int
	units    []venue.RefusedUnit
	accepted map[acceptedKey]acceptedRung
}

// recordPushGovernance stores what the review gate refused on the push's job
// row, where the status endpoint reads it back for the waiting `kapi push`.
//
// Best-effort: the content has landed and the transition has committed, so a
// failure here must not fail the push. It is logged rather than dropped,
// because a report that never arrives is a producer that goes on re-sending
// the same refused verdicts, and nothing anywhere saying why.
func recordPushGovernance(ctx context.Context, deps *WorkerDeps, jobID string, report venue.PushGovernance) {
	if report.Empty() || deps.JobStore == nil {
		return
	}
	payload, err := json.Marshal(report)
	if err != nil {
		slog.WarnContext(ctx, "could not render the push governance report", "job_id", jobID, "error", err)
		return
	}
	if err := deps.JobStore.SetPushGovernance(ctx, jobID, string(payload)); err != nil {
		slog.WarnContext(ctx, "could not record what the push governance refused; the producer will not be told",
			"job_id", jobID, "error", err)
	}
}

// judging reports whether this push carries anything the gate has to judge.
func (g *pushGovernor) judging() bool { return g != nil && g.gate != nil }

// err reports a permission question that could not be answered. It is checked
// inside the transition, so a push whose governance could not be resolved rolls
// back rather than landing with its verdicts silently demoted.
func (g *pushGovernor) err() error {
	if g == nil || g.permits == nil {
		return nil
	}
	return g.permits.Err()
}

// summary is the one line a push's log carries when the gate refused anything.
func (g *pushGovernor) summary() string {
	report := g.report()
	if report.Empty() {
		return ""
	}
	parts := make([]string, 0, len(report.Refusals))
	for _, r := range report.Refusals {
		parts = append(parts, fmt.Sprintf("%d %s(s) for %s (%s)", r.Count, r.Kind, r.Locale, r.Reason))
	}
	return "Not accepted by the platform's review governance: " + strings.Join(parts, ", ")
}

// unitRef names a decision's unit within the item that scopes its identity.
type unitRef struct{ item, unit string }

// refusalRef is one line of the report: a language, a kind of verdict and the
// reason it was refused.
type refusalRef struct{ locale, kind, reason string }

// acceptedRung is one rung change this push made that the audit trail records:
// which target moved, from where to where.
//
// The target is named twice over. blockID is the venue's row when the push
// found one; item and unit are the durable identity, which is all a block
// arriving for the first time has until the transition stores it. The audit
// entry names the row, so an unresolved one is looked up after the commit.
type acceptedRung struct {
	blockID  string
	item     string
	unit     string
	locale   string
	from, to model.TargetStatus
}

// acceptedKey is one target, however the push named it: a push states a
// promotion twice (on the block and in the decision record) and the audit trail
// records the change rather than the statements of it.
type acceptedKey struct{ ref, locale string }

// newPushGovernor resolves everything the gate needs before the transition
// opens: which rows the payload's blocks and units name, who last wrote each
// target by hand, the workspace policy, and the pusher's review permission for
// each language a verdict names.
//
// It reports an error when a question could not be ASKED: an unreadable
// authorship table, a permission lookup that failed, a deployment with no way
// to resolve permissions at all. That fails the push, which is the only safe
// reading: demoting on an unanswerable question would silently discard an
// approval the pusher was entitled to make, and the content is not lost by a
// failed push (the producer still holds it and sends it again).
func newPushGovernor(
	ctx context.Context,
	deps *WorkerDeps,
	projectID, stream, workspaceID, actor string,
	staged []stagedGroup,
	decisions []venue.UnitDecision,
) (*pushGovernor, error) {
	g := &pushGovernor{
		actor:       actor,
		storedID:    map[string]string{},
		unitID:      map[unitRef]string{},
		priorStatus: map[platstore.TargetRef]model.TargetStatus{},
		counts:      map[refusalRef]int{},
		accepted:    map[acceptedKey]acceptedRung{},
	}
	if !carriesVerdict(staged, decisions) {
		return g, nil // nothing to judge; no lookups, no gate
	}
	if deps.ReviewAuthority == nil {
		return nil, errors.New("this deployment cannot resolve review permissions, so a push carrying approvals is refused")
	}

	locales := verdictLocales(staged, decisions)
	g.loadPriorRows(ctx, deps, projectID, stream, staged, decisions)

	blockIDs := make([]string, 0, len(g.storedID))
	seen := map[string]bool{}
	for _, id := range g.storedID {
		if id != "" && !seen[id] {
			seen[id] = true
			blockIDs = append(blockIDs, id)
		}
	}
	for _, id := range g.unitID {
		if id != "" && !seen[id] {
			seen[id] = true
			blockIDs = append(blockIDs, id)
		}
	}
	sort.Strings(blockIDs)

	g.permits = review.NewLanguagePermits(ctx, deps.ReviewAuthority, review.Query{
		ProjectID:   projectID,
		WorkspaceID: workspaceID,
		UserID:      actor,
		Permission:  platauth.PermReview,
	})
	cfg := review.Config{
		Actor:       actor,
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Stream:      stream,
		BlockIDs:    blockIDs,
		Locales:     locales,
		Policy:      deps.ReviewAuthority,
		Permits:     g.permits.Allows,
		// One record for the push, filed with the count, rather than one per
		// unit on a bus that drops what it cannot keep up with.
		Silent: true,
	}
	if authors, ok := deps.ContentStore.(platstore.TargetAuthorStore); ok {
		cfg.Authors = authors
	}
	gate, err := review.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	g.gate = gate
	return g, nil
}

// loadPriorRows reads, once per item, the rows the venue already holds: the
// join between what the payload calls a block and what authorship is keyed on,
// and the rung each target sits on now.
//
// Best-effort per item. An item the venue has never held has no prior rows and
// no authorship, which is the correct answer for content arriving for the first
// time rather than a failure to look.
func (g *pushGovernor) loadPriorRows(
	ctx context.Context,
	deps *WorkerDeps,
	projectID, stream string,
	staged []stagedGroup,
	decisions []venue.UnitDecision,
) {
	items := map[string]bool{}
	var looseIDs []string
	for _, group := range staged {
		if group.ItemName != "" {
			items[group.ItemName] = true
			continue
		}
		for _, b := range group.Blocks {
			if b != nil && b.ID != "" {
				looseIDs = append(looseIDs, b.ID)
			}
		}
	}
	for _, d := range decisions {
		if d.ItemName != "" && d.CarriesVerdict() {
			items[d.ItemName] = true
		}
	}

	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, itemName := range names {
		rows, err := deps.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: projectID, Stream: stream, ItemName: itemName,
		})
		if err != nil {
			continue // an item the venue does not hold yet
		}
		g.indexRows(itemName, rows)
	}
	if len(looseIDs) > 0 {
		rows, err := deps.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: projectID, Stream: stream, IDs: looseIDs, Limit: len(looseIDs),
		})
		if err == nil {
			g.indexRows("", rows)
		}
	}
}

// indexRows records one item's rows under every name the payload may use for
// them, and the rung each of their targets holds.
func (g *pushGovernor) indexRows(itemName string, rows []*venue.StoredBlock) {
	for _, row := range rows {
		if row == nil || row.ID == "" {
			continue
		}
		g.storedID[row.ID] = row.ID
		if row.SourceID != "" {
			g.storedID[row.SourceID] = row.ID
			g.unitID[unitRef{item: itemName, unit: row.SourceID}] = row.ID
		}
		if row.Block == nil {
			continue
		}
		for key, target := range row.Block.Targets {
			if target == nil {
				continue
			}
			g.priorStatus[platstore.TargetRef{BlockID: row.ID, Locale: string(key.Locale)}] = target.Status
		}
	}
}

// rowFor resolves the row a payload block names, joined the way the rest of the
// push joins it: by row id, then by the durable structural name.
func (g *pushGovernor) rowFor(b *model.Block) string {
	if b == nil {
		return ""
	}
	if id, ok := g.storedID[b.ID]; ok {
		return id
	}
	if b.Name != "" {
		if id, ok := g.storedID[b.Name]; ok {
			return id
		}
	}
	return ""
}

// allow puts one (block, locale) pair to the gate. It counts the refusal for
// the report; countRefusal is false when the caller counts it itself.
func (g *pushGovernor) allow(blockID, locale, kind string, countRefusal bool) (bool, string) {
	err := g.gate.Allow(blockID, locale)
	if err == nil {
		return true, ""
	}
	refusal, ok := errors.AsType[review.Refusal](err)
	reason := venue.RefusedNoReviewPermission
	if ok {
		reason = refusal.Reason
	}
	if countRefusal {
		g.counts[refusalRef{locale: locale, kind: kind, reason: reason}]++
	}
	return false, reason
}

// refusalLocale is the language a refusal is reported against, falling back to
// the raw variant when it is one this venue cannot read.
func refusalLocale(locale, variant string) string {
	if locale == "" {
		return variant
	}
	return locale
}

// noteAccepted records a rung this push moved, for the audit trail. A rung the
// venue already holds moved nothing and is not recorded; a target stated twice
// is recorded once.
func (g *pushGovernor) noteAccepted(blockID, item, unit, locale string, from, to model.TargetStatus) {
	if from == to {
		return
	}
	ref := blockID
	if ref == "" {
		if unit == "" {
			return // nothing to name the target by
		}
		ref = item + "\x00" + unit
	}
	g.accepted[acceptedKey{ref: ref, locale: locale}] = acceptedRung{
		blockID: blockID, item: item, unit: unit, locale: locale, from: from, to: to,
	}
}

// noteUnit records which unit a refusal was about, up to the bound the report
// carries. The counts stay exact whether or not the unit made the list.
func (g *pushGovernor) noteUnit(itemName, unit, variant, reason string) {
	if unit == "" || len(g.units) >= venue.RefusedUnitLimit {
		return
	}
	g.units = append(g.units, venue.RefusedUnit{
		ItemName: itemName, Unit: unit, Variant: variant, Reason: reason,
	})
}

// vetTargets demotes every pushed target whose rung the gate refuses.
//
// The content lands either way. A refused target keeps its translation and
// sits at translated, the rung a translation nobody has blessed occupies, or
// stays at draft when there is nothing to translate.
func (g *pushGovernor) vetTargets(staged []stagedGroup) {
	if g.gate == nil {
		return
	}
	for _, group := range staged {
		for _, b := range group.Blocks {
			if b == nil {
				continue
			}
			blockID := g.rowFor(b)
			for key, target := range b.Targets {
				if target == nil || target.Status.Rank() <= model.TargetStatusTranslated.Rank() {
					continue
				}
				locale := string(key.Locale)
				prior := g.priorStatus[platstore.TargetRef{BlockID: blockID, Locale: locale}]
				if target.Status.Rank() <= prior.Rank() {
					// The venue already holds this rung, or a higher one. The
					// push claims nothing new, so there is nothing to judge,
					// and judging it would let a pusher without review
					// permission UNDO an approval by sending it back.
					continue
				}
				kind := venue.VerdictApproval
				if target.Status == model.TargetStatusSignedOff {
					kind = venue.VerdictSignOff
				}
				allowed, reason := g.allow(blockID, locale, kind, true)
				if allowed {
					g.noteAccepted(blockID, group.ItemName, b.Name, locale, prior, target.Status)
					continue
				}
				g.noteUnit(group.ItemName, b.Name, variantText(key), reason)
				target.Status = refusedRung(target, prior)
			}
		}
	}
}

// refusedRung is where a refused target lands: the rung a translation nobody
// has approved sits on (translated when it carries a translation, draft when
// it does not), or the rung the venue already holds, whichever is higher.
//
// Never lower than the venue's own. A refusal withholds what the push asked
// for; it does not undo what somebody with the right to decide already did.
func refusedRung(target *model.Target, prior model.TargetStatus) model.TargetStatus {
	landing := model.TargetStatusTranslated
	if model.RunsText(target.Runs) == "" {
		landing = model.TargetStatusDraft
	}
	if prior.Rank() > landing.Rank() {
		return prior
	}
	return landing
}

// vetDecisions returns the decision records this push may write.
//
// A record whose verdict the gate refuses is kept as the basis it carries and
// nothing more: the venue records that a translation exists and which source it
// was written for, and records nobody as having approved it. A record the
// ledger already holds passes whatever the gate says: re-sending what the
// platform decided is not a decision, and refusing it would make every push
// after a legitimate approval report a refusal.
//
// Every accepted verdict is attributed to the authenticated pusher. A
// client-supplied decider is never trusted: it is a string in a file that
// anyone with a text editor can write.
func (g *pushGovernor) vetDecisions(held []venue.UnitDecision, decisions []venue.UnitDecision) []venue.UnitDecision {
	if g.gate == nil || len(decisions) == 0 {
		return decisions
	}
	ledger := make(map[unitVariantRef]venue.UnitDecision, len(held))
	for _, d := range held {
		ledger[unitVariantRef{item: d.ItemName, unit: d.Unit, variant: d.Variant}] = d
	}

	out := make([]venue.UnitDecision, 0, len(decisions))
	for _, d := range decisions {
		if !d.CarriesVerdict() {
			out = append(out, d) // a basis, not a verdict
			continue
		}
		ref := unitVariantRef{item: d.ItemName, unit: d.Unit, variant: d.Variant}
		prior, inLedger := ledger[ref]
		if inLedger && sameVerdict(prior, d) {
			// Already held. Keep the ledger's decider: the platform recorded
			// who decided, and a re-push is not a second decision.
			d.DecidedBy = prior.DecidedBy
			d.DecidedAt = prior.DecidedAt
			out = append(out, d)
			continue
		}

		locale := decisionLocale(d)
		kind := d.VerdictKind()
		refuse := func(reason string) {
			g.counts[refusalRef{locale: refusalLocale(locale, d.Variant), kind: kind, reason: reason}]++
			if inLedger && prior.CarriesVerdict() {
				// The venue holds a verdict of its own here. Writing the basis
				// would erase it, which is how a refusal turns into the very
				// override it exists to prevent. The venue's record stands, and
				// the unit is not offered for the producer to retire: the
				// producer's copy is the stale one, and a pull is what settles
				// it.
				return
			}
			g.noteUnit(d.ItemName, d.Unit, d.Variant, reason)
			out = append(out, d.AsBasis(model.TargetStatusTranslated))
		}
		if locale == "" {
			// A variant this venue cannot read is a language it cannot check a
			// permission for. Refuse the verdict rather than guess at it.
			refuse(venue.RefusedNoReviewPermission)
			continue
		}
		blockID := g.unitID[unitRef{item: d.ItemName, unit: d.Unit}]
		allowed, reason := g.allow(blockID, locale, kind, false)
		if !allowed {
			refuse(reason)
			continue
		}
		d.DecidedBy = g.actor
		priorRung := g.priorStatus[platstore.TargetRef{BlockID: blockID, Locale: locale}]
		g.noteAccepted(blockID, d.ItemName, d.Unit, locale, priorRung, model.TargetStatus(d.Status))
		out = append(out, d)
	}
	return out
}

// unitVariantRef is the ledger's key for a record.
type unitVariantRef struct{ item, unit, variant string }

// sameVerdict reports whether the venue already holds this exact verdict for
// this unit: the same rung, the same review state, and the same pairing of
// translation and source. The decider and the time are not part of it: a
// producer re-sending a record it pulled carries the platform's own
// attribution back and a producer that never pulled carries its own, and
// neither is a new decision about a verdict the ledger already holds.
func sameVerdict(a, b venue.UnitDecision) bool {
	return a.Status == b.Status &&
		a.ReviewState == b.ReviewState &&
		a.TargetHash == b.TargetHash &&
		a.ContentHash == b.ContentHash
}

// decisionLocale reads the language out of a decision's variant.
func decisionLocale(d venue.UnitDecision) string {
	var key model.VariantKey
	if err := key.UnmarshalText([]byte(d.Variant)); err != nil {
		return ""
	}
	return string(key.Locale)
}

// carriesVerdict reports whether a push claims any rung above translated or any
// review state, in its blocks or in its decision records. A push that claims
// none needs no gate, no authorship query and no permission lookup.
func carriesVerdict(staged []stagedGroup, decisions []venue.UnitDecision) bool {
	for _, group := range staged {
		for _, b := range group.Blocks {
			if b == nil {
				continue
			}
			for _, target := range b.Targets {
				if target != nil && target.Status.Rank() > model.TargetStatusTranslated.Rank() {
					return true
				}
			}
		}
	}
	for _, d := range decisions {
		if d.CarriesVerdict() {
			return true
		}
	}
	return false
}

// verdictLocales is every language a verdict in this push names, for the one
// authorship query the gate opens with.
func verdictLocales(staged []stagedGroup, decisions []venue.UnitDecision) []string {
	set := map[string]bool{}
	for _, group := range staged {
		for _, b := range group.Blocks {
			if b == nil {
				continue
			}
			for key, target := range b.Targets {
				if target != nil && target.Status.Rank() > model.TargetStatusTranslated.Rank() {
					set[string(key.Locale)] = true
				}
			}
		}
	}
	for _, d := range decisions {
		if !d.CarriesVerdict() {
			continue
		}
		if locale := decisionLocale(d); locale != "" {
			set[locale] = true
		}
	}
	out := make([]string, 0, len(set))
	for locale := range set {
		out = append(out, locale)
	}
	sort.Strings(out)
	return out
}

// report renders what the gate refused, in the shape the push status endpoint
// hands back to the producer.
func (g *pushGovernor) report() venue.PushGovernance {
	if len(g.counts) == 0 {
		return venue.PushGovernance{}
	}
	refusals := make([]venue.DecisionRefusal, 0, len(g.counts))
	for ref, count := range g.counts {
		refusals = append(refusals, venue.DecisionRefusal{
			Locale: ref.locale, Kind: ref.kind, Reason: ref.reason, Count: count,
		})
	}
	sort.Slice(refusals, func(i, j int) bool {
		if refusals[i].Locale != refusals[j].Locale {
			return refusals[i].Locale < refusals[j].Locale
		}
		if refusals[i].Kind != refusals[j].Kind {
			return refusals[i].Kind < refusals[j].Kind
		}
		return refusals[i].Reason < refusals[j].Reason
	})
	total := 0
	for _, r := range refusals {
		total += r.Count
	}
	return venue.PushGovernance{
		Refusals:       refusals,
		Units:          g.units,
		UnitsTruncated: total > len(g.units),
	}
}

// logRefusals writes one line per refused language, so a refusal is visible to
// whoever runs the venue and not only to whoever ran the push.
func (g *pushGovernor) logRefusals(ctx context.Context, projectID, stream string) {
	for _, r := range g.report().Refusals {
		slog.InfoContext(ctx, "push carried a review verdict the platform did not accept",
			"project", projectID, "stream", stream, "locale", r.Locale,
			"kind", r.Kind, "reason", r.Reason, "count", r.Count, "actor", g.actor)
	}
	if v := g.gate.Violations(); v > 0 {
		slog.InfoContext(ctx, "push carried verdicts on work the pusher wrote",
			"project", projectID, "stream", stream, "count", v,
			"policy", string(g.gate.Mode()), "actor", g.actor)
	}
}

// emitAudit records every rung this push actually moved, in the same audit
// event the web review surfaces emit, attributed to the pusher and marked as
// having arrived by push.
//
// After the transition, never inside it: an audit line for a rung change that
// rolled back would be a record of something that did not happen.
func (g *pushGovernor) emitAudit(ctx context.Context, deps *WorkerDeps, projectID, stream string) {
	if deps.EventBus == nil {
		return
	}
	keys := make([]acceptedKey, 0, len(g.accepted))
	for key := range g.accepted {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ref != keys[j].ref {
			return keys[i].ref < keys[j].ref
		}
		return keys[i].locale < keys[j].locale
	})
	rows := map[string]map[string]string{}
	for _, key := range keys {
		a := g.accepted[key]
		if a.blockID == "" {
			a.blockID = resolveStoredBlock(ctx, deps, projectID, stream, a.item, a.unit, rows)
		}
		if a.blockID == "" {
			// The venue stored the content but this unit's row cannot be
			// named, so the entry would point at nothing. Say so rather than
			// file a trail entry with an empty subject.
			slog.WarnContext(ctx, "a rung this push moved could not be named for the audit trail",
				"project", projectID, "stream", stream, "item", a.item, "unit", a.unit,
				"locale", a.locale, "actor", g.actor)
			continue
		}
		deps.EventBus.Publish(platev.Event{
			Type:         platev.EventReviewDecided,
			Source:       "sync-worker",
			ProjectID:    projectID,
			Actor:        g.actor,
			ResourceType: "block",
			ResourceID:   a.blockID,
			Data: map[string]string{
				"locale":   a.locale,
				"stream":   stream,
				"decision": review.DecisionName(true, a.to),
				"via":      "push",
			},
			Before: map[string]string{"status": string(a.from)},
			After:  map[string]string{"status": string(a.to)},
		})
	}
}

// resolveStoredBlock names the row a unit landed in, after the transition
// stored it. One read per item, remembered, because a push that promoted a
// hundred new units in one file must not read that file a hundred times.
func resolveStoredBlock(ctx context.Context, deps *WorkerDeps, projectID, stream, item, unit string, cache map[string]map[string]string) string {
	if item == "" || unit == "" {
		return ""
	}
	byUnit, read := cache[item]
	if !read {
		byUnit = map[string]string{}
		stored, err := deps.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: projectID, Stream: stream, ItemName: item,
		})
		if err == nil {
			for _, row := range stored {
				if row != nil && row.SourceID != "" {
					byUnit[row.SourceID] = row.ID
				}
			}
		}
		cache[item] = byUnit
	}
	return byUnit[unit]
}

// recordSoDViolations files one audit record for the whole push when the
// workspace policy caught verdicts on the pusher's own work.
func (g *pushGovernor) recordSoDViolations(deps *WorkerDeps, projectID string) {
	if deps.EventBus == nil || g.gate == nil || g.gate.Violations() == 0 {
		return
	}
	deps.EventBus.Publish(platev.Event{
		Type:      platev.EventType("sod.violation"),
		Source:    "sync-worker",
		ProjectID: projectID,
		Actor:     g.actor,
		Effect:    "deny",
		Data: map[string]string{
			"actor":    g.actor,
			"resource": "push:" + projectID,
			"mode":     string(g.gate.Mode()),
			"targets":  strconv.Itoa(g.gate.Violations()),
			"via":      "push",
		},
	})
}
