package server

import (
	"net/http"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addWorkspaceWithOwner creates a fresh workspace (with the given slug) plus a
// user who owns it, returning a signed token for that owner. Used to model a
// second tenant distinct from newTestServer's "test-ws".
func addWorkspaceWithOwner(t *testing.T, s *Server, wsID, slug, userID, email string) string {
	t.Helper()
	ctx := t.Context()
	u := &platauth.User{ID: userID, Email: email, Name: email}
	require.NoError(t, s.AuthStore.CreateUser(ctx, u))
	ws := &platauth.Workspace{ID: wsID, Name: slug, Slug: slug, Type: platauth.WorkspaceTypePersonal}
	require.NoError(t, s.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, s.AuthStore.AddMember(ctx, wsID, userID, platauth.RoleOwner))
	token, err := platauth.GenerateToken(u, "test-secret", time.Hour)
	require.NoError(t, err)
	return token
}

// TestCrossTenantProjectIDOR proves that the owner/admin of one workspace cannot
// read or write a project that belongs to a DIFFERENT workspace by addressing it
// through their own workspace slug (/api/v1/<their-ws>/<victim-project-id>).
//
// Before the ProjectAccessMiddleware cross-tenant guard, WorkspaceAccessMiddleware
// only proved membership of :ws while the handlers fetched the project by ID
// globally; when the caller had no project membership, ProjectAccessMiddleware
// fell back to their OWN workspace role, granting owner/admin permissions on a
// foreign project. The guard fails closed with 404 (anti-enumeration).
func TestCrossTenantProjectIDOR(t *testing.T) {
	s, ownerToken := newTestServer(t) // test-user owns "test-ws" (slug "test")
	attackerToken := addWorkspaceWithOwner(t, s, "attacker-ws", "attacker", "attacker-user", "attacker@example.com")

	ctx := t.Context()
	// Victim project lives in the FIRST workspace (test-ws).
	require.NoError(t, s.ContentStore.CreateProject(ctx, &platstore.Project{
		ID: "victim-proj", Name: "Victim", DefaultSourceLanguage: "en", WorkspaceID: "test-ws",
	}))
	blk := &model.Block{ID: "vb", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "secret"}}}}
	require.NoError(t, s.ContentStore.StoreBlocks(ctx, "victim-proj", "main", []*model.Block{blk}))

	t.Run("read via foreign workspace slug is 404", func(t *testing.T) {
		// Attacker is owner of attacker-ws, addresses the victim project through it.
		code := do(t, s, http.MethodGet, "/api/v1/attacker/victim-proj", attackerToken, "")
		assert.Equal(t, http.StatusNotFound, code,
			"cross-tenant project read must be denied (fail-closed 404)")
	})

	t.Run("write via foreign workspace slug is 404", func(t *testing.T) {
		code := do(t, s, http.MethodPut, "/api/v1/attacker/victim-proj/blocks/main/vb", attackerToken,
			`{"target_locale":"fr","text":"pwned"}`)
		assert.Equal(t, http.StatusNotFound, code,
			"cross-tenant project write must be denied (fail-closed 404)")
	})

	t.Run("update-project via foreign workspace slug is 404", func(t *testing.T) {
		code := do(t, s, http.MethodPut, "/api/v1/attacker/victim-proj", attackerToken,
			`{"name":"hijacked","default_source_language":"en","target_languages":["fr"]}`)
		assert.Equal(t, http.StatusNotFound, code,
			"cross-tenant project update must be denied (fail-closed 404)")
	})

	t.Run("delete via foreign workspace slug is 404", func(t *testing.T) {
		code := do(t, s, http.MethodDelete, "/api/v1/attacker/victim-proj", attackerToken, "")
		assert.Equal(t, http.StatusNotFound, code,
			"cross-tenant project delete must be denied (fail-closed 404)")
	})

	// The legitimate owner still reaches the project through its own workspace.
	t.Run("legitimate owner read succeeds", func(t *testing.T) {
		code := do(t, s, http.MethodGet, "/api/v1/test/victim-proj", ownerToken, "")
		assert.Equal(t, http.StatusOK, code, "the project's own workspace owner must still read it")
	})

	t.Run("legitimate owner write succeeds", func(t *testing.T) {
		code := do(t, s, http.MethodPut, "/api/v1/test/victim-proj/blocks/main/vb", ownerToken,
			`{"target_locale":"fr","text":"bonjour"}`)
		assert.Less(t, code, 300, "the project's own workspace owner must still write it")
	})
}

// TestCrossTenantBrandProfileIDOR proves an owner/admin of one workspace cannot
// read, update, or delete another workspace's brand profile by addressing it
// through their own workspace slug. VoiceStore.GetProfile/UpdateProfile/
// DeleteProfile resolve by GLOBAL id, so the handlers assert the profile's
// WorkspaceID matches the request workspace and 404 (anti-enumeration) otherwise.
func TestCrossTenantBrandProfileIDOR(t *testing.T) {
	s, _ := newTestServer(t) // test-user owns "test-ws" (slug "test")
	attackerToken := addWorkspaceWithOwner(t, s, "attacker-ws", "attacker", "attacker-user", "attacker@example.com")

	// Wire a real brand store (its own schema on the shared container).
	db := pgtest.NewTestDB(t)
	bs, err := voicepg.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	s.VoiceStore = bs

	ctx := t.Context()
	victim := &coreprofile.VoiceProfile{ID: "victim-bp", Scope: "test-ws", Name: "Victim"}
	require.NoError(t, bs.CreateProfile(ctx, victim))

	t.Run("read via foreign workspace slug is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodGet, "/api/v1/attacker/brand-profiles/victim-bp", attackerToken, ""),
			"cross-tenant brand-profile read must be denied (fail-closed 404)")
	})
	t.Run("update via foreign workspace slug is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodPut, "/api/v1/attacker/brand-profiles/victim-bp", attackerToken, `{"name":"pwned"}`),
			"cross-tenant brand-profile update must be denied (fail-closed 404)")
	})
	t.Run("delete via foreign workspace slug is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodDelete, "/api/v1/attacker/brand-profiles/victim-bp", attackerToken, ""),
			"cross-tenant brand-profile delete must be denied (fail-closed 404)")
	})
	t.Run("check via foreign workspace slug is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodPost, "/api/v1/attacker/brand-profiles/victim-bp/check", attackerToken, `{"text":"hi"}`),
			"cross-tenant brand-profile check must be denied (fail-closed 404)")
	})

	// The victim profile is neither modified nor deleted.
	got, err := bs.GetProfile(ctx, "victim-bp")
	require.NoError(t, err, "victim profile must survive the cross-tenant attempts")
	assert.Equal(t, "Victim", got.Name, "victim profile must not have been renamed")
}

// TestCrossTenantJobIDOR proves an owner/admin of one workspace cannot read or
// cancel another workspace's job by addressing it through their own workspace
// slug. GetJob resolves by GLOBAL id (unlike ListJobs); the handlers compare the
// job's WorkspaceSlug to the request :ws and 404 on mismatch.
func TestCrossTenantJobIDOR(t *testing.T) {
	s, _ := newTestServer(t) // test-user owns "test-ws" (slug "test")
	attackerToken := addWorkspaceWithOwner(t, s, "attacker-ws", "attacker", "attacker-user", "attacker@example.com")
	ctx := t.Context()

	require.NotNil(t, s.JobStore, "job store must be wired by initTestStores")
	victim := &jobs.TranslationJob{
		ID: "victim-job", WorkspaceSlug: "test", ProjectID: "secret-proj",
		ItemName: "secret.json", TargetLocale: "fr", Status: jobs.StatusProcessing,
	}
	require.NoError(t, s.JobStore.CreateJob(ctx, victim))

	t.Run("read via foreign workspace slug is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodGet, "/api/v1/attacker/jobs/victim-job", attackerToken, ""),
			"cross-tenant job read must be denied (fail-closed 404)")
	})
	t.Run("cancel via foreign workspace slug is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodDelete, "/api/v1/attacker/jobs/victim-job", attackerToken, ""),
			"cross-tenant job cancel must be denied (fail-closed 404)")
	})

	// The victim job was not cancelled by the foreign delete.
	got, err := s.JobStore.GetJob(ctx, "victim-job")
	require.NoError(t, err)
	assert.Equal(t, jobs.StatusProcessing, got.Status, "victim job must not have been cancelled")
}

// TestWorkspaceSiblingIDRoutesNotGuarded is the regression guard for the
// cross-tenant IDOR fix over-applying. ProjectAccessMiddleware runs for EVERY
// route under /:ws, including sibling routes whose :id is a NON-project resource
// (/:ws/jobs/:id, /:ws/brand-profiles/:id, ...). For those GetProject returns
// sql.ErrNoRows, which the guard must treat as "not a project route" and fall
// through — NOT as a cross-tenant hit. The earlier (buggy) guard 404'd every
// such request (and emitted a spurious cross_workspace_project audit), silently
// breaking a broad swath of the authenticated workspace API. These cases fail
// (404) against that bug and pass (200) with the fix.
func TestWorkspaceSiblingIDRoutesNotGuarded(t *testing.T) {
	s, ownerToken := newTestServer(t) // test-user owns "test-ws" (slug "test")
	ctx := t.Context()

	t.Run("jobs/:id reaches handler for the workspace owner", func(t *testing.T) {
		require.NotNil(t, s.JobStore, "job store must be wired by initTestStores")
		job := &jobs.TranslationJob{
			ID:            "job-sibling-1",
			WorkspaceSlug: "test",
			ProjectID:     "some-proj",
			ItemName:      "en.json",
			TargetLocale:  "fr",
			Status:        jobs.StatusQueued,
		}
		require.NoError(t, s.JobStore.CreateJob(ctx, job))

		code := do(t, s, http.MethodGet, "/api/v1/test/jobs/"+job.ID, ownerToken, "")
		assert.Equal(t, http.StatusOK, code,
			"jobs/:id must reach its handler for the owner; the guard must not 404 a non-project :id")
	})

	t.Run("brand-profiles/:id reaches handler for the workspace owner", func(t *testing.T) {
		// VoiceStore is not wired by initTestStores; attach a real one (its own
		// schema on the shared container, mirroring setupBrandLoopServer) so the
		// handler can return a profile rather than 503.
		db := pgtest.NewTestDB(t)
		bs, err := voicepg.NewPostgresVoiceStore(db)
		require.NoError(t, err)
		s.VoiceStore = bs

		profile := &coreprofile.VoiceProfile{ID: "bp-sibling-1", Scope: "test-ws", Name: "Sib"}
		require.NoError(t, bs.CreateProfile(ctx, profile))

		code := do(t, s, http.MethodGet, "/api/v1/test/brand-profiles/"+profile.ID, ownerToken, "")
		assert.Equal(t, http.StatusOK, code,
			"brand-profiles/:id must reach its handler for the owner; the guard must not 404 a non-project :id")
	})
}

// TestCrossTenantGitHubInstallationIDOR proves the owner/admin of one workspace
// cannot reach a GitHub App installation belonging to a DIFFERENT workspace by
// addressing it through their own workspace slug.
//
// One registered GitHub App serves every workspace on a shared instance, and
// the app JWT behind s.GitHubApp can mint an access token for ANY installation
// of that app. The installation id in the path is GitHub's own opaque integer
// and proves nothing on its own, so the ownership record is the whole of what
// stands between workspace A and workspace B's repositories — which is why
// every setup endpoint consults it FIRST, before the GitHub App is so much as
// considered. The guard fails closed with 404 (anti-enumeration, matching
// ProjectAccessMiddleware), so an installation another workspace owns reads
// exactly like one that has never existed.
func TestCrossTenantGitHubInstallationIDOR(t *testing.T) {
	s, ownerToken := newTestServer(t) // test-user owns "test-ws" (slug "test")
	attackerToken := addWorkspaceWithOwner(t, s, "attacker-ws", "attacker", "attacker-user", "attacker@example.com")

	ctx := t.Context()
	require.NotNil(t, s.ForgeInstallationStore, "installation store must be wired by initTestStores")

	// Installation 4242 belongs to the FIRST workspace; 7000 belongs to nobody
	// (recorded by the webhook, never claimed).
	_, err := s.ForgeInstallationStore.Claim(ctx, 4242, "test-ws")
	require.NoError(t, err)
	require.NoError(t, s.ForgeInstallationStore.Record(ctx, 7000, "acme"))

	base := "/api/v1/attacker/github/installations/"

	t.Run("listing another workspace's installation is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodGet, base+"4242/repositories", attackerToken, ""),
			"cross-tenant installation read must be denied (fail-closed 404)")
	})
	t.Run("detecting inside another workspace's installation is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodGet, base+"4242/repositories/acme/site/detect", attackerToken, ""),
			"cross-tenant repository detection must be denied (fail-closed 404)")
	})
	t.Run("binding from another workspace's installation is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodPost, base+"4242/repositories", attackerToken,
				`{"repository":"acme/site","project_id":"p1"}`),
			"cross-tenant bind must be denied (fail-closed 404)")
	})
	t.Run("an unclaimed installation is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodGet, base+"7000/repositories", attackerToken, ""),
			"an installation nobody has claimed belongs to no workspace")
	})
	t.Run("claiming another workspace's installation is 404", func(t *testing.T) {
		state, err := platauth.GenerateSetupState("attacker-ws", "test-secret", time.Hour)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound,
			do(t, s, http.MethodPost, base+"4242/claim", attackerToken, `{"state":"`+state+`"}`),
			"valid state for one's own workspace must not take another workspace's installation")
	})

	// The installation still belongs to the workspace that installed it.
	inst, err := s.ForgeInstallationStore.Get(ctx, 4242)
	require.NoError(t, err)
	assert.Equal(t, "test-ws", inst.WorkspaceID, "the owning workspace must be unchanged")

	// The legitimate owner still passes the guard: its request proceeds to the
	// next gate (no GitHub App is configured in this harness) rather than being
	// turned away as not-found. This is the over-application regression guard —
	// a gate that 404s everyone protects nothing anyone can use.
	t.Run("the owning workspace still reaches its installation", func(t *testing.T) {
		code := do(t, s, http.MethodGet,
			"/api/v1/test/github/installations/4242/repositories", ownerToken, "")
		assert.NotEqual(t, http.StatusNotFound, code,
			"the workspace that installed it must pass the ownership guard")
		assert.Equal(t, http.StatusServiceUnavailable, code,
			"no GitHub App is configured in this harness, which is the next gate")
	})
}
