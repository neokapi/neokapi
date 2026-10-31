package server

// Decide on the server: the questions `kapi rules` asks about the suggestions
// waiting in a project's context ledger, answered by a signed-in member.
//
// core/rulesview builds the questions and fills in their counts, and
// host/screens shapes them, the same calls the desktop's Decide makes. This
// file reads the inputs from the server's stores: the ledger's records as the
// member sees them, the collections of the stream with the coordinates each
// one sits at, and the stream's blocks, read one item at a time.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/rulesview"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/screens"
)

// The answers a member gives to a question, as `kapi rules` names them.
const (
	ruleAnswerKeep  = "keep"
	ruleAnswerDrop  = "drop"
	ruleAnswerLater = "later"
)

// registerRulesDecideRoutes adds the Decide routes under the project group
// /:ws. The stream is the :ref segment, as the item routes take it.
func (s *Server) registerRulesDecideRoutes(g *echo.Group) {
	g.GET("/:id/rules/:ref/decide", s.HandleRulesDecideView)
	g.GET("/:id/rules/:ref/decide/consequence", s.HandleRulesDecideConsequence)
	g.POST("/:id/rules/:ref/decide", s.HandleDecideRule)
	g.POST("/:id/rules/:ref/assign", s.HandleAssignRule)
	g.GET("/:id/rules/:ref/kept", s.HandleKeptRules)
	g.GET("/context/queue", s.HandleContextQueue)
}

// decideState is what every Decide call reads once: the project, its key,
// its ledger, the records as the member sees them, and the places a rule's
// scope is read against.
type decideState struct {
	project *platstore.Project
	stream  string
	key     workspace.ProjectKey
	member  LedgerMember
	reader  contextop.Actor
	ledger  *contextop.Ledger
	records []contextop.Record
	byID    map[string]contextop.Record
	places  rulesview.Places
	// items are the stream's items, and collectionOf names the collection
	// each item's path sits in.
	items        []*platstore.Item
	collectionOf map[string]string
	coordinates  map[string]map[string]string
}

// errNoProject reports a project id the content store does not hold.
var errNoProject = errors.New("project not found")

// decideState reads a project for a Decide call made by the request's
// member.
func (s *Server) decideState(c echo.Context) (*decideState, error) {
	if s.ContentStore == nil {
		return nil, errors.New("content store not configured")
	}
	cos, ok := s.contextObjects()
	if !ok {
		return nil, errors.New("context objects not configured")
	}
	ctx := c.Request().Context()
	pid := projectParam(c)
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil || proj == nil {
		return nil, errNoProject
	}
	member := ledgerMemberOf(c)
	st := &decideState{
		project: proj,
		stream:  streamParamWithProject(c, proj),
		member:  member,
		reader:  contextop.Actor{Kind: contextop.ActorPerson, Name: s.displayName(ctx, member.UserID)},
		byID:    map[string]contextop.Record{},
		places:  rulesview.Places{Space: axis.Space{}},
	}
	raw, err := cos.ContextKey(ctx, pid)
	if err != nil {
		return nil, err
	}
	st.key = workspace.ProjectKey(raw)
	if st.ledger, err = s.ContextLedger(ctx, pid, member); err != nil {
		return nil, err
	}
	if st.key != "" {
		records, rerr := st.ledger.Records(ctx, contextop.Filter{})
		if rerr != nil {
			return nil, rerr
		}
		st.records = rulesview.SeenBy(records, st.reader)
	}
	for _, r := range st.records {
		st.byID[r.ID] = r
	}
	if err := st.readPlaces(ctx, s.ContentStore); err != nil {
		return nil, err
	}
	return st, nil
}

// readPlaces reads the stream's collections and items: the coordinates each
// collection the recipe declares sits at, as the push stored them, and the
// collection each item belongs to. A collection the workspace owns, such as
// the default one an upload lands in, declares no point and is left out. The
// project's own point is the coordinates every declared collection shares,
// the project's defaults as each collection carries them.
func (st *decideState) readPlaces(ctx context.Context, cs platstore.ContentStore) error {
	collections, err := cs.ListCollections(ctx, st.project.ID, st.stream)
	if err != nil {
		return err
	}
	byID := map[string]*platstore.Collection{}
	var root map[string]string
	first := true
	for _, col := range collections {
		if col == nil || col.Name == "" || !venue.IsRecipeOwned(col.Owner) {
			continue
		}
		byID[col.ID] = col
		st.places.Collections = append(st.places.Collections, rulesview.Place{Name: col.Name, Coordinates: maps.Clone(col.Context)})
		if first {
			root, first = maps.Clone(col.Context), false
			continue
		}
		maps.DeleteFunc(root, func(axis, value string) bool { return col.Context[axis] != value })
	}
	st.places.Root = root
	if st.items, err = cs.ListItems(ctx, st.project.ID, st.stream); err != nil {
		return err
	}
	st.collectionOf = map[string]string{}
	st.coordinates = map[string]map[string]string{}
	for _, it := range st.items {
		path := itemSourcePath(it)
		col, ok := byID[it.CollectionID]
		if !ok {
			st.coordinates[path] = root
			if len(st.places.Collections) == 0 {
				st.places.Files = append(st.places.Files, rulesview.Place{Name: path, Coordinates: root})
			}
			continue
		}
		st.collectionOf[path] = col.Name
		st.coordinates[path] = col.Context
	}
	return nil
}

// itemSourcePath is the path a person knows an item's content by: the file
// it was lifted out of, or the item's own name.
func itemSourcePath(it *platstore.Item) string {
	if p := it.Properties[platstore.ItemPropSourcePath]; p != "" {
		return p
	}
	return it.Name
}

// collectionFor names the collection a path sits in, for the digest. A
// stream with no named collection has none.
func (st *decideState) collectionFor() func(string) string {
	if len(st.places.Collections) == 0 {
		return nil
	}
	return func(path string) string { return st.collectionOf[path] }
}

// questions reads the suggestions waiting as questions, without counts.
func (st *decideState) questions() []rulesview.Decision {
	if st.key == "" {
		return []rulesview.Decision{}
	}
	digest := rulesview.BuildDigest(st.records, st.key, time.Time{}, time.Now().UTC(), st.collectionFor())
	return rulesview.Questions(digest, st.records)
}

// blocks reads the source text of every block of the stream, one item at a
// time and in batches within an item, with the path and coordinates of the
// item's point. Only the text is kept.
func (st *decideState) blocks(ctx context.Context, cs platstore.ContentStore) ([]rulesview.Block, error) {
	var out []rulesview.Block
	translatable := true
	for _, it := range st.items {
		path := itemSourcePath(it)
		coordinates := st.coordinates[path]
		err := platstore.EachBlockBatch(ctx, cs, platstore.BlockQuery{
			ProjectID: st.project.ID, Stream: st.stream, ItemName: it.Name, Translatable: &translatable,
		}, 0, func(batch []*venue.StoredBlock) error {
			for _, b := range batch {
				if b == nil || b.Block == nil {
					continue
				}
				out = append(out, rulesview.Block{Path: path, Coordinates: coordinates, Text: b.SourceText()})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// counter counts over blocks where the project's places say a rule applies.
func (st *decideState) counter(blocks []rulesview.Block) rulesview.Counter {
	return rulesview.Counter{Space: st.places.Space, Applies: st.places.Applies, Blocks: slices.Values(blocks)}
}

// counted reads the questions and fills in their counts from the stream's
// blocks.
func (st *decideState) counted(ctx context.Context, cs platstore.ContentStore) ([]rulesview.Decision, []rulesview.Block, error) {
	questions := st.questions()
	var blocks []rulesview.Block
	if rulesview.NeedsBlocks(questions) {
		var err error
		if blocks, err = st.blocks(ctx, cs); err != nil {
			return nil, nil, err
		}
	}
	counter := st.counter(blocks)
	for i := range questions {
		counter.Fill(&questions[i])
	}
	return questions, blocks, nil
}

// find returns the question id belongs to and the option it names: id is a
// suggestion's own, or one side's of a disagreement. The option is -1 for a
// suggestion's own id.
func find(questions []rulesview.Decision, id string) (rulesview.Decision, int, bool) {
	for _, d := range questions {
		if id != "" && d.ID == id {
			return d, -1, true
		}
		for i, o := range d.Options {
			if id != "" && o.ID == id {
				return d, i, true
			}
		}
	}
	return rulesview.Decision{}, -1, false
}

// questionIDs are the ids a question answers to: the suggestion's, or each
// side's of a disagreement.
func questionIDs(d rulesview.Decision) []string {
	if d.ID != "" {
		return []string{d.ID}
	}
	ids := make([]string, 0, len(d.Options))
	for _, o := range d.Options {
		if o.ID != "" {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// established is every rule in force the ledger holds for the project: term
// rules and notes a person kept or the evidence settled, newest first.
func (st *decideState) established() []contextop.Record {
	var out []contextop.Record
	for _, r := range st.records {
		if r.Project != st.key || !r.Kind.Bears() || r.Kind == contextop.KindImport || !r.Established || !r.Status.Answers() {
			continue
		}
		if r.Subject.Kind != contextop.SubjectTerm && r.Subject.Kind != contextop.SubjectNote {
			continue
		}
		out = append(out, r)
	}
	return out
}

// counts are the rules the ledger holds in force, the suggestions waiting and
// the rules that hold in only part of the project.
func (st *decideState) counts() (rules, suggested, narrowed int) {
	for _, r := range st.established() {
		rules++
		if _, part := st.places.Applies(r.Scope); part {
			narrowed++
		}
	}
	for _, r := range st.records {
		if r.Project != st.key || !r.Kind.Bears() || r.Kind == contextop.KindImport || r.Established ||
			!r.Status.Advises() || r.Subject.Kind == contextop.SubjectNone {
			continue
		}
		suggested++
	}
	return rules, suggested, narrowed
}

// assignees reads who each question is assigned to, by suggestion id.
func (s *Server) assignees(ctx context.Context, projectID string) (map[string]platstore.ContextAssignment, error) {
	out := map[string]platstore.ContextAssignment{}
	as, ok := platstore.ContextAssignments(s.ContentStore)
	if !ok {
		return out, nil
	}
	list, err := as.ListContextAssignments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		out[a.SuggestionID] = a
	}
	return out, nil
}

// assigneeOf is the member a question is assigned to, as the reader sees
// them: the reader is a person with no name.
func (s *Server) assigneeOf(ctx context.Context, d rulesview.Decision, assigned map[string]platstore.ContextAssignment, reader LedgerMember) *screens.ContextActorDTO {
	for _, id := range questionIDs(d) {
		a, ok := assigned[id]
		if !ok {
			continue
		}
		who := screens.ContextActorDTO{Kind: string(contextop.ActorPerson)}
		if a.Assignee != reader.UserID {
			who.Name = s.displayName(ctx, a.Assignee)
		}
		return &who
	}
	return nil
}

// decideError answers an error reading a Decide call's state.
func decideError(c echo.Context, err error) error {
	if errors.Is(err, errNoProject) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return serverErr(c, err)
}

// HandleRulesDecideView answers GET /:ws/:id/rules/:ref/decide with every
// question waiting for a decision (screens.RulesDecideViewDTO), each option
// counted over the stream's blocks.
func (s *Server) HandleRulesDecideView(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	ctx := c.Request().Context()
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	questions, _, err := st.counted(ctx, s.ContentStore)
	if err != nil {
		return serverErr(c, err)
	}
	assigned, err := s.assignees(ctx, st.project.ID)
	if err != nil {
		return serverErr(c, err)
	}
	out := screens.RulesDecideViewDTO{
		Project:     string(st.key),
		ProjectName: st.project.Name,
		Decisions:   []screens.RuleDecisionDTO{},
	}
	out.Rules, out.Suggested, out.Narrowed = st.counts()
	for _, d := range questions {
		dto := screens.RuleDecisionFrom(d)
		dto.Assignee = s.assigneeOf(ctx, d, assigned, st.member)
		out.Decisions = append(out.Decisions, dto)
	}
	return c.JSON(http.StatusOK, out)
}

// ruleConsequence is the body of GET /:ws/:id/rules/:ref/decide/consequence.
type ruleConsequence struct {
	Consequence string `json:"consequence"`
}

// HandleRulesDecideConsequence answers GET
// /:ws/:id/rules/:ref/decide/consequence?id=&form= with what keeping the name
// id names, with form as the one to write, does: the sentence
// rulesview.Counter.Consequence writes, counted over the stream's blocks.
func (s *Server) HandleRulesDecideConsequence(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	ctx := c.Request().Context()
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	id := c.QueryParam("id")
	d, _, ok := find(st.questions(), id)
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("no suggestion %s waits for a decision", contextop.ShortID(id))})
	}
	if d.Kind != rulesview.DecisionName {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "only a name takes a form of its own"})
	}
	blocks, err := st.blocks(ctx, s.ContentStore)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, ruleConsequence{Consequence: st.counter(blocks).Consequence(d, c.QueryParam("form"))})
}

// ruleDecideRequest is the body of POST /:ws/:id/rules/:ref/decide.
type ruleDecideRequest struct {
	// ID names the suggestion, or for a disagreement the side a keep
	// chooses.
	ID string `json:"id"`
	// Answer is "keep", "drop" or "later".
	Answer string `json:"answer"`
	// Form is the form a kept name says to write. Empty keeps the form the
	// suggestion names.
	Form string `json:"form,omitempty"`
}

// decideRefusal is a decision the ledger is not asked to record, with the
// status it is answered with.
type decideRefusal struct {
	status int
	code   string
	msg    string
}

func (e *decideRefusal) Error() string { return e.msg }

// HandleDecideRule answers POST /:ws/:id/rules/:ref/decide: one answer to one
// question, recorded through the project's context ledger as the signed-in
// member. A keep records a keep of the suggestion, with the form a name
// writes; a keep of one side of a disagreement drops every rival side first;
// a drop records a drop of the suggestion, or of every side. "Later" records
// nothing. The member needs the permission the subject's kind takes
// (governingPermission) over the coordinates the suggestion holds at.
func (s *Server) HandleDecideRule(c echo.Context) error {
	var req ruleDecideRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	ctx := c.Request().Context()
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	if st.key == "" {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: ErrNoContextKey.Error()})
	}
	d, option, ok := find(st.questions(), req.ID)
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("no suggestion %s waits for a decision", contextop.ShortID(req.ID))})
	}
	out := screens.RuleDecideResultDTO{Answer: req.Answer}
	var plan []contextop.Record
	switch req.Answer {
	case ruleAnswerLater:
		out.Later = len(questionIDs(d))
		return c.JSON(http.StatusOK, out)
	case ruleAnswerKeep:
		plan, err = st.keepPlan(d, option, req.Form)
	case ruleAnswerDrop:
		plan, err = st.dropPlan(d)
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("answer %q is none of keep, drop and later", req.Answer)})
	}
	if err == nil {
		err = st.mayRecord(plan)
	}
	if refusal, ok := errors.AsType[*decideRefusal](err); ok {
		return c.JSON(refusal.status, ErrorResponse{Error: refusal.msg, Code: refusal.code})
	}
	if err != nil {
		return serverErr(c, err)
	}
	for _, r := range plan {
		if _, err := st.ledger.Append(ctx, r); err != nil {
			if errors.Is(err, ErrOutsideReach) {
				return c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error(), Code: contextOutsideReachCode})
			}
			return serverErr(c, err)
		}
		switch r.Kind {
		case contextop.KindKeep:
			out.Kept++
		case contextop.KindDrop:
			out.Dropped++
		}
	}
	return c.JSON(http.StatusOK, out)
}

// keepPlan is the records a keep of option of d writes: for a disagreement, a
// drop of every rival side that still answers and a keep of the chosen side;
// otherwise a keep of the suggestion, which edits a name to write form.
func (st *decideState) keepPlan(d rulesview.Decision, option int, form string) ([]contextop.Record, error) {
	if d.Kind == rulesview.DecisionDisagreement {
		if option < 0 || option >= len(d.Options) {
			option = d.Chosen
		}
		chosen, ok := st.byID[d.Options[option].ID]
		if !ok {
			return nil, fmt.Errorf("the ledger holds no operation %s", d.Options[option].ID)
		}
		if !chosen.Status.Answers() {
			return nil, &decideRefusal{status: http.StatusConflict, msg: fmt.Sprintf("operation %s is %s: record it again to choose it", chosen.Short, chosen.Status)}
		}
		var plan []contextop.Record
		for _, id := range chosen.ContestedBy {
			rival, ok := st.byID[id]
			if !ok || !rival.Kind.Bears() || !rival.Status.Answers() {
				continue
			}
			plan = append(plan, st.record(contextop.KindDrop, rival))
		}
		keep := st.record(contextop.KindKeep, chosen)
		keep.Scope = chosen.Scope
		return append(plan, keep), nil
	}
	target, ok := st.byID[d.ID]
	if !ok {
		return nil, fmt.Errorf("the ledger holds no operation %s", d.ID)
	}
	switch {
	case target.Status == contextop.StatusContested && !target.Established && !target.ContestedByEvidence():
		return nil, &decideRefusal{status: http.StatusConflict, msg: fmt.Sprintf("operation %s cannot be kept yet: it is contested", target.Short)}
	case !target.Status.Answers():
		return nil, &decideRefusal{status: http.StatusConflict, msg: fmt.Sprintf("operation %s is %s: record it again to keep it", target.Short, target.Status)}
	}
	if form == "" && option >= 0 && option < len(d.Options) {
		form = d.Options[option].Form
	}
	keep := st.record(contextop.KindKeep, target)
	keep.Scope = target.Scope
	if rule, ok := target.Subject.Rule(); ok && form != "" && form != rule.Replacement {
		if edited, changed := rulesview.EditSubject(target.Subject, form, nil); changed {
			keep.Subject = edited
		}
	}
	return []contextop.Record{keep}, nil
}

// dropPlan is the records a drop of d writes: a drop of the suggestion, or of
// every side of a disagreement.
func (st *decideState) dropPlan(d rulesview.Decision) ([]contextop.Record, error) {
	var plan []contextop.Record
	for _, id := range questionIDs(d) {
		target, ok := st.byID[id]
		if !ok {
			return nil, fmt.Errorf("the ledger holds no operation %s", id)
		}
		if !target.Status.Answers() {
			return nil, &decideRefusal{status: http.StatusConflict, msg: fmt.Sprintf("operation %s is already %s", target.Short, target.Status)}
		}
		plan = append(plan, st.record(contextop.KindDrop, target))
	}
	return plan, nil
}

// record is a keep or a drop of target by the member.
func (st *decideState) record(kind contextop.Kind, target contextop.Record) contextop.Record {
	return contextop.Record{
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: st.reader.Name},
		Kind:    kind,
		Target:  target.ID,
		Project: target.Project,
	}
}

// mayRecord refuses a plan with a record the member may not write: one about
// a subject whose permission the member lacks, or at a point outside the
// coordinates they hold. Nothing of a refused plan is written.
func (st *decideState) mayRecord(plan []contextop.Record) error {
	for _, r := range plan {
		target := st.byID[r.Target]
		if err := st.member.governs(target.Subject.Kind, target.Scope.Coordinates); err != nil {
			return &decideRefusal{status: http.StatusForbidden, code: contextOutsideReachCode, msg: err.Error()}
		}
	}
	return nil
}

// governs refuses a decision about a subject of kind at point that the
// member's permissions or coordinates do not cover.
func (m LedgerMember) governs(kind contextop.SubjectKind, point map[string]string) error {
	need := governingPermission(kind)
	if !m.Permissions.Has(need) {
		return fmt.Errorf("deciding this needs the %s permission", need)
	}
	if !m.Reach.Reaches(point) {
		return fmt.Errorf("this suggestion holds at %s, outside the coordinates you hold (%s)",
			describePoint(point), describeReach(m.Reach))
	}
	return nil
}

// ruleAssignRequest is the body of POST /:ws/:id/rules/:ref/assign. A null
// or empty UserID removes the assignment.
type ruleAssignRequest struct {
	ID     string  `json:"id"`
	UserID *string `json:"user_id"`
}

// ruleAssignment is what an assignment holds now.
type ruleAssignment struct {
	ID       string                   `json:"id"`
	Assignee *screens.ContextActorDTO `json:"assignee"`
}

// HandleAssignRule answers POST /:ws/:id/rules/:ref/assign: it assigns the
// question id names to a member, or to no one. An assignment is team
// workflow and records nothing in the ledger. It needs the permission
// deciding the question needs.
func (s *Server) HandleAssignRule(c echo.Context) error {
	var req ruleAssignRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	as, ok := platstore.ContextAssignments(s.ContentStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "assignments not configured"})
	}
	ctx := c.Request().Context()
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	d, _, ok := find(st.questions(), req.ID)
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("no suggestion %s waits for a decision", contextop.ShortID(req.ID))})
	}
	if err := st.member.governs(d.Subject.Kind, d.Scope.Coordinates); err != nil {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error(), Code: contextOutsideReachCode})
	}
	assignee := ""
	if req.UserID != nil {
		assignee = *req.UserID
	}
	if assignee != "" && s.AuthStore != nil {
		if m, merr := s.AuthStore.GetMembership(ctx, st.project.WorkspaceID, assignee); merr != nil || m == nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "the assignee is not a member of this workspace"})
		}
	}
	// One question holds one assignment, under the id it was asked by:
	// another side's assignment of the same disagreement goes.
	for _, id := range questionIDs(d) {
		if id == req.ID {
			continue
		}
		if err := as.AssignContextSuggestion(ctx, platstore.ContextAssignment{ProjectID: st.project.ID, SuggestionID: id}); err != nil {
			return serverErr(c, err)
		}
	}
	if err := as.AssignContextSuggestion(ctx, platstore.ContextAssignment{
		ProjectID: st.project.ID, SuggestionID: req.ID, Assignee: assignee,
		AssignedBy: st.member.UserID, AssignedAt: time.Now().UTC(),
	}); err != nil {
		return serverErr(c, err)
	}
	out := ruleAssignment{ID: req.ID}
	if assignee != "" {
		out.Assignee = s.assigneeOf(ctx, d, map[string]platstore.ContextAssignment{req.ID: {Assignee: assignee}}, st.member)
	}
	return c.JSON(http.StatusOK, out)
}

// HandleKeptRules answers GET /:ws/:id/rules/:ref/kept with every rule in
// force the project's ledger holds (screens.KeptRuleDTO), newest first, each
// with who put it in force and when, as the member sees them.
func (s *Server) HandleKeptRules(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	st, err := s.decideState(c)
	if err != nil {
		return decideError(c, err)
	}
	acts := rulesview.Acts(st.records, st.byID)
	out := []screens.KeptRuleDTO{}
	for _, r := range st.established() {
		who, at := keptBy(r, acts[r.ID])
		applies, _ := st.places.Applies(r.Scope)
		out = append(out, screens.KeptRuleDTO{
			Kind:    keptKind(r.Subject),
			Rule:    rulesview.Sentence(r.Subject),
			From:    keptFrom(r),
			Applies: applies,
			KeptBy:  screens.ActorFrom(who),
			KeptAt:  at.UTC(),
		})
	}
	slices.SortStableFunc(out, func(a, b screens.KeptRuleDTO) int { return b.KeptAt.Compare(a.KeptAt) })
	return c.JSON(http.StatusOK, out)
}

// keptBy is who put a rule in force and when: the actor of the latest keep
// or establishment that acts on it, or the rule's own actor when nothing
// does.
func keptBy(r contextop.Record, acts []contextop.Record) (contextop.Actor, time.Time) {
	who, at := r.Actor, r.At
	for _, act := range acts {
		if act.Kind == contextop.KindKeep || act.Kind == contextop.KindEstablish {
			who, at = act.Actor, act.At
		}
	}
	return who, at
}

// keptKind sorts a rule in force into the kinds Decide asks about.
func keptKind(s contextop.Subject) string {
	switch {
	case s.Kind == contextop.SubjectNote:
		return rulesview.DecisionNote
	case rulesview.ThemeOf(s) == rulesview.ThemeNames:
		return rulesview.DecisionName
	}
	return rulesview.DecisionWord
}

// keptFrom is where a rule in force is held: the terms store, or the
// workspace for a rule that holds across it. A note is held in the ledger
// alone.
func keptFrom(r contextop.Record) string {
	switch {
	case r.Subject.Kind == contextop.SubjectNote:
		return ""
	case r.Scope.Level == contextop.LevelWorkspace:
		return "workspace"
	}
	return "terms"
}

// contextQueueProject is one project of the workspace's Decide queue.
type contextQueueProject struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	// Waiting counts the questions waiting for a decision, and Assigned the
	// ones among them assigned to the caller.
	Waiting  int `json:"waiting"`
	Assigned int `json:"assigned"`
}

// contextQueue is the body of GET /:ws/context/queue.
type contextQueue struct {
	Projects []contextQueueProject `json:"projects"`
}

// HandleContextQueue answers GET /:ws/context/queue with, for every project
// of the workspace the caller can view, how many questions wait for a
// decision and how many of them are assigned to the caller. A project whose
// context no push has bound counts none.
func (s *Server) HandleContextQueue(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "content store not configured"})
	}
	cos, ok := s.contextObjects()
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context objects not configured"})
	}
	ctx := c.Request().Context()
	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return serverErr(c, err)
	}
	out := contextQueue{Projects: []contextQueueProject{}}
	for _, p := range projects {
		if p == nil || p.WorkspaceID != wsID || p.Archived {
			continue
		}
		if !s.canViewProject(c, p.ID) {
			continue
		}
		row := contextQueueProject{ProjectID: p.ID, ProjectName: p.Name}
		raw, kerr := cos.ContextKey(ctx, p.ID)
		if kerr != nil {
			return serverErr(c, kerr)
		}
		if raw != "" {
			ledger, lerr := s.ContextLedger(ctx, p.ID, LedgerMember{UserID: userID})
			if lerr != nil {
				return serverErr(c, lerr)
			}
			records, rerr := ledger.Records(ctx, contextop.Filter{})
			if rerr != nil {
				return serverErr(c, rerr)
			}
			assigned, aerr := s.assignees(ctx, p.ID)
			if aerr != nil {
				return serverErr(c, aerr)
			}
			digest := rulesview.BuildDigest(records, workspace.ProjectKey(raw), time.Time{}, time.Now().UTC(), nil)
			for _, d := range rulesview.Questions(digest, records) {
				row.Waiting++
				for _, id := range questionIDs(d) {
					if a, ok := assigned[id]; ok && a.Assignee == userID {
						row.Assigned++
						break
					}
				}
			}
		}
		out.Projects = append(out.Projects, row)
	}
	return c.JSON(http.StatusOK, out)
}

// canViewProject reports whether the caller may view a project's content,
// resolved as the project access middleware resolves it for a request that
// names the project. Without an auth store, the request's own permissions
// answer.
func (s *Server) canViewProject(c echo.Context, projectID string) bool {
	if s.AuthStore == nil {
		return hasPermission(c, platauth.PermViewContent)
	}
	userID, _ := c.Get("user_id").(string)
	wsID, _ := c.Get("workspace_id").(string)
	role, _ := c.Get("workspace_role").(platauth.Role)
	plan, _ := c.Get("workspace_plan").(string)
	access := s.resolveProjectAccess(c.Request().Context(), accessRequest{
		userID: userID, projectID: projectID, workspaceID: wsID, role: role, plan: plan,
	})
	return access.permissions.Has(platauth.PermViewContent)
}
