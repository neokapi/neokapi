package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// The review plan's settle pass runs on its own after a run drafts
// translations. A convergence run that produced work completes on the plan's
// stream, and the server runs the same pass the settle step runs by hand
// (settleReviewPlan). It runs when the policy in force leaves translations to
// the checks: a first pass, or a pass under the full policy once the context
// is decided.

// The pass is credited to the person who last set the project's
// involvement, or, when nobody has, to the person who confirmed the
// coordinates. That person must still hold the permission the settle step
// asks for; the plan says why the pass does not run when nobody qualifies.

// settleAfterRunGroup is the durable consumer group that settles a project
// after a run. Stable by design (see convergeOnPushGroup): the group's
// cursor is keyed by this name.
const settleAfterRunGroup = "review-settle-after-run"

// Project properties recording the project's last settle pass: when it ran,
// how many translations it established, who it is credited to, and whether
// the server ran it after a run ("true") or a person ran it by hand.
const (
	propSettleLastAt        = "review_settle_last_at"
	propSettleLastApproved  = "review_settle_last_approved"
	propSettleLastBy        = "review_settle_last_by"
	propSettleLastAutomatic = "review_settle_last_automatic"
)

// Who the settle pass after a run is credited to: the person who set the
// involvement, or the person who confirmed the coordinates.
const (
	autoSettleByInvolvement = "involvement"
	autoSettleByCoordinates = "coordinates"
)

// Why the settle pass after a run does not run, as the plan reports it.
const (
	// autoSettleNoOne: nobody has set the involvement or confirmed the
	// coordinates, so there is no one to credit the decisions to.
	autoSettleNoOne = "no_one"
	// autoSettleNoPermission: the person the pass would be credited to no
	// longer holds the review permission on the project.
	autoSettleNoPermission = "no_permission"
)

// Why a settle pass refuses to run.
var (
	errSettleNone = errors.New("under the involvement every, a person establishes every translation")
	errSettleHeld = errors.New("the first pass is taken; decide the context before the next one")
	errSettleBusy = errors.New("a settle pass is already running for this project")
)

// planAutoSettleDTO is who the settle pass after each run is credited to,
// or why it does not run.
type planAutoSettleDTO struct {
	By     string `json:"by,omitempty"`
	ByName string `json:"by_name,omitempty"`
	// Source is involvement or coordinates: which decision names By.
	Source string `json:"source,omitempty"`
	// Skip is no_one or no_permission while the pass does not run.
	Skip string `json:"skip,omitempty"`
}

// planSettleResultDTO is one settle pass as the project recorded it.
type planSettleResultDTO struct {
	At        string `json:"at"`
	Approved  int    `json:"approved"`
	By        string `json:"by,omitempty"`
	ByName    string `json:"by_name,omitempty"`
	Automatic bool   `json:"automatic"`
}

// lastSettleOf reads the project's last settle pass, nil before any.
func lastSettleOf(p *platstore.Project) *planSettleResultDTO {
	at := p.Properties[propSettleLastAt]
	if at == "" {
		return nil
	}
	n, _ := strconv.Atoi(p.Properties[propSettleLastApproved])
	return &planSettleResultDTO{
		At:        at,
		Approved:  n,
		By:        p.Properties[propSettleLastBy],
		Automatic: p.Properties[propSettleLastAutomatic] == "true",
	}
}

// settleReviewPlan runs one settle pass over the project as the person c
// carries: every waiting translation that clears every bar is established,
// as approve-passing does, and each decision is recorded in the ledger as
// that person's with the reason "review policy". It records the pass on the
// project, and the first pass when it is one. The settle step and the pass
// after a run both call it.
//
// It leaves out the collections under every, and refuses when the whole
// project is under every (errSettleNone), once the first
// pass is taken while the context is open (errSettleHeld), and while
// another pass runs for the project (errSettleBusy).
func (s *Server) settleReviewPlan(c echo.Context, ws *platauth.Workspace, projectID, stream string, locales []string, automatic bool) (reviewPlanSettleResponse, error) {
	ctx := c.Request().Context()
	release, ok := s.beginSettle(ctx, projectID)
	if !ok {
		return reviewPlanSettleResponse{}, errSettleBusy
	}
	defer release()
	// Read inside the guard: a pass that just finished may have taken the
	// first pass.
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return reviewPlanSettleResponse{}, errNoProject
	}
	_, pi, err := s.involvementScopeOf(ctx, proj, stream)
	if err != nil {
		return reviewPlanSettleResponse{}, err
	}
	policy, err := s.planPolicy(ctx, ws, proj, pi)
	if err != nil {
		return reviewPlanSettleResponse{}, err
	}
	switch policy.Settle {
	case settleNone:
		return reviewPlanSettleResponse{}, errSettleNone
	case settleHeld:
		return reviewPlanSettleResponse{}, errSettleHeld
	}
	// The collections under every, by the project's involvement or an
	// override at their point, are a person's to establish.
	resp, err := s.approvePassing(c, proj, stream, locales, []string{reasonReviewPolicy}, pi.every())
	if err != nil {
		return reviewPlanSettleResponse{}, err
	}
	out := reviewPlanSettleResponse{ApprovePassingResponse: resp}
	userID, _ := c.Get("user_id").(string)
	now := time.Now().UTC().Format(time.RFC3339)
	props := map[string]string{
		propSettleLastAt:        now,
		propSettleLastApproved:  strconv.Itoa(resp.Approved),
		propSettleLastBy:        userID,
		propSettleLastAutomatic: strconv.FormatBool(automatic),
	}
	if policy.Settle == settleFirstPass {
		props[propFirstPassBy] = userID
		props[propFirstPassAt] = now
		out.FirstPass = true
	}
	if err := s.updatePlanProperties(ctx, proj.ID, props); err != nil {
		return out, err
	}
	return out, nil
}

// beginSettle claims the project's settle pass and answers the release, or
// false while another pass holds it. Two guards apply: one per instance, and
// a PostgreSQL advisory lock per project that keeps instances apart. The
// lock is a session lock on a connection held for the pass, so it lets go
// when the pass ends or its connection drops. Without PostgreSQL only the
// instance guard applies.
func (s *Server) beginSettle(ctx context.Context, projectID string) (func(), bool) {
	if _, busy := s.settling.LoadOrStore(projectID, struct{}{}); busy {
		return nil, false
	}
	local := func() { s.settling.Delete(projectID) }
	db := s.advisoryDB()
	if db == nil {
		return local, true
	}
	key := settleLockKey(projectID)
	conn, err := db.Conn(ctx)
	if err != nil {
		local()
		slog.WarnContext(ctx, "review settle: no database connection for the lock", "project", projectID, "error", err)
		return nil, false
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&locked); err != nil || !locked {
		_ = conn.Close()
		local()
		return nil, false
	}
	return func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext($1))`, key)
		_ = conn.Close()
		local()
	}, true
}

// settleLockKey names the advisory lock of a project's settle pass.
func settleLockKey(projectID string) string { return "bowrain_review_settle:" + projectID }

// autoSettleActor is the person the settle pass after a run is credited to,
// with what they may do on the project.
type autoSettleActor struct {
	planAutoSettleDTO
	user   *platauth.User
	role   platauth.Role
	access projectAccess
}

// autoSettleActorOf resolves who the settle pass after a run is credited to:
// the person who last set the involvement, else the person who confirmed the
// coordinates. That person must be a member who holds the review permission
// on the project now; otherwise Skip says why.
func (s *Server) autoSettleActorOf(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project) autoSettleActor {
	var a autoSettleActor
	a.By, a.Source = proj.Properties[propInvolvementSetBy], autoSettleByInvolvement
	if a.By == "" {
		a.By, a.Source = proj.Properties[propCoordinatesConfirmedBy], autoSettleByCoordinates
	}
	if a.By == "" {
		a.Source, a.Skip = "", autoSettleNoOne
		return a
	}
	if s.AuthStore == nil {
		a.Skip = autoSettleNoPermission
		return a
	}
	if u, err := s.AuthStore.GetUser(ctx, a.By); err == nil && u != nil {
		a.user, a.ByName = u, u.Name
	}
	m, err := s.AuthStore.GetMembership(ctx, ws.ID, a.By)
	if err != nil || m == nil || a.user == nil {
		a.Skip = autoSettleNoPermission
		return a
	}
	a.role = m.Role
	a.access = s.resolveProjectAccess(ctx, accessRequest{
		userID: a.By, projectID: proj.ID, workspaceID: ws.ID, role: m.Role, plan: ws.Plan,
	})
	if !a.access.permissions.Has(platauth.PermReview) {
		a.Skip = autoSettleNoPermission
	}
	return a
}

// subscribeSettleAfterRun runs the review plan's settle pass after a run
// that produced translations. The handler always acknowledges: a pass that
// could not run is logged, and the next run tries again.
func (s *Server) subscribeSettleAfterRun() {
	if s.EventBus == nil || s.ContentStore == nil || s.AuthStore == nil {
		return
	}
	s.EventBus.SubscribeGroup(settleAfterRunGroup, func(ev platev.Event) error {
		if ev.Type != platev.EventConvergenceRunCompleted || ev.ProjectID == "" {
			return nil
		}
		switch ev.Data["state"] {
		case bstore.ConvergenceRunConverged, bstore.ConvergenceRunParked:
		default:
			return nil
		}
		// A run with no production pass drafted nothing; the completing run
		// that follows a review is one.
		if passes, _ := strconv.Atoi(ev.Data["passes"]); passes <= 0 {
			return nil
		}
		// Event-bus callback: the pass outlives the run that published the
		// event, so no request context applies.
		s.settleAfterRun(context.Background(), ev.ProjectID, ev.Data["stream"])
		return nil
	})
}

// settleAfterRun runs the settle pass for a project whose run on stream just
// completed, when the policy leaves translations to the checks and someone
// can be credited with the decisions. It answers the pass's result, or nil
// when no pass ran.
func (s *Server) settleAfterRun(ctx context.Context, projectID, stream string) *reviewPlanSettleResponse {
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		slog.WarnContext(ctx, "review settle: could not load the project", "project", projectID, "error", err)
		return nil
	}
	// The plan settles its own stream; a run on a branch leaves it alone.
	if pushedStream(stream, proj) != reviewInboxStream {
		return nil
	}
	ws, err := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID)
	if err != nil || ws == nil {
		slog.WarnContext(ctx, "review settle: could not load the workspace", "project", projectID, "error", err)
		return nil
	}
	actor := s.autoSettleActorOf(ctx, ws, proj)
	if actor.Skip != "" {
		slog.InfoContext(ctx, "review settle: skipped after a run", "project", projectID, "reason", actor.Skip, "by", actor.By)
		return nil
	}
	c := s.actingContext(ctx, ws, proj.ID, actor)
	out, err := s.settleReviewPlan(c, ws, proj.ID, reviewInboxStream, nil, true) //nolint:contextcheck // c carries ctx as its request context
	if err != nil {
		if errors.Is(err, errSettleBusy) || errors.Is(err, errSettleHeld) || errors.Is(err, errSettleNone) {
			slog.InfoContext(ctx, "review settle: skipped after a run", "project", projectID, "reason", err.Error())
			return nil
		}
		slog.WarnContext(ctx, "review settle: pass after a run failed", "project", projectID, "error", err)
		return nil
	}
	slog.InfoContext(ctx, "review settle: settled after a run", "project", projectID,
		"approved", out.Approved, "first_pass", out.FirstPass, "by", actor.By)
	return &out
}

// actingContext is a request context with no request behind it, acting for
// the person a settle pass is credited to: their id and name, the workspace
// and project, and the permissions and languages they hold there. It lets
// the pass the server runs share the settle step's code, which reads who
// acts from the request.
func (s *Server) actingContext(ctx context.Context, ws *platauth.Workspace, projectID string, a autoSettleActor) echo.Context {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", http.NoBody)
	e := s.Echo
	if e == nil {
		e = echo.New()
	}
	c := e.NewContext(req, discardResponse{header: http.Header{}})
	c.SetParamNames("ws", "id")
	c.SetParamValues(ws.Slug, projectID)
	c.Set("user_id", a.By)
	c.Set("name", a.ByName)
	c.Set("workspace_id", ws.ID)
	c.Set("workspace_role", a.role)
	c.Set("workspace_plan", ws.Plan)
	c.Set("project_permissions", a.access.permissions)
	c.Set("project_languages", a.access.languages)
	c.Set("project_coordinates", a.access.coordinates)
	return c
}

// discardResponse is the response of a request context nobody answers.
type discardResponse struct{ header http.Header }

func (d discardResponse) Header() http.Header       { return d.header }
func (discardResponse) Write(b []byte) (int, error) { return len(b), nil }
func (discardResponse) WriteHeader(int)             {}

// advisoryDB is the database advisory locks are taken on: the server's
// PostgreSQL database, or the content store's when only that is wired.
func (s *Server) advisoryDB() *sql.DB {
	if s.lockDB != nil {
		return s.lockDB
	}
	if x, ok := s.ContentStore.(interface{ SQLDB() *sql.DB }); ok {
		return x.SQLDB()
	}
	return nil
}
