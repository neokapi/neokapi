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
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
)

// The context a reviewer decides in, for ONE unit, gathered once.
//
// The shape is the review model every review client reads (core/review): the
// five layers Kapi Desktop, the CLI and the MCP tools receive from the host,
// spelled the same way and on the same scales, so a reviewer comparing the
// two surfaces reads the same facts for one unit. The platform adds beside it
// the rows only the platform holds: the unit's own address here, the
// positioned term hits its document surface marks, the block's notes, and the
// unit's voice score against its profile's bar.
//
// The pieces are resolved elsewhere for other purposes: the queue holds the
// voice profile id that enables its button, the bulk memory pass reads the
// content-memory match, and the review handler writes the ledger row naming
// who decided a unit and when. The endpoint gathers them once.
//
// It is per-unit rather than per-queue-entry deliberately. A queue page already
// carries a full block payload per row; the neighbours would roughly triple it,
// and the memory and term lookups are per-block matcher runs, 500 of them for
// one page of the queue. A reviewer decides one unit at a time, so the context
// is fetched one unit at a time.

// reviewContextResponse is the review model for one (block, locale) pair, with
// the platform's own rows beside it.
type reviewContextResponse struct {
	review.Context

	// BlockID, ItemName and Locale name the unit the context was gathered
	// for, the way the platform addresses it.
	BlockID  string `json:"block_id"`
	ItemName string `json:"item_name"`
	Locale   string `json:"locale"`
	// CollectionID is the collection this block's item belongs to, "" for an
	// item in none. The point names the collection.
	CollectionID string `json:"collection_id"`
	// Terms are the terms store's hits over this block's source, positioned:
	// the same lookup the document surface's term marks come from.
	Terms []BlockTermMatchResponse `json:"terms"`
	// Notes are the block's notes, oldest first.
	Notes []BlockNoteResponse `json:"notes"`
	// VoiceScore is the block's latest voice score, absent for a block nothing
	// has scored, and VoiceBar the bar the profile in force sets, present
	// whenever a profile is bound. The findings behind the score are the
	// judgement's findings.
	VoiceScore *int `json:"voice_score,omitempty"`
	VoiceBar   *int `json:"voice_bar,omitempty"`
}

// HandleGetReviewContext answers with everything the surfaces need to show what
// governs one unit, what surrounds it, what was decided about it before, and
// what the checks found in it.
//
// Every read here is bounded to the unit: the block, its neighbours by keyset
// cursor, its term and memory lookups, its notes, its latest score, its ledger
// row.
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
		BlockID:  bid,
		ItemName: sb.ItemName,
		Locale:   targetLocale,
		Terms:    []BlockTermMatchResponse{},
		Notes:    []BlockNoteResponse{},
	}

	// Point: where the content sits, and what governs it there.
	profile := s.fillReviewPoint(ctx, &out, proj, sb, stream, ws, loc)

	// Neighbourhood: the units either side, in the order the document reads.
	out.Neighbourhood = s.reviewNeighbourhood(ctx, pid, stream, sb, loc)

	// History: the wording the corpus already blessed.
	out.History = s.reviewHistory(ctx, &out.Point, proj, ws, sb, loc)
	if notes, nerr := s.ContentStore.ListBlockNotes(ctx, pid, "main", bid); nerr == nil {
		for _, n := range notes {
			out.Notes = append(out.Notes, blockNoteToResponse(n))
		}
	}

	// Judgement: the findings behind the score, not only the number.
	s.fillReviewJudgement(ctx, &out, profile, pid, stream, bid, loc)

	// Provenance: how this target was produced, and the decision in force.
	out.Provenance = s.reviewProvenance(ctx, pid, stream, sb, loc)

	return c.JSON(http.StatusOK, out)
}

// fillReviewPoint resolves the governance bound at this block's point: the
// voice profile through the same ladder the worker translates under, the terms
// the block's source actually matches, and the collection it sits in with the
// coordinates that collection declares. Each material degrades to a note rather
// than an error, as the host's point does: a reviewer mid-decision is better
// served by four layers than by a refusal.
//
// The resolved profile is returned so the judgement can read its bar without a
// second resolution.
func (s *Server) fillReviewPoint(
	ctx context.Context,
	out *reviewContextResponse,
	proj *store.Project,
	sb *venue.StoredBlock,
	stream, ws string,
	loc model.LocaleID,
) *coreprofile.VoiceProfile {
	p := review.Point{
		Path:     sb.ItemName,
		Language: string(loc),
		IsSource: loc != "" && loc == proj.DefaultSourceLanguage,
	}
	var profile *coreprofile.VoiceProfile
	if s.VoiceStore != nil {
		var wd voicescope.WorkspaceDefault
		if s.AuthStore != nil {
			wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
		}
		resolved, perr := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, voicescope.Scope{
			WorkspaceID: proj.WorkspaceID,
			ProjectID:   proj.ID,
			Stream:      stream,
			Locale:      loc,
		})
		switch {
		case perr != nil:
			p.Notes = append(p.Notes, "the voice bound here could not be read: "+perr.Error())
		case resolved != nil:
			profile = resolved
			p.Voice = &review.Voice{
				Name:   resolved.Name,
				Source: "store:" + resolved.ID,
				Guide:  coreprofile.RenderVoiceGuideCompact(resolved),
			}
			rules := profileTermRules(resolved)
			p.TermRules = review.LeadTermRules(rules, sb.Block.SourceText())
			p.TermsTotal = len(rules)
			// The bar the profile sets is known as soon as the profile is, so
			// an unscored unit still reads what it will be held to.
			bar := resolved.ComplianceBar()
			out.VoiceBar = &bar
		}
	}

	if terms, terr := editorLookupTermsForBlock(ctx, s.ContentStore, s.wsStores, ws, proj.ID, stream, sb.Block.ID, string(loc)); terr != nil {
		p.Notes = append(p.Notes, "the terms bound here could not be read: "+terr.Error())
	} else {
		out.Terms = append(out.Terms, terms...)
	}

	if sb.ItemName != "" {
		item, ierr := s.ContentStore.GetItem(ctx, proj.ID, stream, sb.ItemName)
		if ierr == nil && item != nil && item.CollectionID != "" {
			out.CollectionID = item.CollectionID
			if coll, cerr := s.ContentStore.GetCollection(ctx, proj.ID, item.CollectionID); cerr == nil && coll != nil {
				p.Collection = coll.Name
				if len(coll.Context) > 0 {
					p.Coordinates = coll.Context
				}
			}
		}
	}
	out.Point = p
	return profile
}

// profileTermRules is the profile's vocabulary as one list of term rules: the
// preferred renderings, the forbidden terms and the competitor terms, in the
// order the guidance states them. A rule with no term is dropped, since there
// is nothing for a reviewer to check against.
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

// reviewNeighbourhood reads the translatable units either side of this one
// inside its item, by keyset cursor: the window's worth each way, wherever the
// block sits in the document, so a reviewer reads the neighbourhood the model
// read. A block stored without an item has no neighbourhood to read.
//
// The keys are the platform's block ids, which is how its surfaces address a
// block: the same role the stable unit key plays on the host.
func (s *Server) reviewNeighbourhood(ctx context.Context, pid, stream string, sb *venue.StoredBlock, loc model.LocaleID) review.Neighbourhood {
	n := review.Neighbourhood{Key: sb.Block.ID, Window: review.DefaultWindow}
	if sb.ItemName == "" {
		return n
	}
	translatable := true
	read := func(q store.BlockQuery) []review.Neighbour {
		q.ProjectID, q.Stream, q.ItemName = pid, stream, sb.ItemName
		q.Translatable, q.Limit = &translatable, review.DefaultWindow
		blocks, err := s.ContentStore.GetBlocks(ctx, q)
		if err != nil {
			return nil
		}
		var out []review.Neighbour
		for _, stored := range blocks {
			if stored == nil || stored.Block == nil {
				continue
			}
			b := stored.Block
			src := b.SourceRuns()
			if len(src) == 0 {
				continue
			}
			nb := review.Neighbour{Key: b.ID, Source: src}
			if t := b.Target(loc); t != nil {
				nb.Target = t.Runs
				nb.Status = string(t.Status)
			}
			out = append(out, nb)
		}
		return out
	}
	// The cursor answers nearest first on both sides; Before reads nearest
	// last, so that the three lists read the document in order.
	before := read(store.BlockQuery{BeforeID: sb.Block.ID})
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}
	n.Before = before
	n.After = read(store.BlockQuery{AfterID: sb.Block.ID})
	return n
}

// reviewHistory reads what the corpus already blessed for this unit: the prior
// version from the memory's version chain, judged against the context the
// current target was produced under, and the best match with its wording. Both are
// rendered through core/review, so this assembler and the host's cannot
// disagree on a score or a chain. A memory that cannot be opened is a note on
// the point rather than an empty history a reviewer would read as "nothing
// close exists".
func (s *Server) reviewHistory(
	ctx context.Context,
	point *review.Point,
	proj *store.Project,
	ws string,
	sb *venue.StoredBlock,
	loc model.LocaleID,
) review.History {
	var h review.History
	tm, err := s.wsStores.getMemory(ws)
	if err != nil {
		point.Notes = append(point.Notes, "the content memory could not be read: "+err.Error())
		return h
	}
	source := proj.DefaultSourceLanguage

	if vr, versioned := tm.(memory.VersionReader); versioned {
		fingerprint := review.GoverningFingerprint(sb.Block, loc, model.Origin{})
		h.Prior = review.PriorVersionOf(ctx, vr, sb.Block, source, loc, fingerprint)
	}

	opts := memory.DefaultLookupOptions()
	opts.MaxResults = 1
	opts.ProjectID = proj.ID // for scoring boost
	matches, lerr := tm.Lookup(ctx, sb.Block, source, loc, opts)
	if lerr == nil && len(matches) > 0 {
		h.Match = review.MatchOf(matches[0], source, loc)
	}
	return h
}

// fillReviewJudgement reads the block's latest voice score and the findings
// the scoring pass persisted with it. Best-effort: an unscored block, a store
// without the per-block read, or a failed read all leave the surface with the
// check findings alone and the bar the point already named.
func (s *Server) fillReviewJudgement(
	ctx context.Context,
	out *reviewContextResponse,
	profile *coreprofile.VoiceProfile,
	pid, stream, bid string,
	loc model.LocaleID,
) {
	reader, ok := s.VoiceStore.(coreprofile.BlockScoreReader)
	if !ok {
		return
	}
	score, err := reader.GetBlockScore(ctx, pid, stream, bid, loc)
	if err != nil || score == nil {
		return
	}
	out.Judgement.Findings = append(out.Judgement.Findings, score.Findings...)
	value := score.Score
	out.VoiceScore = &value
	bar := coreprofile.DefaultMinScore
	if profile != nil && profile.ID == score.ProfileID {
		bar = profile.ComplianceBar()
	} else if s.VoiceStore != nil {
		if p, perr := s.VoiceStore.GetProfile(ctx, score.ProfileID); perr == nil {
			bar = p.ComplianceBar()
		}
	}
	out.VoiceBar = &bar
}

// reviewProvenance reads how the target under review was produced and the
// ledger row for this unit, grading the row's basis against the source the
// block carries now. The unit key is the block's SourceID, the durable
// identity a decision names, so a block stored without an item has no ledger
// row to find.
func (s *Server) reviewProvenance(ctx context.Context, pid, stream string, sb *venue.StoredBlock, loc model.LocaleID) review.Provenance {
	var p review.Provenance
	if d := s.unitDecisionFor(ctx, pid, stream, sb, string(loc)); d != nil {
		p.ReviewState = d.ReviewState
		p.Status = d.Status
		p.By = d.DecidedBy
		p.At = d.DecidedAt
		p.Note = d.Note
		// A decision whose recorded basis names source wording the block no
		// longer carries was a verdict on a sentence that has since changed.
		// A record with no basis at all is unknown rather than moved.
		p.Stale = d.ContentHash != "" && sb.ContentHash != "" && d.ContentHash != sb.ContentHash
	}
	if t := sb.Block.Target(loc); t != nil && t.Origin.Kind != "" {
		o := t.Origin
		p.Origin = &o
	}
	return p
}

// unitDecisionFor reads the ledger row for this unit and locale, nil when the
// store keeps none or the unit has no durable identity to look one up by.
func (s *Server) unitDecisionFor(ctx context.Context, pid, stream string, sb *venue.StoredBlock, locale string) *venue.UnitDecision {
	if sb.SourceID == "" {
		return nil
	}
	dr, ok := s.ContentStore.(store.UnitDecisionReader)
	if !ok {
		return nil
	}
	d, err := dr.GetUnitDecision(ctx, pid, stream, sb.ItemName, sb.SourceID, locale)
	if err != nil {
		return nil
	}
	return d
}
