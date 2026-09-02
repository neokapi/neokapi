package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// ApprovePassingRequest scopes a bulk approve-passing pass. Both fields are
// optional: an empty stream defaults to the project's default stream (then
// "main"), and an empty Locales approves across every target language the caller
// may review.
type ApprovePassingRequest struct {
	Stream  string   `json:"stream,omitempty"`
	Locales []string `json:"locales,omitempty"`
}

// ApprovePassingResponse reports what the bulk pass did. RemainingPending is the
// count of targets still awaiting review after the pass (the excluded
// failing/non-compliant blocks). ReviewCompleted is true when this pass emptied the
// project's whole review queue and kicked off the completing convergence run →
// delivery, so a surface can show "all approved · delivering…".
//
// The Skipped* fields name WHICH bar each excluded target missed — the same
// three the review queue's entries now carry, so the surface that previewed the
// pass and the response that reports it speak in one vocabulary. A target can
// miss more than one bar; it is counted against the first in gate order
// (checks, then terminology, then voice). SkippedSelfAuthored is the fourth bar
// and the only one about the caller rather than the content: a translation the
// caller wrote, in a workspace whose separation-of-duties policy blocks
// self-approval. The four sum to Skipped.
type ApprovePassingResponse struct {
	Approved              int  `json:"approved"`
	Skipped               int  `json:"skipped"`
	SkippedFailingChecks  int  `json:"skipped_failing_checks"`
	SkippedTermViolations int  `json:"skipped_term_violations"`
	SkippedBelowVoiceBar  int  `json:"skipped_below_voice_bar"`
	SkippedSelfAuthored   int  `json:"skipped_self_authored"`
	RemainingPending      int  `json:"remaining_pending"`
	ReviewCompleted       bool `json:"review_completed"`
}

// HandleApprovePassing bulk-approves every block whose target for a locale is
// awaiting review AND clears the ship bar — passes the project's checks with
// no error-severity finding AND meets the voice compliance bar (the same #1365
// shipstate predicate the dashboard aggregates). Blocks that fail checks or fall
// below the bar are EXCLUDED and left pending for a person. Approved blocks are
// promoted to reviewed; the locales that clear their review queue have their
// review task(s) closed, and if that empties the project's whole review queue
// the loop continues to a completing convergence run → delivery (RV-B).
//
// Every promotion is a decision: it goes to the decision ledger with the
// decider and the hash of the translation it blesses, and from there into the
// workspace content memory, exactly as a per-block approval does. A translation
// the caller wrote themselves is left pending when the workspace's
// separation-of-duties policy blocks self-approval.
//
// POST /:ws/:id/review/approve-passing  { "stream"?: "main", "locales"?: ["fr"] }
func (s *Server) HandleApprovePassing(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	var req ApprovePassingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found: " + err.Error()})
	}
	stream := refParamWithProject(c, proj)
	if req.Stream != "" {
		stream = req.Stream
	}

	// Candidate locales: the project's target languages, narrowed by the optional
	// locales filter and by the caller's own language scope (a reviewer scoped to
	// fr cannot approve de). project_languages empty = all languages allowed.
	wanted := map[string]bool{}
	for _, l := range req.Locales {
		wanted[l] = true
	}
	allowed, _ := c.Get("project_languages").([]string)
	allowedSet := map[string]bool{}
	for _, l := range allowed {
		allowedSet[l] = true
	}
	var locales []model.LocaleID
	for _, loc := range proj.TargetLanguages {
		ls := string(loc)
		if len(wanted) > 0 && !wanted[ls] {
			continue
		}
		if len(allowedSet) > 0 && !allowedSet[ls] {
			continue
		}
		locales = append(locales, loc)
	}

	scores := latestVoiceScores(ctx, s.VoiceStore, pid, stream)
	// The same terminology gate the dashboard ship/compliant pass uses, resolved
	// once (workspace terms snapshot + per-locale voice profile): a pending
	// draft that uses a forbidden term or misses a mandated one is non-compliant and
	// must not be auto-approved. Nil gate (no terms/voice store) is a no-op.
	wsID, _ := c.Get("workspace_id").(string)
	gate := s.resolveTermGate(ctx, proj, stream, wsID)

	approved, skipped, remaining := 0, 0, 0
	skippedBy := map[approveBlocker]int{}
	touchedSet := map[model.LocaleID]bool{}
	perLocale := map[model.LocaleID]int{}
	sodRefused, sodViolations := 0, 0
	sodActor, _ := c.Get("user_id").(string)
	sodMode := platauth.SoDOff

	// The decision ledger and the separation-of-duties policy both apply here,
	// per batch rather than per block: opening the ledger resolves the decider
	// and the workspace memory once, and one authorship query answers for a
	// whole batch. A block whose translation the caller wrote themselves is
	// left pending under a blocking policy, the same outcome a failing check
	// produces.
	ledger := s.newReviewLedger(ctx, c, pid, stream)
	localeStrings := make([]string, 0, len(locales))
	for _, loc := range locales {
		localeStrings = append(localeStrings, string(loc))
	}

	// A batch at a time: decide it, write back the blocks it changed, count
	// what it leaves pending, drop it. Reading a whole project's blocks — and
	// holding every modified one until the end — is what OOM-killed the server,
	// and "approve everything that passes" is by nature the request that touches
	// the most of a corpus at once.
	//
	// Writing per batch rather than once at the end does mean a failure part-way
	// leaves the earlier batches approved. That is the same guarantee the single
	// call gave (StoreBlocks is an upsert, not a transaction over the set), and
	// the operation is idempotent: re-running approves what is still pending.
	//
	// Safe to write during the walk: the cursor is keyed on block id and an
	// approval changes a target's status, never an id, so the page boundary the
	// next query resumes from cannot move underneath it.
	walkErr := platstore.EachBlockBatch(ctx, s.ContentStore,
		platstore.BlockQuery{ProjectID: pid, Stream: stream},
		platstore.DefaultBlockBatch,
		func(batch []*venue.StoredBlock) error {
			blockIDs := make([]string, 0, len(batch))
			for _, sb := range batch {
				if sb != nil && sb.Block != nil {
					blockIDs = append(blockIDs, sb.Block.ID)
				}
			}
			sod, err := s.newReviewSoD(ctx, c, pid, stream, blockIDs, localeStrings)
			if err != nil {
				return err
			}
			sod.quiet()

			var toStore []*model.Block
			var decisions []venue.UnitDecision
			for _, sb := range batch {
				if sb == nil || sb.Block == nil || !sb.Block.Translatable {
					continue
				}
				modified := false
				for _, loc := range locales {
					if !targetPendingReview(sb.Block, loc) {
						continue // untranslated, or already approved — not a candidate
					}
					scored := scores[string(locale.Normalize(loc))]
					blocker := blockApproveBlocker(ctx, sb.Block, loc, scored, gate)
					switch {
					case blocker != approveBlockerNone:
						// Failing/non-compliant: left pending for a person, and named
						// by the bar it missed rather than lumped into one count.
						skipped++
						skippedBy[blocker]++
					case sod.vet(sb.Block.ID, string(loc)) != nil:
						// The caller wrote this translation and the workspace
						// blocks self-approval: left pending for someone else.
						sodRefused++
					default:
						sb.Block.Target(loc).Status = model.TargetStatusReviewed
						approved++
						perLocale[loc]++
						touchedSet[loc] = true
						modified = true
						if ledger != nil && sb.SourceID != "" {
							decisions = append(decisions, unitDecisionFor(sb, string(loc),
								model.TargetStatusReviewed, true, ledger.decider))
						}
					}
				}
				if modified {
					toStore = append(toStore, sb.Block)
				}
			}

			if len(toStore) > 0 {
				if err := s.ContentStore.StoreBlocks(ctx, pid, stream, toStore); err != nil {
					return fmt.Errorf("store blocks: %w", err)
				}
			}
			// The ledger follows the write, batch by batch. Holding every
			// decision until the pass ends would reintroduce the unbounded
			// accumulation the batching exists to avoid, and the ledger write
			// is idempotent, so a pass that fails part-way leaves the batches
			// it finished consistent with the statuses it stored.
			ledger.write(ctx, decisions)
			sodViolations += sod.violations
			sodMode = sod.mode

			// What this batch leaves pending: the excluded (failing/non-compliant)
			// targets. Counted after the promotion above, over the same blocks,
			// so an approval in this pass is not also reported as outstanding.
			for _, sb := range batch {
				if sb == nil || sb.Block == nil {
					continue
				}
				for _, loc := range locales {
					if targetPendingReview(sb.Block, loc) {
						remaining++
					}
				}
			}
			return nil
		})
	if walkErr != nil {
		return serverErr(c, walkErr)
	}

	touched := make([]model.LocaleID, 0, len(touchedSet))
	for loc := range touchedSet {
		touched = append(touched, loc)
	}
	actor, _ := c.Get("user_id").(string)
	reviewCompleted := s.advanceReviewLoop(ctx, proj, stream, touched, actor)

	s.invalidateDashboardCache(wsID, pid)

	// One separation-of-duties record for the pass, with how many targets it
	// covered. A record per block would flood the bus a corpus-sized pass
	// shares with every other subscriber.
	if sodViolations > 0 {
		s.recordSoDViolation(c, sodActor, "approve_passing:"+pid+":"+stream, sodMode, sodViolations)
	}

	// One audit record per locale the pass promoted. The unit is the pass
	// because that is what the person decided: they approved a rule, and the
	// server applied it to whatever cleared the bar. Which blocks it took, and
	// who took them, is the decision ledger written above, block by block.
	for loc, n := range perLocale {
		s.emitAudit(c, auditEvent{
			Type:         platev.EventReviewBulkApproved,
			ProjectID:    pid,
			ResourceType: "project",
			ResourceID:   pid,
			Data: map[string]string{
				"locale":                string(loc),
				"stream":                stream,
				"approved":              strconv.Itoa(n),
				"skipped":               strconv.Itoa(skipped + sodRefused),
				"skipped_self_authored": strconv.Itoa(sodRefused),
			},
			After: map[string]string{"status": string(model.TargetStatusReviewed)},
		})
	}

	return c.JSON(http.StatusOK, ApprovePassingResponse{
		Approved:              approved,
		Skipped:               skipped + sodRefused,
		SkippedFailingChecks:  skippedBy[approveBlockerChecks],
		SkippedTermViolations: skippedBy[approveBlockerTerms],
		SkippedBelowVoiceBar:  skippedBy[approveBlockerVoice],
		SkippedSelfAuthored:   sodRefused,
		RemainingPending:      remaining,
		ReviewCompleted:       reviewCompleted,
	})
}
