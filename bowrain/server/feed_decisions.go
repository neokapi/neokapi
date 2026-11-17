package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// feedDecisionLimit bounds the decisions one roll-up lists: an hour of one
// reviewer's work in one language.
const feedDecisionLimit = 500

// feedDecision is one review decision a feed roll-up counts, beside the
// translation as it stands now.
type feedDecision struct {
	// Card is the translation as it stands now, in the shape the review deck
	// deals, so a client withdraws the decision on its current revision.
	Card     deckCard  `json:"card"`
	Decision string    `json:"decision"`
	At       time.Time `json:"at"`
	// Current is true while this is the newest decision on the translation
	// and the translation still stands where the decision left it.
	Current bool `json:"current"`
	// CanUndo is a current approval or rejection the reader made: a client
	// offers to withdraw it.
	CanUndo bool `json:"can_undo"`
}

type feedDecisionsResponse struct {
	Locale    string         `json:"locale"`
	Decisions []feedDecision `json:"decisions"`
	// BulkApproved counts the approvals of a bulk pass, which records the
	// pass and no decision per translation.
	BulkApproved int `json:"bulk_approved"`
}

// HandleFeedDecisions lists the decisions a review roll-up in the feed
// counts: GET /:ws/:id/feed/:entry/decisions. Each comes with the translation
// as it stands now, whether the decision still holds, and whether the reader
// may undo it.
func (s *Server) HandleFeedDecisions(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if s.ActivityStore == nil || s.AuditLogger == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "this server keeps no feed"})
	}
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: errNoProject.Error()})
	}
	entry, err := s.ActivityStore.Get(ctx, c.Param("entry"))
	if err != nil {
		return serverErr(c, err)
	}
	if entry == nil || entry.ProjectID != proj.ID || entry.Kind != bstore.FeedKindReviewSummary {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no review entry with that id in this project"})
	}
	locale := entry.EntityID
	stream := entry.Stream
	if stream == "" {
		stream = streamParamWithProject(c, proj)
	}

	rows, err := s.AuditLogger.RollupEntries(ctx, entry.ID, feedDecisionLimit)
	if err != nil {
		return serverErr(c, err)
	}
	// One line per translation: its newest decision in the roll-up.
	var ids []string
	seen := map[string]bool{}
	var decided []auditDecision
	for _, r := range rows {
		if r.EventType != string(platev.EventReviewDecided) || r.ResourceType != "block" || seen[r.ResourceID] {
			continue
		}
		d := parseAuditDecision(r)
		if d.locale != locale {
			continue
		}
		seen[r.ResourceID] = true
		ids = append(ids, r.ResourceID)
		decided = append(decided, d)
	}

	blocks, err := s.deckBlocks(ctx, proj.ID, stream, ids)
	if err != nil {
		return serverErr(c, err)
	}
	latest, err := s.AuditLogger.LatestBlockDecisions(ctx, proj.ID, locale, ids)
	if err != nil {
		return serverErr(c, err)
	}

	viewer, _ := c.Get("user_id").(string)
	loc := model.LocaleID(locale)
	resp := feedDecisionsResponse{Locale: locale, Decisions: []feedDecision{}, BulkApproved: entry.Counts["bulk_approved"]}
	for _, d := range decided {
		sb := blocks[d.blockID]
		if sb == nil {
			continue
		}
		b := sb.Block
		target, _ := b.TargetEdition(loc)
		newest := latest[d.blockID]
		current := newest.ID == d.id && string(target.Status) == d.to
		resp.Decisions = append(resp.Decisions, feedDecision{
			Card: deckCard{
				BlockID:        b.ID,
				ItemName:       sb.ItemName,
				Key:            b.Name,
				Locale:         locale,
				SourceRuns:     nonNilRuns(b.SourceRuns()),
				TargetRuns:     nonNilRuns(target.Runs),
				SourceText:     model.RunsDisplayText(b.SourceRuns()),
				TargetText:     model.RunsDisplayText(target.Runs),
				TargetEditText: model.RunsEditText(target.Runs),
				SourceRevision: check.SourceRevision(b, proj.DefaultSourceLanguage),
				TargetRevision: store.TargetRevision(sb, loc),
				TargetStatus:   string(target.Status),
				Notes:          []deckNote{},
			},
			Decision: d.decision,
			At:       d.at,
			Current:  current,
			CanUndo:  current && viewer != "" && d.actor == viewer && (d.decision == "approved" || d.decision == "rejected"),
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// auditDecision is a review.decided audit entry, read.
type auditDecision struct {
	id       int64
	blockID  string
	actor    string
	locale   string
	decision string
	// to is the status the decision moved the translation to.
	to string
	at time.Time
}

func parseAuditDecision(r event.AuditEntry) auditDecision {
	var data, after map[string]string
	_ = json.Unmarshal([]byte(r.Data), &data)
	if r.After != "" {
		_ = json.Unmarshal([]byte(r.After), &after)
	}
	return auditDecision{
		id:       r.ID,
		blockID:  r.ResourceID,
		actor:    r.Actor,
		locale:   data["locale"],
		decision: data["decision"],
		to:       after["status"],
		at:       r.CreatedAt,
	}
}
