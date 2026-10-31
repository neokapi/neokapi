package server

// The Map and the Reader on the server, in the shapes the desktop draws them
// (host/screens). The Map counts what the stores hold and reads no block. The
// Reader reads one item's blocks and checks them at the source language, as
// the project-wide check does item by item.
//
// A fix from the Reader files a source proposal. The server holds blocks, not
// files, so the fix is applied to a copy of the block and the copy's source is
// proposed in edit-text form, which keeps its inline codes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/rulesview"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host/screens"
)

// readerTermsLimit caps the word rules the Reader lists for an item, as the
// desktop's Reader caps them (host.DefaultContextTermsLimit).
const readerTermsLimit = 25

// registerMapReaderRoutes adds the Map and Reader routes under the project
// group /:ws. The stream is the :ref segment, as the item routes take it.
func (s *Server) registerMapReaderRoutes(g *echo.Group) {
	g.GET("/:id/map/:ref", s.HandleMapSummary)
	g.POST("/:id/map/:ref/check", s.HandleCheckProject)
	g.GET("/:id/reader/:ref", s.HandleReaderFile)
	g.POST("/:id/reader/:ref/fix", s.HandleReaderFix)
	g.POST("/:id/reader/:ref/fix-file", s.HandleReaderFixFile)
}

// HandleMapSummary answers GET /:ws/:id/map/:ref with the Map of the
// project's stream (screens.MapSummaryDTO). Files are the stream's items,
// each with its translatable block count and the findings its tally holds;
// the rules are those in force at the project's own point; the suggestions
// are the ledger's, as Decide reads them. Findings is nil, and CheckedAt
// empty, until a project-wide check has recorded a tally.
func (s *Server) HandleMapSummary(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	ctx := c.Request().Context()
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	out := screens.MapSummaryDTO{
		ProjectKey:      string(st.key),
		TargetLanguages: []string{},
		FileRows:        []screens.MapFileDTO{},
		Suggestions:     []screens.MapSuggestionDTO{},
	}
	for _, l := range st.project.TargetLanguages {
		out.TargetLanguages = append(out.TargetLanguages, string(l))
	}

	tallied := map[string]platstore.ItemCheckTally{}
	if ts, ok := platstore.ItemCheckTallies(s.ContentStore); ok {
		list, err := ts.ListItemCheckTallies(ctx, st.project.ID, st.stream)
		if err != nil {
			return serverErr(c, err)
		}
		var checkedAt time.Time
		findings := 0
		for _, t := range list {
			tallied[t.ItemID] = t
			findings += t.Findings
			if t.CheckedAt.After(checkedAt) {
				checkedAt = t.CheckedAt
			}
		}
		if len(list) > 0 {
			out.Findings = &findings
			out.CheckedAt = checkedAt.UTC().Format(time.RFC3339Nano)
		}
	}

	translatable := true
	for _, it := range st.items {
		counts, err := s.ContentStore.CountBlocks(ctx, platstore.BlockQuery{
			ProjectID: st.project.ID, Stream: st.stream, ItemName: it.Name, Translatable: &translatable,
		})
		if err != nil {
			return serverErr(c, err)
		}
		path := itemSourcePath(it)
		row := screens.MapFileDTO{
			// The server knows an item by its name, which the Reader takes.
			Path:       it.Name,
			Relative:   path,
			Format:     it.Format,
			Collection: st.collectionOf[path],
			Blocks:     counts.Translatable,
			Findings:   tallied[it.ID].Findings,
		}
		if row.Blocks > 0 {
			out.ReadFiles++
		}
		out.Blocks += row.Blocks
		out.FileRows = append(out.FileRows, row)
	}
	out.Files = len(out.FileRows)
	screens.SortMapFiles(out.FileRows)

	if out.Rules, err = s.mapRuleCount(ctx, c, st); err != nil {
		return serverErr(c, err)
	}

	if st.key != "" {
		digest := rulesview.BuildDigest(st.records, st.key, time.Time{}, time.Now().UTC(), st.collectionFor())
		evidence := make(map[string][]contextop.Evidence, len(st.records))
		for _, r := range st.records {
			evidence[r.ID] = r.Evidence
		}
		conflicts := make([]screens.DigestConflict, len(digest.Conflicts))
		for i, cf := range digest.Conflicts {
			conflicts[i] = screens.DigestConflictFrom(cf)
		}
		rows := screens.MapSuggestionRows(conflicts, screens.DigestSuggestedItems(digest.Suggested), evidence)
		out.Suggestions = rows.Rows
		out.Conflicts, out.Suggested = rows.Conflicts, rows.Suggested
		out.Awaiting = out.Conflicts + out.Suggested
	}
	return c.JSON(http.StatusOK, out)
}

// mapRuleCount counts the rules in force at the project's own point: the
// concepts in the workspace's terms store, the patterns of the voice bound
// there, and the term rules and notes the ledger holds established.
func (s *Server) mapRuleCount(ctx context.Context, c echo.Context, st *decideState) (int, error) {
	total := len(st.established())
	checks, err := jobs.ResolvePointChecks(ctx, s.checkBinding(c.Param("ws")), st.project.ID, st.stream, "", st.project.WorkspaceID, st.project.DefaultSourceLanguage)
	if err != nil {
		return 0, err
	}
	total += voiceRuleCount(checks.Voice)
	if checks.Terms != nil {
		_, n, err := checks.Terms.Search(ctx, "", "", "", 0, 1)
		if err != nil {
			return 0, fmt.Errorf("count terms: %w", err)
		}
		total += n
	}
	return total, nil
}

// voiceRuleCount counts the rules a voice profile applies to text: the word
// rules its file carries and its prohibited and required patterns, as the
// desktop's Map counts them.
func voiceRuleCount(p *coreprofile.VoiceProfile) int {
	if p == nil {
		return 0
	}
	n := 0
	for _, set := range coreprofile.CarriedRuleSets(p) {
		for _, r := range set.Rules {
			if strings.TrimSpace(r.Term) != "" {
				n++
			}
		}
	}
	for _, pat := range slices.Concat(p.Style.ProhibitedPatterns, p.Style.RequiredPatterns) {
		if strings.TrimSpace(pat.Regex) != "" {
			n++
		}
	}
	return n
}

// projectCheckStarted is the body POST /:ws/:id/map/:ref/check answers with:
// the job to follow at GET /:ws/jobs/:id.
type projectCheckStarted struct {
	JobID string `json:"job_id"`
}

// HandleCheckProject answers POST /:ws/:id/map/:ref/check: it enqueues a
// project-wide check of the stream (jobs.NewProjectCheckJob), which checks
// every item at the source language and records each one's tally for the
// Map. The job's progress counts items.
func (s *Server) HandleCheckProject(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.JobStore == nil || s.JobQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "content store not configured"})
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: errNoProject.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	job := jobs.NewProjectCheckJob(wsID, c.Param("ws"), proj.ID, streamParamWithProject(c, proj), userID)
	if err := s.JobStore.CreateJob(ctx, job); err != nil {
		return serverErr(c, err)
	}
	if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusAccepted, projectCheckStarted{JobID: job.ID})
}

// readerItem is one read of an item for the Reader, with the blocks it was
// drawn from and the fixes its findings carry.
type readerItem struct {
	dto    *screens.ReaderFileDTO
	item   *platstore.Item
	source model.LocaleID
	// blocks holds each stored block by the id the Reader names it by.
	blocks map[string]*model.Block
	fixes  map[string]string
	order  []string
	// blockOf names the block each fixable finding is on.
	blockOf map[string]string
	labels  map[string]string
}

// errNoItem reports an item the stream does not hold.
var errNoItem = errors.New("item not found")

// readItem reads one item of the stream for the Reader: its translatable
// blocks with the findings of the checks at the source language on each,
// and the rules that hold at the item.
func (s *Server) readItem(c echo.Context, st *decideState, name string) (*readerItem, error) {
	ctx := c.Request().Context()
	var item *platstore.Item
	for _, it := range st.items {
		if it.Name == name {
			item = it
			break
		}
	}
	if item == nil {
		return nil, errNoItem
	}
	path := itemSourcePath(item)
	out := &readerItem{item: item, source: st.project.DefaultSourceLanguage, blocks: map[string]*model.Block{}, blockOf: map[string]string{}, labels: map[string]string{}}
	var blocks []*model.Block
	var findings []screens.CheckedFinding
	checks, err := jobs.CheckItemSource(ctx, s.checkBinding(c.Param("ws")), jobs.ItemCheck{
		ProjectID: st.project.ID, Stream: st.stream, WorkspaceID: st.project.WorkspaceID, Item: item.Name, Doc: path,
	}, func(sb *venue.StoredBlock, found []jobs.ItemFinding) error {
		// The Reader names a block as the format's reader named it, as the
		// check on the desktop does; the store's own id files a proposal.
		view := *sb.Block
		if sb.SourceID != "" {
			view.ID = sb.SourceID
		}
		blocks = append(blocks, &view)
		out.blocks[view.ID] = sb.Block
		for _, f := range found {
			findings = append(findings, checkedFinding(&view, f, out.source))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	screens.SortCheckedFindings(findings)
	placed := screens.PlaceFindings(blocks, findings)
	out.fixes, out.order = placed.Fixes, placed.Order
	verdict := string(check.VerdictPassed)
	switch {
	case checks.Voice == nil:
		// At the source language only the voice and the terms bound with it
		// check anything; with none bound, nothing was checked.
		verdict = string(check.VerdictDidNotRun)
	case slices.ContainsFunc(findings, func(f screens.CheckedFinding) bool { return f.Fails }):
		verdict = string(check.VerdictFailed)
	}
	for _, b := range placed.Blocks {
		for _, f := range b.Findings {
			out.blockOf[f.ID] = b.ID
			out.labels[f.ID] = f.Label
		}
	}
	out.dto = &screens.ReaderFileDTO{
		Path:      path,
		Format:    item.Format,
		Blocks:    placed.Blocks,
		Document:  placed.Document,
		Findings:  len(findings),
		Fixable:   placed.Fixable,
		Verdict:   verdict,
		Attention: []string{},
		Refused:   []screens.ReaderRefusalDTO{},
	}
	out.dto.Rules, out.dto.RulesTotal, err = s.readerRules(ctx, st, checks, path)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// checkedFinding is a finding of the checks at the source language, as the
// Reader places it.
func checkedFinding(b *model.Block, f jobs.ItemFinding, source model.LocaleID) screens.CheckedFinding {
	cf := screens.CheckedFinding{
		BlockID:      b.ID,
		Field:        "source",
		Locale:       string(source),
		Check:        f.Family,
		Category:     f.Finding.Category,
		Rule:         screens.FindingRule(f.Finding),
		OriginalText: f.Finding.OriginalText,
		Message:      f.Finding.Message,
		Fails:        f.Finding.Fails && !f.Finding.Suggested,
		Suggested:    f.Finding.Suggested,
		Position:     f.Finding.Position,
	}
	if f.Finding.Metadata != nil {
		cf.Replacement = f.Finding.Metadata["replacement"]
		cf.Term = f.Finding.Metadata["term"]
	}
	if f.Fix != nil {
		if raw, err := json.Marshal(f.Fix); err == nil {
			cf.Fix = string(raw)
		}
	}
	return cf
}

// readerRules lists the rules that hold at an item, in the order `kapi
// context <path>` lists them. The voice bound at its point comes first. The
// word rules of the terms and of the voice's own file follow
// (rulesview.SayThisNotThat), capped at readerTermsLimit. Last come the term
// rules and notes the ledger holds established at the item's point, each with
// who put it in force and when. The total counts every rule.
func (s *Server) readerRules(ctx context.Context, st *decideState, checks jobs.PointChecks, path string) ([]screens.ReaderRuleDTO, int, error) {
	out := []screens.ReaderRuleDTO{}
	if checks.Voice != nil {
		if row, ok := screens.ReaderToneRule(coreprofile.RenderVoiceBrief(checks.Voice), checks.Voice.Name); ok {
			out = append(out, row)
		}
	}
	var hits []rulesview.TermHit
	if checks.Terms != nil {
		concepts, err := checks.Terms.Concepts(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("read the terms: %w", err)
		}
		hits, _ = rulesview.TermsInForce(concepts, "", time.Now().UTC(), 0)
	}
	words, wordsTotal := rulesview.SayThisNotThat(hits, nil, checks.Voice, readerTermsLimit)
	for _, r := range words {
		out = append(out, screens.ReaderWordRule(r))
	}
	extra := wordsTotal - len(words)

	point := st.coordinates[path]
	acts := rulesview.Acts(st.records, st.byID)
	for _, r := range st.established() {
		if r.Scope.Level != contextop.LevelWorkspace && !r.Scope.CoversIn(st.places.Space, point) {
			continue
		}
		who, at := keptBy(r, acts[r.ID])
		out = append(out, screens.ReaderRuleDTO{
			Kind:   keptKind(r.Subject),
			Rule:   rulesview.Sentence(r.Subject),
			Status: "established",
			From:   keptFrom(r),
			By:     who.Name,
			At:     at.UTC().Format(time.RFC3339),
		})
	}
	return out, len(out) + extra, nil
}

// readerError answers an error reading an item for the Reader.
func readerError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, errNoProject), errors.Is(err, errNoItem):
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return serverErr(c, err)
}

// HandleReaderFile answers GET /:ws/:id/reader/:ref?item=<name> with one
// item as the Reader draws it (screens.ReaderFileDTO). It writes nothing,
// so Refused is always empty.
func (s *Server) HandleReaderFile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	name := c.QueryParam("item")
	if name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "item is required"})
	}
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	r, err := s.readItem(c, st, name)
	if err != nil {
		return readerError(c, err)
	}
	return c.JSON(http.StatusOK, r.dto)
}

// readerFixRequest is the body of POST /:ws/:id/reader/:ref/fix and
// /fix-file. FindingID is the finding to fix; fix-file takes none.
type readerFixRequest struct {
	Item      string `json:"item"`
	FindingID string `json:"finding_id,omitempty"`
}

// ReaderFixResult is what a fix from the Reader filed: one source proposal
// per block it changes, and the fixes that could not be proposed, each with
// the reason.
type ReaderFixResult struct {
	Proposals []string                   `json:"proposals"`
	Refused   []screens.ReaderRefusalDTO `json:"refused"`
}

// HandleReaderFix answers POST /:ws/:id/reader/:ref/fix {item, finding_id}:
// it files the finding's fix as a source proposal for the block it is on,
// and answers with the proposal's id.
func (s *Server) HandleReaderFix(c echo.Context) error {
	return s.readerFix(c, false)
}

// HandleReaderFixFile answers POST /:ws/:id/reader/:ref/fix-file {item}: it
// files every fixable finding of the item as source proposals, one per
// block.
func (s *Server) HandleReaderFixFile(c echo.Context) error {
	return s.readerFix(c, true)
}

// readerFix files fixes as source proposals. A fix is the change operation
// its finding carries (check.Fix), applied to the stored block in memory
// with the block's other fixes (change.ApplyBlock). The proposal is the
// copy's source in edit-text form (model.RunsEditText), where each inline code
// stands as a marker that approval maps back to the block's own code, so an
// approved fix keeps every code. Filing needs the permission a source
// proposal needs.
func (s *Server) readerFix(c echo.Context, whole bool) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}
	if s.SourceProposalStore == nil || s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "source proposals not configured"})
	}
	var req readerFixRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.Item == "" || (!whole && req.FindingID == "") {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "item and finding_id are required"})
	}
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	r, err := s.readItem(c, st, req.Item)
	if err != nil {
		return readerError(c, err)
	}
	out := ReaderFixResult{Proposals: []string{}, Refused: []screens.ReaderRefusalDTO{}}
	ids := r.order
	if !whole {
		if _, ok := r.fixes[req.FindingID]; !ok {
			reason := "This finding is no longer reported."
			if _, held := r.labels[req.FindingID]; held {
				reason = "This finding has no fix."
			}
			out.Refused = append(out.Refused, screens.ReaderRefusalDTO{FindingID: req.FindingID, Reason: reason})
			return c.JSON(http.StatusOK, out)
		}
		ids = []string{req.FindingID}
	}

	// The fixes by block, in file order.
	var order []string
	byBlock := map[string][]string{}
	for _, id := range ids {
		b := r.blockOf[id]
		if _, ok := byBlock[b]; !ok {
			order = append(order, b)
		}
		byBlock[b] = append(byBlock[b], id)
	}
	ctx := c.Request().Context()
	for _, blockID := range order {
		proposed, refused := r.proposedSource(blockID, byBlock[blockID])
		out.Refused = append(out.Refused, refused...)
		if proposed == nil {
			continue
		}
		labels := make([]string, 0, len(byBlock[blockID]))
		for _, id := range byBlock[blockID] {
			labels = append(labels, r.labels[id])
		}
		p, err := s.fileSourceProposal(ctx, c, st.project, st.stream, r.item.Name, r.blocks[blockID], bstore.SourceProposalTextFix,
			model.RunsEditText(proposed.SourceRuns()), "Fix from the Reader: "+strings.Join(labels, "; "), "")
		if err != nil {
			return serverErr(c, err)
		}
		out.Proposals = append(out.Proposals, p.ID)
	}
	return c.JSON(http.StatusOK, out)
}

// proposedSource applies the fixes of ids to a copy of the block, as one
// set of operations, and returns the copy. When the change model refuses
// them, it returns nil and each fix with the reason.
func (r *readerItem) proposedSource(blockID string, ids []string) (*model.Block, []screens.ReaderRefusalDTO) {
	refuse := func(reason string) []screens.ReaderRefusalDTO {
		out := make([]screens.ReaderRefusalDTO, len(ids))
		for i, id := range ids {
			out[i] = screens.ReaderRefusalDTO{FindingID: id, Reason: reason}
		}
		return out
	}
	b, ok := r.blocks[blockID]
	if !ok {
		return nil, refuse("The block this finding is on is no longer stored.")
	}
	ops := make([]change.Op, 0, len(ids))
	for _, id := range ids {
		var op change.Op
		if err := json.Unmarshal([]byte(r.fixes[id]), &op); err != nil {
			return nil, refuse("The fix could not be read.")
		}
		ops = append(ops, op)
	}
	edited := b.CopyEditionSet()
	if edited.SourceLocale == "" {
		// The revision a fix names is read in the project's source language
		// (check.SourceRevision).
		edited.SourceLocale = r.source
	}
	for _, res := range change.ApplyBlock(edited, ops, change.BlockEnv{}) {
		if res.Status == change.OpApplied {
			continue
		}
		reason := "The change was refused."
		if res.Error != nil && res.Error.Message != "" {
			reason = res.Error.Message
		}
		return nil, refuse(reason)
	}
	return edited, nil
}
