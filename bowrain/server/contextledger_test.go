package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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
func (nopApplier) InstallCheckpoint(context.Context, []byte) error { return errors.New("no checkpoints") }

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

	ledger, err := srv.ContextLedger(ctx, projectID, userID)
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
	assert.True(t, strings.HasSuffix(refusal.Error, "Name the project in kapi.yaml, then push again"), refusal.Error)
	assert.Contains(t, refusal.Error, string(checkout.key))
	key, err := cs.ContextKey(ctx, fresh)
	require.NoError(t, err)
	assert.Empty(t, key, "a refused push binds no key")

	ledger, err := srv.ContextLedger(ctx, fresh, "member-a")
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

	ledger, err := srv.ContextLedger(ctx, pid, "member-a")
	require.NoError(t, err)
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, int64(1), cs.gets.Load())

	_, err = ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	again, err := srv.ContextLedger(ctx, pid, "member-b")
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

// A segment that does not decode as the project's operations is read past.
func TestContextLedger_SegmentThatDoesNotDecode_SQLite(t *testing.T) {
	ctx := t.Context()
	srv, e, cs := ledgerSQLiteServer(t)
	pid := seedContextObjectProject(t, cs)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	d := newDesktop(t, "site")
	d.observe(t, "use", "utilise")
	require.NoError(t, r.Put(ctx, d.push(t)...))
	require.NoError(t, r.Put(ctx, workspace.Object{Name: workspace.SegmentName("w9", "000000000000000000000001"), Data: []byte("not a segment\n")}))

	ledger, err := srv.ContextLedger(ctx, pid, "member-a")
	require.NoError(t, err)
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	assert.Len(t, records, 1)
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
