package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
)

// The context a reviewer decides in, for ONE unit, gathered once.
//
// Both review surfaces resolved most of this already and showed none of it: the
// queue held a voice profile id it only used to enable a button, the document
// applied content-memory matches nobody had read, and the ledger row naming who
// decided a unit and when was written by the review handler and read only by
// sync. The endpoint gathers it in the five layers a reviewer reads in:
//
//	Point          where this content sits and what governs it there
//	Neighbourhood  the units either side, as run sequences
//	History        the wording the corpus already blessed, and the last decision
//	Judgement      the findings, with their anchors and suggestions
//	Provenance     how the target was produced, and by whom it was last decided
//
// It is per-unit rather than per-queue-entry deliberately. A queue page already
// carries a full block payload per row; the neighbours would roughly triple it,
// and the memory and term lookups are per-block matcher runs — 500 of them for
// one page of the queue. A reviewer decides one unit at a time, so the context
// is fetched one unit at a time.

// reviewContextResponse is the whole context for one (block, locale) pair.
type reviewContextResponse struct {
	BlockID  string `json:"block_id"`
	ItemName string `json:"item_name"`
	Locale   string `json:"locale"`

	// Point ------------------------------------------------------------------

	// VoiceProfile is the profile bound at this point through the same
	// hierarchical ladder the worker translates under, absent when none is
	// bound.
	VoiceProfile *reviewVoiceProfile `json:"voice_profile,omitempty"`
	// Terms are the terms store's hits over this block's source, positioned —
	// the same lookup the document surface's term marks come from.
	Terms []BlockTermMatchResponse `json:"terms"`
	// CollectionID is the collection this block's item belongs to, "" for an
	// item in none; CollectionName names it when there is one.
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name,omitempty"`
	// Coordinates is the point in the project's context space the collection
	// declares (axis → value). Absent for content that sits at no declared
	// point, which is most of it until a recipe says otherwise.
	Coordinates map[string]string `json:"coordinates,omitempty"`

	// Neighbourhood ----------------------------------------------------------

	// Previous and Next are the units either side of this one in the item, each
	// carrying its source as a run sequence so a surface projects it rather
	// than concatenating it. Absent at the ends of the item.
	Previous *reviewNeighbour `json:"previous,omitempty"`
	Next     *reviewNeighbour `json:"next,omitempty"`

	// History ----------------------------------------------------------------

	// MemoryMatch is the best content-memory match for this block in this
	// locale, with the wording on both sides. Absent when the memory holds
	// nothing for it.
	MemoryMatch *MemoryMatchInfoResponse `json:"memory_match,omitempty"`
	// Decision is the ledger's latest record for this unit and locale: the
	// rung, who put it there, when, and any note. Absent for a unit awaiting
	// its first decision.
	Decision *reviewDecision `json:"decision,omitempty"`
	// Notes are the block's notes, oldest first.
	Notes []BlockNoteResponse `json:"notes"`

	// Judgement --------------------------------------------------------------

	// VoiceFindings are the findings behind the block's latest voice score, as
	// the scoring pass persisted them: each carries its own run anchor, the
	// text it was raised against, and what to say instead.
	VoiceFindings []coreprofile.VoiceFinding `json:"voice_findings"`
	// VoiceScore is that score and VoiceBar the profile's compliance bar. Both
	// absent together for a block nothing has scored.
	VoiceScore *int `json:"voice_score,omitempty"`
	VoiceBar   *int `json:"voice_bar,omitempty"`

	// Provenance -------------------------------------------------------------

	// Origin is how the target under review was produced.
	Origin *reviewOrigin `json:"origin,omitempty"`
}

// reviewVoiceProfile names the profile in force and renders what it asks for,
// so a reviewer reads the guidance rather than an id.
type reviewVoiceProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Guidance is core/profile.RenderVoiceGuideCompact — the same rendering the
	// translation prompt carries, so the reviewer judges against what the
	// producer was told.
	Guidance string `json:"guidance,omitempty"`
	// ComplianceBar is the score a block must reach to count as compliant.
	ComplianceBar int `json:"compliance_bar"`
	// TermRules are the profile's vocabulary constraints in force here: one
	// term, what to say instead, and how hard it bites.
	TermRules []coreprofile.TermRule `json:"term_rules"`
}

// reviewNeighbour is one adjacent unit, enough to read it and to move to it.
type reviewNeighbour struct {
	BlockID string `json:"block_id"`
	// SourceRuns is the run sequence, so a surface projects every kind through
	// its own RunSpec rather than concatenating the text runs.
	SourceRuns []model.Run `json:"source_runs"`
	// Status is the neighbour's rung in this locale ("" when it has no target).
	Status string `json:"status,omitempty"`
}

// reviewDecision is the ledger row for this unit, as a reviewer reads it.
type reviewDecision struct {
	// State is approved | rejected | signed-off, "" for a rung change that was
	// not a review verdict.
	State string `json:"state,omitempty"`
	// Status is the target-ladder rung the decision landed the unit on.
	Status string `json:"status,omitempty"`
	By     string `json:"by,omitempty"`
	At     string `json:"at,omitempty"`
	Note   string `json:"note,omitempty"`
	// SourceMoved marks a decision whose recorded basis names source wording
	// the block no longer carries: the verdict was for a sentence that has
	// since changed. False when the record carries no basis at all, which is
	// unknown rather than moved.
	SourceMoved bool `json:"source_moved,omitempty"`
}

// reviewOrigin is how the target under review was produced.
type reviewOrigin struct {
	Kind      string `json:"kind,omitempty"`
	Engine    string `json:"engine,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Reference string `json:"reference,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

// HandleGetReviewContext answers with everything the surfaces need to show what
// governs one unit, what surrounds it, what was decided about it before, and
// what the checks found in it.
//
// Every read here is bounded to the unit: the block, its two neighbours by
// keyset cursor, its term and memory lookups, its notes, its latest score, its
// ledger row.
func (s *Server) HandleGetReviewContext(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	// The workspace stores are named because the memory and term lookups reach
	// through them; a server without them answers no editor request at all.
	if s.ContentStore == nil || s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	ws := c.Param("ws")
	pid := projectParam(c)
	stream := streamParam(c)
	bid := c.Param("bid")
	targetLocale := strings.TrimSpace(c.QueryParam("target_locale"))
	if targetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_locale is required"})
	}

	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	loc := model.LocaleID(targetLocale)
	out := reviewContextResponse{
		BlockID:       bid,
		ItemName:      sb.ItemName,
		Locale:        targetLocale,
		Terms:         []BlockTermMatchResponse{},
		Notes:         []BlockNoteResponse{},
		VoiceFindings: []coreprofile.VoiceFinding{},
	}

	// Point: where the content sits, and what governs it there.
	s.fillReviewPoint(ctx, &out, proj, sb, stream, ws, loc)

	// Neighbourhood: the units either side, in the order the document reads.
	out.Previous, out.Next = s.reviewNeighbours(ctx, pid, stream, sb, targetLocale)

	// History: the wording the corpus already blessed, and the last decision.
	if matches, merr := editorLookupMemoryForBlock(ctx, s.ContentStore, s.wsStores, ws, pid, stream, bid, targetLocale); merr == nil && len(matches) > 0 {
		best := matches[0]
		out.MemoryMatch = &best
	}
	if notes, nerr := s.ContentStore.ListBlockNotes(ctx, pid, "main", bid); nerr == nil {
		for _, n := range notes {
			out.Notes = append(out.Notes, blockNoteToResponse(n))
		}
	}
	out.Decision = s.reviewDecisionFor(ctx, pid, stream, sb, targetLocale)

	// Judgement: the findings behind the score, not only the number.
	s.fillReviewJudgement(ctx, &out, pid, stream, bid, loc)

	// Provenance: how this target was produced.
	if t := sb.Block.Target(loc); t != nil && t.Origin != (model.Origin{}) {
		out.Origin = &reviewOrigin{
			Kind:      t.Origin.Kind,
			Engine:    t.Origin.Engine,
			Tool:      t.Origin.Tool,
			Reference: t.Origin.Reference,
			Timestamp: t.Origin.Timestamp,
			Profile:   t.Origin.Profile,
		}
	}

	return c.JSON(http.StatusOK, out)
}

// fillReviewPoint resolves the governance bound at this block's point: the
// voice profile through the same ladder the worker translates under, the terms
// the block's source actually matches, and the collection it sits in with the
// coordinates that collection declares.
func (s *Server) fillReviewPoint(ctx context.Context, out *reviewContextResponse, proj *store.Project, sb *venue.StoredBlock, stream, ws string, loc model.LocaleID) {
	if s.VoiceStore != nil {
		var wd voicescope.WorkspaceDefault
		if s.AuthStore != nil {
			wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
		}
		profile, perr := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, voicescope.Scope{
			WorkspaceID: proj.WorkspaceID,
			ProjectID:   proj.ID,
			Stream:      stream,
			Locale:      loc,
		})
		if perr == nil && profile != nil {
			out.VoiceProfile = &reviewVoiceProfile{
				ID:            profile.ID,
				Name:          profile.Name,
				Guidance:      coreprofile.RenderVoiceGuideCompact(profile),
				ComplianceBar: profile.ComplianceBar(),
				TermRules:     profileTermRules(profile),
			}
		}
	}

	if terms, terr := editorLookupTermsForBlock(ctx, s.ContentStore, s.wsStores, ws, proj.ID, stream, sb.Block.ID, string(loc)); terr == nil {
		out.Terms = append(out.Terms, terms...)
	}

	if sb.ItemName == "" {
		return
	}
	item, ierr := s.ContentStore.GetItem(ctx, proj.ID, stream, sb.ItemName)
	if ierr != nil || item == nil || item.CollectionID == "" {
		return
	}
	out.CollectionID = item.CollectionID
	if coll, cerr := s.ContentStore.GetCollection(ctx, proj.ID, item.CollectionID); cerr == nil && coll != nil {
		out.CollectionName = coll.Name
		if len(coll.Context) > 0 {
			out.Coordinates = coll.Context
		}
	}
}

// profileTermRules is the profile's vocabulary as one list of term rules: the
// preferred renderings, the forbidden terms and the competitor terms, in the
// order the guidance states them. A rule with no term is dropped — there is
// nothing for a reviewer to check against.
func profileTermRules(p *coreprofile.VoiceProfile) []coreprofile.TermRule {
	out := []coreprofile.TermRule{}
	for _, set := range [][]coreprofile.TermRule{
		p.Vocabulary.PreferredTerms,
		p.Vocabulary.ForbiddenTerms,
		p.Vocabulary.CompetitorTerms,
	} {
		for _, rule := range set {
			if strings.TrimSpace(rule.Term) != "" {
				out = append(out, rule)
			}
		}
	}
	return out
}

// reviewNeighbours reads the units either side of this one inside its item, by
// keyset cursor: one row each, wherever the block sits in the document. A block
// stored without an item has no neighbourhood to read.
func (s *Server) reviewNeighbours(ctx context.Context, pid, stream string, sb *venue.StoredBlock, locale string) (prev, next *reviewNeighbour) {
	if sb.ItemName == "" {
		return nil, nil
	}
	read := func(q store.BlockQuery) *reviewNeighbour {
		q.ProjectID, q.Stream, q.ItemName, q.Limit = pid, stream, sb.ItemName, 1
		blocks, err := s.ContentStore.GetBlocks(ctx, q)
		if err != nil || len(blocks) == 0 || blocks[0].Block == nil {
			return nil
		}
		b := blocks[0].Block
		n := &reviewNeighbour{BlockID: b.ID, SourceRuns: b.Source}
		if t := b.Target(model.LocaleID(locale)); t != nil {
			n.Status = string(t.Status)
		}
		return n
	}
	return read(store.BlockQuery{BeforeID: sb.Block.ID}), read(store.BlockQuery{AfterID: sb.Block.ID})
}

// reviewDecisionFor reads the ledger row for this unit, grading its basis
// against the source the block carries now. The unit key is the block's
// SourceID — the durable identity a decision names — so a block stored without
// an item has no ledger row to find.
func (s *Server) reviewDecisionFor(ctx context.Context, pid, stream string, sb *venue.StoredBlock, locale string) *reviewDecision {
	if sb.SourceID == "" {
		return nil
	}
	dr, ok := s.ContentStore.(store.UnitDecisionReader)
	if !ok {
		return nil
	}
	d, err := dr.GetUnitDecision(ctx, pid, stream, sb.ItemName, sb.SourceID, locale)
	if err != nil || d == nil {
		return nil
	}
	return &reviewDecision{
		State:       d.ReviewState,
		Status:      d.Status,
		By:          d.DecidedBy,
		At:          d.DecidedAt,
		Note:        d.Note,
		SourceMoved: d.ContentHash != "" && sb.ContentHash != "" && d.ContentHash != sb.ContentHash,
	}
}

// fillReviewJudgement reads the block's latest voice score and the findings the
// scoring pass persisted with it. Best-effort: an unscored block, a store
// without the per-block read, or a failed read all leave the surface with the
// check findings alone, which is what it had before.
func (s *Server) fillReviewJudgement(ctx context.Context, out *reviewContextResponse, pid, stream, bid string, loc model.LocaleID) {
	reader, ok := s.VoiceStore.(coreprofile.BlockScoreReader)
	if !ok {
		return
	}
	score, err := reader.GetBlockScore(ctx, pid, stream, bid, loc)
	if err != nil || score == nil {
		return
	}
	out.VoiceFindings = append(out.VoiceFindings, score.Findings...)
	value := score.Score
	out.VoiceScore = &value
	bar := coreprofile.DefaultMinScore
	if out.VoiceProfile != nil && out.VoiceProfile.ID == score.ProfileID {
		bar = out.VoiceProfile.ComplianceBar
	} else if s.VoiceStore != nil {
		if p, perr := s.VoiceStore.GetProfile(ctx, score.ProfileID); perr == nil {
			bar = p.ComplianceBar()
		}
	}
	out.VoiceBar = &bar
}
