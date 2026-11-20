package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
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

// ApprovePassingResponse reports what the bulk pass did. RemainingPending is
// the count of targets still awaiting review after the pass. ReviewCompleted
// is true when this pass emptied the project's whole review queue and started
// the completing convergence run and delivery.
//
// The Skipped* fields name the bar each excluded target missed, the same bars
// the review queue's entries carry. A target that misses several is counted
// against the first in gate order: checks, then terminology, then voice.
// SkippedTermsNotChecked and SkippedVoiceNotChecked count targets in a locale
// that dimension governs with no verdict or score yet. SkippedSelfAuthored
// counts translations the caller wrote, in a workspace whose
// separation-of-duties policy blocks self-approval. The six sum to Skipped.
type ApprovePassingResponse struct {
	Approved               int  `json:"approved"`
	Skipped                int  `json:"skipped"`
	SkippedFailingChecks   int  `json:"skipped_failing_checks"`
	SkippedTermViolations  int  `json:"skipped_term_violations"`
	SkippedTermsNotChecked int  `json:"skipped_terms_not_checked"`
	SkippedBelowVoiceBar   int  `json:"skipped_below_voice_bar"`
	SkippedVoiceNotChecked int  `json:"skipped_voice_not_checked"`
	SkippedSelfAuthored    int  `json:"skipped_self_authored"`
	RemainingPending       int  `json:"remaining_pending"`
	ReviewCompleted        bool `json:"review_completed"`
}

// HandleApprovePassing bulk-approves every block whose target for a locale is
// awaiting review and clears the ship bar: no error-severity finding from the
// project's checks, term-compliant for the locale, and at or above the voice
// compliance bar. A block that misses a bar, or lacks a result for a bar that
// governs the locale, is left pending for a person. When the whole review
// queue empties, the loop continues to a completing convergence run and
// delivery.
//
// Every promotion is a decide operation sent through the stream's change
// service as the caller, bound to the translation the pass read. It reaches
// the decision ledger and the workspace content memory as any other approval
// does. A translation that moved since the pass read it stays pending, as
// does one the caller wrote under a separation-of-duties policy that blocks
// self-approval.
//
// POST /:ws/:id/review/approve-passing { "stream"?: "main", "locales"?: ["fr"] }
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

	resp, err := s.approvePassing(c, proj, stream, req.Locales, nil, nil)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

// approvePassing runs one approve-passing pass over the stream, in the
// project's target languages narrowed to wantedLocales (all when empty) and to
// the caller's language scope. Each decision carries reasons in the ledger:
// the review plan's settle step passes "review policy". The items of
// skipCollections are left alone ("" names the items in no collection).
func (s *Server) approvePassing(c echo.Context, proj *platstore.Project, stream string, wantedLocales, reasons, skipCollections []string) (ApprovePassingResponse, error) {
	ctx := c.Request().Context()
	pid := proj.ID
	skipItems, err := itemsInCollections(ctx, s.ContentStore, pid, stream, skipCollections)
	if err != nil {
		return ApprovePassingResponse{}, err
	}
	// Candidate locales: the project's target languages, narrowed by the optional
	// locales filter and by the caller's own language scope (a reviewer scoped to
	// fr cannot approve de). project_languages empty = all languages allowed.
	wanted := map[string]bool{}
	for _, l := range wantedLocales {
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

	// The decisions of the whole pass go through one change of the stream:
	// its ledger resolves the decider and the workspace memory once, and one
	// authorship query per batch answers the separation-of-duties policy. A
	// block whose translation the caller wrote themselves is left pending
	// under a blocking policy, the same outcome a failing check produces. The
	// review loop continues once, after the pass.
	sc := s.newStreamChange(ctx, c, proj, stream, wsID, c.Param("ws"), requestSender(c))
	sc.holdLoop = true
	sc.decide.quiet = true
	person := change.Actor{Kind: change.ActorPerson, Name: sc.sender.userID}
	localeStrings := make([]string, 0, len(locales))
	for _, loc := range locales {
		localeStrings = append(localeStrings, string(loc))
	}

	// A batch at a time: decide it, write back the blocks it changed, count
	// what it leaves pending, drop it. Reading a whole project's blocks, and
	// holding every modified one until the end, runs the server out of memory
	// on a large corpus.
	//
	// A failure part-way leaves the earlier batches approved. The operation is
	// idempotent: running it again approves what is still pending.

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
			sc.decide.sod = sod

			set := change.Set{}
			pending := 0
			for _, sb := range batch {
				if sb == nil || sb.Block == nil || !sb.Block.Translatable || skipItems[sb.ItemName] {
					continue
				}
				for _, loc := range locales {
					if !targetPendingReview(sb.Block, loc) {
						continue // untranslated, or already approved: not a candidate
					}
					scored := scores[string(locale.Normalize(loc))]
					blocker := blockApproveBlocker(ctx, sb.Block, loc, scored, gate)
					switch {
					case blocker != approveBlockerNone:
						// Failing/non-compliant: left pending for a person, and named
						// by the bar it missed rather than lumped into one count.
						skipped++
						skippedBy[blocker]++
						pending++
					case sod.vet(sb.Block.ID, string(loc)) != nil:
						// The caller wrote this translation and the workspace
						// blocks self-approval: left pending for someone else.
						sodRefused++
						pending++
					default:
						set.Ops = append(set.Ops, change.Op{Kind: change.KindDecide,
							At:      change.Ref{Doc: sb.ItemName, Block: sb.Block.ID, Edition: model.EditionKey{Locale: loc}},
							IfMatch: platstore.TargetRevision(sb, loc),
							Body:    &change.Decide{Outcome: change.OutcomeEstablish, Reasons: reasons}})
					}
				}
			}
			sodViolations += sod.violations()
			sodMode = sod.mode()
			if len(set.Ops) == 0 {
				remaining += pending
				return nil
			}

			res, applied, _, err := sc.applyEach(ctx, set, person)
			if err != nil {
				return err
			}
			// What this batch leaves pending: the excluded targets, and an
			// approval the change service refused because its translation
			// moved since the pass read it.
			landed := 0
			if res != nil && res.Status != change.SetRefused {
				for i, op := range res.Ops {
					if op.Status != change.OpApplied && op.Status != change.OpUnchanged {
						continue
					}
					landed++
					if op.Status == change.OpApplied {
						loc := applied[i].At.Edition.Locale
						approved++
						perLocale[loc]++
						touchedSet[loc] = true
					}
				}
			}
			remaining += pending + len(set.Ops) - landed
			return nil
		})
	if walkErr != nil {
		return ApprovePassingResponse{}, walkErr
	}

	touched := make([]model.LocaleID, 0, len(touchedSet))
	for loc := range touchedSet {
		touched = append(touched, loc)
	}
	actor, _ := c.Get("user_id").(string)
	reviewCompleted := s.advanceReviewLoop(ctx, proj, stream, touched, actor)

	s.shipInputsChanged(ctx, wsID, pid, stream)

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
			After: map[string]string{"status": string(model.TargetStatusEstablished)},
		})
	}

	return ApprovePassingResponse{
		Approved:               approved,
		Skipped:                skipped + sodRefused,
		SkippedFailingChecks:   skippedBy[approveBlockerChecks],
		SkippedTermViolations:  skippedBy[approveBlockerTerms],
		SkippedTermsNotChecked: skippedBy[approveBlockerTermsNotChecked],
		SkippedBelowVoiceBar:   skippedBy[approveBlockerVoice],
		SkippedVoiceNotChecked: skippedBy[approveBlockerVoiceNotChecked],
		SkippedSelfAuthored:    sodRefused,
		RemainingPending:       remaining,
		ReviewCompleted:        reviewCompleted,
	}, nil
}
