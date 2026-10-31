package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
)

// desktop is a machine's workspace that records context and pushes it to a
// folder, as kapi does for a project whose context is shared (the
// workspace.Sync that host's contextSync links a project to).
type desktop struct {
	ws     *workspace.Workspace
	key    workspace.ProjectKey
	folder string
}

func newDesktop(t *testing.T, key workspace.ProjectKey) *desktop {
	t.Helper()
	ws, err := workspace.OpenLocal(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	return &desktop{ws: ws, key: key, folder: t.TempDir()}
}

// observe records a term rule suggestion under a person's name.
func (d *desktop) observe(t *testing.T, term, instead string) contextop.Record {
	t.Helper()
	r, err := contextop.NewLedger(d.ws, nil).Append(t.Context(), contextop.Record{
		Project: d.key,
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"},
		Kind:    contextop.KindObserve,
		Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: instead, Replacement: term}},
	})
	require.NoError(t, err)
	return r
}

// push writes what the desktop recorded to its folder and returns the
// segments the push wrote there, as objects of the context layout.
func (d *desktop) push(t *testing.T) []workspace.Object {
	t.Helper()
	before := d.segments(t)
	report, err := d.ws.NewSync(workspace.NewFileRemote(d.folder), d.key, nil, workspace.SyncOptions{CheckpointEvery: -1}).Push(t.Context())
	require.NoError(t, err)
	require.Positive(t, report.Pushed)
	var out []workspace.Object
	for name, data := range d.segments(t) {
		if _, ok := before[name]; !ok {
			out = append(out, workspace.Object{Name: name, Data: data})
		}
	}
	require.NotEmpty(t, out)
	return out
}

func (d *desktop) segments(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	root := filepath.Join(d.folder, "log")
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(d.folder, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return out
	}
	require.NoError(t, err)
	return out
}

// pullFrom merges what a remote holds into the desktop's log.
func (d *desktop) pullFrom(t *testing.T, r workspace.Remote) workspace.PullReport {
	t.Helper()
	report, err := d.ws.NewSync(r, d.key, nopApplier{}, workspace.SyncOptions{CheckpointEvery: -1}).Pull(t.Context())
	require.NoError(t, err)
	return report
}

// nopApplier applies nothing: the desktops of these tests hold the log alone.
type nopApplier struct{}

func (nopApplier) Apply(context.Context, bool) error { return nil }
func (nopApplier) Checkpoint(context.Context, []string) ([]byte, string, error) {
	return nil, "", errors.New("no checkpoints")
}
func (nopApplier) CheckpointMark([]byte) (string, []string, []string, error) {
	return "", nil, nil, errors.New("no checkpoints")
}
func (nopApplier) InstallCheckpoint(context.Context, []byte) error {
	return errors.New("no checkpoints")
}

// countingObjects counts the context objects read through it.
type countingObjects struct {
	*sqlitestore.SQLiteStore
	gets atomic.Int64
}

func (s *countingObjects) GetContextObject(ctx context.Context, projectID, name string) (*platstore.ContextObject, error) {
	s.gets.Add(1)
	return s.SQLiteStore.GetContextObject(ctx, projectID, name)
}

// ledgerSQLiteServer serves the context object routes over a SQLite content
// store, signing every request in as member-a.
func ledgerSQLiteServer(t *testing.T) (*Server, *echo.Echo, *countingObjects) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	counted := &countingObjects{SQLiteStore: cs}
	srv := &Server{ContentStore: counted, wsStores: newWorkspaceStores()}
	e := echo.New()
	srv.registerContextObjectRoutes(e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", "member-a")
			c.Set("project_permissions", platauth.PermViewContent|contextObjectWritePermission)
			return next(c)
		}
	}))
	return srv, e, counted
}

// pushBody encodes objects as the push route takes them.
func pushBody(t *testing.T, objs []workspace.Object) []byte {
	t.Helper()
	batch := contextObjectBatch{}
	for _, o := range objs {
		batch.Objects = append(batch.Objects, contextObjectWire{Name: o.Name, Data: o.Data})
	}
	body, err := json.Marshal(batch)
	require.NoError(t, err)
	return body
}

// ledgerRoundTrip is the claim on either store: the server reads the
// operations a desktop pushed, records a keep under the signed-in member,
// folds both, and the desktop pulls the keep back.
func ledgerRoundTrip(t *testing.T, srv *Server, r *routeRemote, projectID, userID, memberName string) {
	t.Helper()
	ctx := t.Context()
	d := newDesktop(t, "site")
	observed := d.observe(t, "use", "utilise")
	require.NoError(t, r.Put(ctx, d.push(t)...))

	cos, ok := srv.contextObjects()
	require.True(t, ok)
	key, err := cos.ContextKey(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, "site", key, "the first push binds the project key")

	ledger, err := srv.ContextLedger(ctx, projectID, LedgerMember{UserID: userID, Permissions: platauth.PermManageTerms})
	require.NoError(t, err)
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, observed.ID, records[0].ID)
	assert.Equal(t, contextop.StatusSuggested, records[0].Status)
	assert.Equal(t, "asgeir", records[0].Actor.Name, "the desktop's actor is read back")
	rule, ok := records[0].Rule()
	require.True(t, ok)
	assert.Equal(t, "use", rule.Replacement)

	kept, err := ledger.Append(ctx, contextop.Record{
		Kind:   contextop.KindKeep,
		Target: records[0].Short,
		Actor:  contextop.Actor{Kind: contextop.ActorPerson, Name: "a name the caller made up"},
	})
	require.NoError(t, err)

	names, err := cos.ListContextObjects(ctx, projectID, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Contains(t, names, workspace.SegmentName(contextLedgerWriter, kept.ID), "the keep is one new segment under the server's writer id")

	records, err = ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.Len(t, records, 2)
	byID := map[string]contextop.Record{}
	for _, rec := range records {
		byID[rec.ID] = rec
	}
	assert.Equal(t, contextop.StatusEstablished, byID[observed.ID].Status, "the fold reads the keep")
	assert.Equal(t, contextop.Actor{Kind: contextop.ActorPerson, Name: memberName}, byID[kept.ID].Actor,
		"the keep carries the signed-in member")

	pulled := d.pullFrom(t, r)
	assert.Equal(t, 1, pulled.Merged, "the desktop decodes the segment the server wrote")
	onDesktop, err := contextop.NewLedger(d.ws, nil).Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	for _, rec := range onDesktop {
		if rec.ID == observed.ID {
			assert.Equal(t, contextop.StatusEstablished, rec.Status)
		}
	}
}

func TestContextLedger_RoundTrip_SQLite(t *testing.T) {
	srv, e, cs := ledgerSQLiteServer(t)
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	// The SQLite server has no auth store, so the member's name is its id.
	ledgerRoundTrip(t, srv, r, pid, "member-a", "member-a")
}

// A push naming another project key, two keys, or a checkout's key is
// refused and writes nothing; a write to a project no push has bound is
// refused too.
func TestContextLedger_KeyRefusals_SQLite(t *testing.T) {
	ctx := t.Context()
	srv, e, cs := ledgerSQLiteServer(t)

	bound := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + bound + "/context/objects"}
	site := newDesktop(t, "site")
	site.observe(t, "use", "utilise")
	siteObjects := site.push(t)
	require.NoError(t, r.Put(ctx, siteObjects...))

	other := newDesktop(t, "other")
	other.observe(t, "sign in", "login")
	otherObjects := other.push(t)
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, otherObjects))
	require.Equal(t, http.StatusConflict, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextKeyMismatchCode, refusal.Code)
	assert.Equal(t, "this project's context belongs to project key site, and the push carries project key other", refusal.Error)
	names, err := cs.ListContextObjects(ctx, bound, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Len(t, names, 1, "a refused push writes nothing")

	fresh := seedContextObjectProject(t, cs)
	freshBase := "/api/v1/ws/" + fresh + "/context/objects"
	rec = r.do(ctx, http.MethodPost, freshBase, pushBody(t, append(siteObjects, otherObjects...)))
	require.Equal(t, http.StatusConflict, rec.Code, "one push carries one project")

	checkout := newDesktop(t, workspace.KeyForCheckout("/fakehome/src/site"))
	checkout.observe(t, "use", "utilise")
	rec = r.do(ctx, http.MethodPost, freshBase, pushBody(t, checkout.push(t)))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextCheckoutKeyCode, refusal.Code)
	assert.Equal(t, "the push carries project key "+string(checkout.key)+
		", which belongs to one checkout because kapi.yaml states neither id: nor name:", refusal.Error)
	assert.Equal(t, "Name the project in kapi.yaml, then push again.", refusal.Message)
	assert.Contains(t, refusal.Error, string(checkout.key))
	key, err := cs.ContextKey(ctx, fresh)
	require.NoError(t, err)
	assert.Empty(t, key, "a refused push binds no key")

	ledger, err := srv.ContextLedger(ctx, fresh, testMember("member-a"))
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Kind:    contextop.KindObserve,
		Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "a note"},
	})
	require.ErrorIs(t, err, ErrNoContextKey)
}

// The log is read once and kept: a second reading fetches no segment, and a
// push of a new segment makes the next reading fetch that segment alone.
func TestContextLedger_CacheFollowsTheObjectList_SQLite(t *testing.T) {
	ctx := t.Context()
	srv, e, cs := ledgerSQLiteServer(t)
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	d.observe(t, "use", "utilise")
	require.NoError(t, r.Put(ctx, d.push(t)...))

	ledger, err := srv.ContextLedger(ctx, pid, testMember("member-a"))
	require.NoError(t, err)
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, int64(1), cs.gets.Load())

	_, err = ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	again, err := srv.ContextLedger(ctx, pid, testMember("member-b"))
	require.NoError(t, err)
	_, err = again.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), cs.gets.Load(), "an unchanged object list reads no segment")

	d.observe(t, "sign in", "login")
	require.NoError(t, r.Put(ctx, d.push(t)...))
	records, err = ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	assert.Len(t, records, 2, "the pushed segment is read")
	assert.Equal(t, int64(2), cs.gets.Load(), "only the new segment is fetched")
}

// An object under log/ that is not a segment of the project's operations is
// refused, whoever pushes it, and binds no key: a desktop could never pull
// past it, since an object is never replaced.
func TestContextLedger_SegmentThatDoesNotDecodeIsRefused_SQLite(t *testing.T) {
	ctx := t.Context()
	_, e, cs := ledgerSQLiteServer(t)
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	junk := workspace.Object{Name: workspace.SegmentName("w9", "000000000000000000000001"), Data: []byte("not a segment\n")}
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, []workspace.Object{junk}))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextInvalidSegmentCode, refusal.Code)
	assert.Equal(t, junk.Name, refusal.Details)

	d := newDesktop(t, "site")
	d.observe(t, "use", "utilise")
	good := d.push(t)
	rec = r.do(ctx, http.MethodPost, r.base, pushBody(t, append(good, junk)))
	require.Equal(t, http.StatusBadRequest, rec.Code, "one bad segment refuses the whole push")
	names, err := cs.ListContextObjects(ctx, pid, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Empty(t, names)
	key, err := cs.ContextKey(ctx, pid)
	require.NoError(t, err)
	assert.Empty(t, key)

	// A segment of the bound key carrying a line of another project.
	require.NoError(t, r.Put(ctx, good...))
	mixed := append([]byte{}, workspacetest.Segment("site", "000000000000000000000003", "a")...)
	mixed = append(mixed, workspacetest.Segment("other", "000000000000000000000004", "b")...)
	rec = r.do(ctx, http.MethodPost, r.base, pushBody(t, []workspace.Object{{Name: workspace.SegmentName("w9", "000000000000000000000003"), Data: mixed}}))
	assert.Equal(t, http.StatusConflict, rec.Code, "a segment naming two projects is refused")
}

// The same claims on the full server, behind its real auth middleware and
// over PostgreSQL: the keep carries the member's display name.
func TestContextLedger_Postgres(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	createProject := func(t *testing.T) string {
		p := &platstore.Project{
			Name: "ctx", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
			WorkspaceID: "test-ws", Properties: map[string]string{},
		}
		require.NoError(t, srv.ContentStore.CreateProject(t.Context(), p))
		return p.ID
	}

	t.Run("round trip", func(t *testing.T) {
		pid := createProject(t)
		r := &routeRemote{h: e, base: "/api/v1/test/" + pid + "/context/objects", bearer: token}
		ledgerRoundTrip(t, srv, r, pid, "test-user", "Test")
	})

	t.Run("refusals", func(t *testing.T) {
		ctx := t.Context()
		pid := createProject(t)
		r := &routeRemote{h: e, base: "/api/v1/test/" + pid + "/context/objects", bearer: token}
		site := newDesktop(t, "site")
		site.observe(t, "use", "utilise")
		require.NoError(t, r.Put(ctx, site.push(t)...))

		other := newDesktop(t, "other")
		other.observe(t, "sign in", "login")
		rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, other.push(t)))
		require.Equal(t, http.StatusConflict, rec.Code)
		var refusal ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
		assert.Equal(t, "this project's context belongs to project key site, and the push carries project key other", refusal.Error)

		checkout := newDesktop(t, workspace.KeyForCheckout("/fakehome/src/site"))
		checkout.observe(t, "use", "utilise")
		freshBase := "/api/v1/test/" + createProject(t) + "/context/objects"
		rec = r.do(ctx, http.MethodPost, freshBase, pushBody(t, checkout.push(t)))
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
		assert.Equal(t, contextCheckoutKeyCode, refusal.Code)
	})
}

// testMember is a member with the permission to govern the whole project.
func testMember(userID string) LedgerMember {
	return LedgerMember{UserID: userID, Permissions: platauth.PermManageTerms}
}

// observeAt records a term rule suggestion seen at a point.
func (d *desktop) observeAt(t *testing.T, term, instead string, point map[string]string) contextop.Record {
	t.Helper()
	r, err := contextop.NewLedger(d.ws, nil).Append(t.Context(), contextop.Record{
		Project: d.key,
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"},
		Kind:    contextop.KindObserve,
		Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: instead, Replacement: term}},
		Scope:   contextop.Scope{Coordinates: point},
	})
	require.NoError(t, err)
	return r
}

// keep records a person keeping a suggestion.
func (d *desktop) keep(t *testing.T, id string) {
	t.Helper()
	_, err := contextop.NewLedger(d.ws, nil).Append(t.Context(), contextop.Record{
		Project: d.key,
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"},
		Kind:    contextop.KindKeep,
		Target:  id,
	})
	require.NoError(t, err)
}

// A custodian whose grants name brand=acme pushes the decisions at acme and
// the suggestions anywhere, and a decision at another brand is refused. The
// same holds for a keep recorded through the server, and a member without
// the permission to govern keeps nothing.
func TestContextLedger_DecisionsStayInReach_SQLite(t *testing.T) {
	ctx := t.Context()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	acme := platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"})
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	e := echo.New()
	srv.registerContextObjectRoutes(e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", "custodian")
			c.Set("project_permissions", platauth.PermViewContent|contextObjectWritePermission)
			c.Set("project_coordinates", acme)
			return next(c)
		}
	}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}

	d := newDesktop(t, "site")
	atAcme := d.observeAt(t, "use", "utilise", map[string]string{"brand": "acme"})
	d.keep(t, atAcme.ID)
	atOther := d.observeAt(t, "sign in", "login", map[string]string{"brand": "other"})
	require.NoError(t, r.Put(ctx, d.push(t)...), "a decision at acme and a suggestion at other")

	d.keep(t, atOther.ID)
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, d.push(t)))
	require.Equal(t, http.StatusForbidden, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextOutsideReachCode, refusal.Code)
	assert.Contains(t, refusal.Error, "governs brand=other, outside the coordinates you hold (brand=acme)")

	ledger, err := srv.ContextLedger(ctx, pid, LedgerMember{UserID: "custodian", Permissions: platauth.PermManageTerms, Reach: acme})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{Kind: contextop.KindKeep, Target: atOther.ID})
	require.ErrorIs(t, err, ErrOutsideReach)
	_, err = ledger.Append(ctx, contextop.Record{Kind: contextop.KindDrop, Target: atAcme.ID})
	require.NoError(t, err, "a decision at acme is the custodian's")

	reviewer, err := srv.ContextLedger(ctx, pid, LedgerMember{UserID: "reviewer", Permissions: platauth.PermReview})
	require.NoError(t, err)
	_, err = reviewer.Append(ctx, contextop.Record{Kind: contextop.KindKeep, Target: atOther.ID})
	require.ErrorIs(t, err, ErrOutsideReach)
	assert.Contains(t, err.Error(), "needs the")
}

// reachSQLiteServer serves the context object routes as a custodian holding
// PermManageTerms over the reach given.
func reachSQLiteServer(t *testing.T, reach platauth.CoordinateReach) (*echo.Echo, *sqlitestore.SQLiteStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	e := echo.New()
	srv.registerContextObjectRoutes(e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", "custodian")
			c.Set("project_permissions", platauth.PermViewContent|contextObjectWritePermission)
			c.Set("project_coordinates", reach)
			return next(c)
		}
	}))
	return e, cs
}

// A custodian bounded to one brand cannot push a decision about a rule that
// holds across the whole project.
func TestContextLedger_ProjectWideDecisionNeedsAnUnboundedReach_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := reachSQLiteServer(t, platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}

	d := newDesktop(t, "site")
	everywhere := d.observe(t, "use", "utilise")
	d.keep(t, everywhere.ID)
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, d.push(t)))
	require.Equal(t, http.StatusForbidden, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextOutsideReachCode, refusal.Code)
	assert.Contains(t, refusal.Error, "governs the whole project, outside the coordinates you hold (brand=acme)")
	names, err := cs.ListContextObjects(ctx, pid, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Empty(t, names, "a refused push writes nothing")
}

// A decision about an operation the project's context does not hold is
// refused, whatever the pusher's reach: its point cannot be read.
func TestContextLedger_DecisionOnAnUnknownOperation_SQLite(t *testing.T) {
	ctx := t.Context()
	keepOf := func(t *testing.T, target string) []workspace.Object {
		t.Helper()
		at := time.Now().UTC()
		op := workspace.Op{
			ID: workspace.NewOpID(at, ""), Project: "site", Kind: contextop.OpKindPrefix + string(contextop.KindKeep),
			Payload: []byte(`{"actor":{"kind":"person","name":"asgeir"},"target":"` + target + `"}`), At: at,
		}
		data, err := workspace.EncodeSegment([]workspace.Op{op})
		require.NoError(t, err)
		return []workspace.Object{{Name: workspace.SegmentName("w1", op.ID), Data: data}}
	}
	unknown := workspace.NewOpID(time.Now().Add(-time.Hour), "")

	e, cs := reachSQLiteServer(t, platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, keepOf(t, unknown)))
	require.Equal(t, http.StatusForbidden, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Contains(t, refusal.Error, "which the project's context does not hold")

	unbounded := LedgerMember{UserID: "owner", Permissions: platauth.PermManageTerms}
	keep := keepOf(t, unknown)
	ops, err := workspace.DecodeSegment(keep[0].Data, "site")
	require.NoError(t, err)
	require.ErrorIs(t, checkReach(unbounded, ops[0], map[string]workspace.Op{}), ErrOutsideReach)

	noPermission := LedgerMember{UserID: "viewer", Permissions: platauth.PermViewContent}
	err = checkReach(noPermission, ops[0], map[string]workspace.Op{unknown: {ID: unknown}})
	var reachErr *ReachError
	require.ErrorAs(t, err, &reachErr)
	assert.Equal(t, platauth.PermManageTerms, reachErr.Permission, "a full reach without the permission governs nothing")
}

// heldOp returns the operation a desktop holds under an id.
func (d *desktop) heldOp(t *testing.T, id string) workspace.Op {
	t.Helper()
	ops, err := d.ws.Select(t.Context(), workspace.OpQuery{Project: d.key})
	require.NoError(t, err)
	for _, op := range ops {
		if op.ID == id {
			return op
		}
	}
	t.Fatalf("the desktop holds no operation %s", id)
	return workspace.Op{}
}

// withPayload returns op with the payload fields given set over its own.
func withPayload(t *testing.T, op workspace.Op, fields map[string]any) workspace.Op {
	t.Helper()
	body := map[string]any{}
	require.NoError(t, json.Unmarshal(op.Payload, &body))
	maps.Copy(body, fields)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	op.Payload = raw
	return op
}

// segmentOf encodes ops as one segment object under a writer.
func segmentOf(t *testing.T, writer string, ops ...workspace.Op) workspace.Object {
	t.Helper()
	data, err := workspace.EncodeSegment(ops)
	require.NoError(t, err)
	return workspace.Object{Name: workspace.SegmentName(writer, ops[0].ID), Data: data}
}

// A bounded custodian cannot move a held suggestion into their reach by
// pushing a copy of it under its id with other coordinates, and a keep riding
// on the copy is refused with it.
func TestContextLedger_ForgedCopyOfAHeldOperationIsRefused_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := reachSQLiteServer(t, platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	atOther := d.observeAt(t, "sign in", "login", map[string]string{"brand": "other"})
	require.NoError(t, r.Put(ctx, d.push(t)...))

	forged := withPayload(t, d.heldOp(t, atOther.ID), map[string]any{"scope": map[string]any{"coordinates": map[string]string{"brand": "acme"}}})
	at := time.Now().UTC()
	keep := workspace.Op{
		ID: workspace.NewOpID(at, atOther.ID), Project: "site", Kind: contextop.OpKindPrefix + string(contextop.KindKeep),
		Payload: []byte(`{"actor":{"kind":"person","name":"custodian"},"target":"` + atOther.ID + `"}`), At: at,
	}
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, []workspace.Object{segmentOf(t, "forger", forged, keep)}))
	require.Equal(t, http.StatusConflict, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextOperationConflictCode, refusal.Code)
	assert.Equal(t, atOther.ID, refusal.Details)

	names, err := cs.ListContextObjects(ctx, pid, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Len(t, names, 1, "nothing of the forged push is written")
}

// Where two segments hold one id, the operation kept is the one in the
// segment whose name sorts first, on every reading.
func TestContextLedger_MergeKeepsOneOperationPerIDInNameOrder(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := workspace.NewOpID(at, "")
	op := func(text string) workspace.Op {
		return workspace.Op{ID: id, Project: "site", Kind: "context.observe", Payload: []byte(`{"note":"` + text + `"}`), At: at}
	}
	segments := map[string]readSegment{
		workspace.SegmentName("zz", id): {ops: []workspace.Op{op("last")}},
		workspace.SegmentName("aa", id): {ops: []workspace.Op{op("first")}},
		workspace.SegmentName("mm", id): {ops: []workspace.Op{op("middle")}},
	}
	for range 50 {
		merged := mergeSegments(segments)
		require.Len(t, merged, 1)
		require.JSONEq(t, `{"note":"first"}`, string(merged[0].Payload))
	}
}

// A push refused for reach binds no key: the next push may still name the
// project.
func TestContextLedger_RefusedPushBindsNoKey_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := reachSQLiteServer(t, platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	atOther := d.observeAt(t, "sign in", "login", map[string]string{"brand": "other"})
	d.keep(t, atOther.ID)
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, d.push(t)))
	require.Equal(t, http.StatusForbidden, rec.Code)
	key, err := cs.ContextKey(ctx, pid)
	require.NoError(t, err)
	assert.Empty(t, key, "a refused push leaves the project unbound")
}

// An establishment settling derives from the log is accepted at any point,
// and keeps the tool actor it names; one whose address its evidence does not
// derive is checked as any decision is.
func TestContextLedger_SettlingFollowsFromTheLog_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := reachSQLiteServer(t, platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	ledger := contextop.NewLedger(d.ws, nil)
	s, err := ledger.Append(ctx, contextop.Record{
		Project: "site", Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s1"},
		Kind: contextop.KindObserve, Scope: contextop.Scope{Coordinates: map[string]string{"brand": "other"}},
		Subject:  contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: "Quick cast", Replacement: "Quickcast"}},
		Evidence: []contextop.Evidence{{Path: "docs/guide.md"}},
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "site", Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "settle-merged"}, Kind: contextop.KindSignal,
		Target: s.ID, Signal: &contextop.Signal{Source: contextop.SignalMerge, Commit: "abc1234def", PR: 412, Preferred: 2},
	})
	require.NoError(t, err)
	settled, err := ledger.Settle(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.NoError(t, r.Put(ctx, d.push(t)...), "a settling at brand=other follows from the log the push carries")

	srvLedger, err := reachServerLedger(t, cs, pid)
	require.NoError(t, err)
	records, err := srvLedger.Records(ctx, contextop.Filter{Kinds: []contextop.Kind{contextop.KindEstablish}})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, contextop.SettleActor, records[0].Actor, "the settling keeps its tool actor")

	// An establishment naming evidence it was not derived from.
	at := time.Now().UTC()
	forged := workspace.Op{
		ID: workspace.NewOpID(at, records[0].ID), Project: "site", Kind: contextop.OpKindPrefix + string(contextop.KindEstablish),
		Address: "context.establish:00000000000000000000000000000000",
		Payload: []byte(`{"actor":{"kind":"tool","name":"settle"},"target":"` + s.ID + `","because":["` + records[0].Because[0] + `"]}`), At: at,
	}
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, []workspace.Object{segmentOf(t, "forger", forged)}))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// reachServerLedger opens the server ledger of a project for an unbounded
// custodian.
func reachServerLedger(t *testing.T, cs *sqlitestore.SQLiteStore, pid string) (*contextop.Ledger, error) {
	t.Helper()
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	return srv.ContextLedger(t.Context(), pid, testMember("owner"))
}

// Every operation recorded through a member's ledger carries the member,
// whatever actor the caller names; the settler records under the settling
// tool, which the server sets.
func TestContextLedger_RecordStampsTheMember_SQLite(t *testing.T) {
	ctx := t.Context()
	srv, e, cs := ledgerSQLiteServer(t)
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	s := d.observe(t, "use", "utilise")
	require.NoError(t, r.Put(ctx, d.push(t)...))

	ledger, err := srv.ContextLedger(ctx, pid, testMember("member-a"))
	require.NoError(t, err)
	member := contextop.Actor{Kind: contextop.ActorPerson, Name: "member-a"}
	signal, err := ledger.Append(ctx, contextop.Record{
		Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "settle-merged"}, Kind: contextop.KindSignal,
		Target: s.ID, Signal: &contextop.Signal{Source: contextop.SignalMerge, Commit: "abc1234def", PR: 7, Preferred: 1},
	})
	require.NoError(t, err)
	claimed, err := ledger.Append(ctx, contextop.Record{
		Actor: contextop.SettleActor, Kind: contextop.KindEstablish, Target: s.ID, Because: []string{signal.ID},
	})
	require.NoError(t, err)
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	byID := map[string]contextop.Record{}
	for _, rec := range records {
		byID[rec.ID] = rec
	}
	assert.Equal(t, member, byID[signal.ID].Actor, "a claimed tool actor is stamped over")
	assert.Equal(t, member, byID[claimed.ID].Actor)

	other := d.observe(t, "sign in", "login")
	_, err = contextop.NewLedger(d.ws, nil).Append(ctx, contextop.Record{
		Project: "site", Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "settle-merged"}, Kind: contextop.KindSignal,
		Target: other.ID, Signal: &contextop.Signal{Source: contextop.SignalMerge, Commit: "fed4321cba", PR: 8, Preferred: 1},
	})
	require.NoError(t, err)
	require.NoError(t, r.Put(ctx, d.push(t)...))
	settler, err := srv.ContextSettler(pid)
	require.NoError(t, err)
	settled, err := settler.Settle(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.NotEmpty(t, settled)
	established, err := settler.Records(ctx, contextop.Filter{Kinds: []contextop.Kind{contextop.KindEstablish}})
	require.NoError(t, err)
	var bySettler int
	for _, rec := range established {
		if rec.Actor == contextop.SettleActor {
			bySettler++
		}
	}
	assert.Positive(t, bySettler, "the settler records under the settling tool")
}

// An establishment that settling would not record is refused for a bounded
// custodian, whatever address it claims: the address is the one its evidence
// derives, but it names a scope settling does not give it.
func TestContextLedger_ForgedEstablishIsRefused_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := reachSQLiteServer(t, platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"brand": "acme"}))
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	atOther := d.observeAt(t, "sign in", "login", map[string]string{"brand": "other"})
	require.NoError(t, r.Put(ctx, d.push(t)...))

	// The evidence is a keep's worth of nothing: an observation at another
	// brand, which settling establishes on no signal at all.
	at := time.Now().UTC()
	because := []string{atOther.ID}
	forged := workspace.Op{
		ID: workspace.NewOpID(at, atOther.ID), Project: "site", Kind: contextop.OpKindPrefix + string(contextop.KindEstablish),
		Address: contextop.EstablishAddress(atOther.ID, because),
		Payload: []byte(`{"actor":{"kind":"tool","name":"settle"},"target":"` + atOther.ID + `","because":["` + atOther.ID + `"],"scope":{"level":"project","coordinates":{"brand":"acme"}}}`),
		At:      at,
	}
	rec := r.do(ctx, http.MethodPost, r.base, pushBody(t, []workspace.Object{segmentOf(t, "forger", forged)}))
	require.Equal(t, http.StatusForbidden, rec.Code)
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, contextOutsideReachCode, refusal.Code)
}

// A keep that declares a subject of its own is governed by what it keeps: a
// note subject on a term rule needs the terms permission, so a member holding
// the voice permission alone keeps nothing, through the server or a push.
func TestContextLedger_KeepIsGovernedByItsTarget_SQLite(t *testing.T) {
	ctx := t.Context()
	srv, e, cs := ledgerSQLiteServer(t)
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	term := d.observe(t, "use", "utilise")
	require.NoError(t, r.Put(ctx, d.push(t)...))

	voiceOnly, err := srv.ContextLedger(ctx, pid, LedgerMember{UserID: "voice", Permissions: platauth.PermManageVoice})
	require.NoError(t, err)
	_, err = voiceOnly.Append(ctx, contextop.Record{
		Kind: contextop.KindKeep, Target: term.ID,
		Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "a note, not a rule"},
	})
	require.ErrorIs(t, err, ErrOutsideReach)

	at := time.Now().UTC()
	keep := workspace.Op{
		ID: workspace.NewOpID(at, term.ID), Project: "site", Kind: contextop.OpKindPrefix + string(contextop.KindKeep),
		Payload: []byte(`{"actor":{"kind":"person","name":"voice"},"target":"` + term.ID + `","subject":{"kind":"note","text":"x"}}`), At: at,
	}
	voiceEcho := echo.New()
	(&Server{ContentStore: cs, wsStores: newWorkspaceStores()}).registerContextObjectRoutes(voiceEcho.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", "voice")
			c.Set("project_permissions", platauth.PermViewContent|platauth.PermManageVoice)
			return next(c)
		}
	}))
	voicePush := &routeRemote{h: voiceEcho, base: r.base}
	rec := voicePush.do(ctx, http.MethodPost, r.base, pushBody(t, []workspace.Object{segmentOf(t, "voice", keep)}))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
