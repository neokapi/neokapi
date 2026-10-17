package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenProjectAutoOpensMemory(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	// Scaffold a sample project into a temp directory.
	require.NoError(t, sample.Scaffold("kapimart", dir))

	// Open the project — the seeded content memory and terms of its own store
	// should be adopted as tab handles.
	tab, err := app.OpenProject(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	app.mu.RLock()
	op := app.projects[tab.ID]
	app.mu.RUnlock()
	require.NotNil(t, op)

	assert.NotEmpty(t, op.memoryHandle, "project-scoped content memory should be auto-opened")
	assert.NotEmpty(t, op.tbHandle, "project-scoped terms should be auto-opened")

	// Handles should be valid.
	tm, ok := app.memoryHandles.Get(op.memoryHandle)
	assert.True(t, ok)
	memoryCount, err := tm.Count(t.Context())
	require.NoError(t, err)
	assert.Greater(t, memoryCount, 0)

	tb, ok := app.tbHandles.Get(op.tbHandle)
	assert.True(t, ok)
	tbCount, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Greater(t, tbCount, 0)
}

func TestGetProjectHandles(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))

	tab, err := app.OpenProject(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	h := app.GetProjectHandles(tab.ID)
	assert.Equal(t, tab.ID, h.TabID)
	assert.NotEmpty(t, h.MemoryHandle, "project content-memory handle should be reachable")
	assert.NotEmpty(t, h.TermsHandle, "project terms store handle should be reachable")

	// The bundled ids match the single-getter accessors and resolve to live
	// handles the frontend can preselect.
	assert.Equal(t, app.GetProjectMemoryHandle(tab.ID), h.MemoryHandle)
	assert.Equal(t, app.GetProjectTermsHandle(tab.ID), h.TermsHandle)
	_, ok := app.memoryHandles.Get(h.MemoryHandle)
	assert.True(t, ok)
	_, ok = app.tbHandles.Get(h.TermsHandle)
	assert.True(t, ok)
}

func TestGetProjectHandlesUnknownTab(t *testing.T) {
	app := NewApp()
	h := app.GetProjectHandles("nope")
	assert.Equal(t, "nope", h.TabID)
	assert.Empty(t, h.MemoryHandle)
	assert.Empty(t, h.TermsHandle)
}

// The frontend's ProjectHandles type (src/types/api.ts) is hand-written, so no
// typecheck relates it to this struct: MemoriesPage reads `memoryHandle` off the
// decoded response, and when these tags still said `tmHandle`/`termbaseHandle`
// it silently read undefined and showed "no project content memory found" for
// every project. Assert the wire names, not just the Go field values.
func TestProjectHandlesJSONFieldNames(t *testing.T) {
	b, err := json.Marshal(ProjectHandles{TabID: "t", MemoryHandle: "m", TermsHandle: "s"})
	require.NoError(t, err)

	var wire map[string]string
	require.NoError(t, json.Unmarshal(b, &wire))

	assert.Equal(t, map[string]string{
		"tabID":        "t",
		"memoryHandle": "m",
		"termsHandle":  "s",
	}, wire, "wire field names must match the frontend ProjectHandles type")
}

func TestOpenProjectNoAutoOpenWithoutDotKapi(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "plain")

	app.mu.RLock()
	op := app.projects[tab.ID]
	app.mu.RUnlock()

	assert.Empty(t, op.memoryHandle, "no content-memory handle when the project has no store")
	assert.Empty(t, op.tbHandle, "no terms handle when the project has no store")
}

func TestCloseProjectClosesHandles(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))

	tab, err := app.OpenProject(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)

	app.mu.RLock()
	memoryHandle := app.projects[tab.ID].memoryHandle
	tbHandle := app.projects[tab.ID].tbHandle
	app.mu.RUnlock()

	app.CloseProject(tab.ID)

	// Handles should be closed.
	_, ok := app.memoryHandles.Get(memoryHandle)
	assert.False(t, ok, "content-memory handle should be closed")
	_, ok = app.tbHandles.Get(tbHandle)
	assert.False(t, ok, "terms handle should be closed")
}

func TestCreateSampleProjectIdempotent(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	// Scaffold manually first to simulate existing project.
	require.NoError(t, sample.Scaffold("kapimart", dir))

	// Opening the same project twice should return the same tab.
	kapiPath := filepath.Join(dir, "kapi.yaml")
	tab1, err := app.OpenProject(kapiPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab1.ID) })
	tab2, err := app.OpenProject(kapiPath)
	require.NoError(t, err)
	assert.Equal(t, tab1.ID, tab2.ID)
}

func TestCreateSampleProjectInvalidName(t *testing.T) {
	app := NewApp()
	_, err := app.CreateSampleProject("nonexistent")
	assert.Error(t, err)
}

// A sample left on disk by an older app version may carry a recipe that no
// longer parses (v1 schema, legacy top-level languages, list-form `plugins:`).
// Opening it must recover by re-scaffolding rather than failing (issue #4
// follow-up).
func TestCreateSampleProjectRecoversStaleRecipe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAPI_HOME_DIR", home)

	targetDir := filepath.Join(home, "KapiProjects", sample.DisplayName["kapimart"])
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	// Legacy schema: v1 version, top-level languages, `content:` in place of
	// `collections:`, and list-form plugins (the exact shape that triggered
	// "cannot unmarshal !!seq into map[string]project.PluginSpec").
	stale := "version: v1\nname: KapiMart\nsource_language: en-US\ntarget_languages:\n  - fr-FR\nplugins:\n  - okapi-bridge\ncontent:\n  - path: \"input/*.json\"\n    format: okf_json\n"
	kapiPath := filepath.Join(targetDir, "kapi.yaml")
	require.NoError(t, os.WriteFile(kapiPath, []byte(stale), 0o644))

	// Plant a stale/corrupt store: one with an incompatible schema would break
	// re-seeding ("apply migration N: no such table ..."). Recovery must delete it
	// so Scaffold reseeds cleanly. A non-DB file reproduces the class.
	staleKapi := filepath.Join(targetDir, project.StateDirName)
	require.NoError(t, os.MkdirAll(staleKapi, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleKapi, project.StoreFileName), []byte("not a database"), 0o644))

	// The stale recipe must not parse...
	_, err := project.Load(kapiPath)
	require.Error(t, err, "precondition: stale recipe should fail to parse")

	app := NewApp()
	tab, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err, "CreateSampleProject should recover a stale sample recipe")
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// ...and after recovery the on-disk recipe parses cleanly.
	_, err = project.Load(kapiPath)
	require.NoError(t, err, "recipe should parse after re-scaffold")
}

func TestSampleProjectFilesExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))

	// Verify all expected files.
	for _, path := range []string{
		"kapi.yaml",
		"src/en/store-ui.json",
		"web/en/getting-started.md",
		".kapi/work/store.db",
	} {
		_, err := os.Stat(filepath.Join(dir, path))
		assert.NoError(t, err, "missing: %s", path)
	}
}

// Scaffolding stamps a sample manifest; an older on-disk revision is reported as
// upgradable, Reset backs up + re-scaffolds to the current revision, and
// Acknowledge clears the prompt without re-scaffolding.
func TestSampleUpgradeFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAPI_HOME_DIR", home)
	dir := filepath.Join(home, "KapiProjects", sample.DisplayName["kapimart"])

	app := NewApp()
	tab, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// Freshly scaffolded → current revision, no upgrade.
	info := app.GetSampleInfo(tab.ID)
	require.True(t, info.IsSample)
	assert.Equal(t, "kapimart", info.Name)
	assert.Equal(t, sample.CurrentRevision("kapimart"), info.CurrentRevision)
	assert.False(t, info.UpgradeAvailable)

	// Simulate a copy left by an older kapi (revision 1).
	require.NoError(t, sample.SetManifestRevision(dir, 1))
	info = app.GetSampleInfo(tab.ID)
	assert.True(t, info.UpgradeAvailable)
	assert.Equal(t, 1, info.OnDiskRevision)

	// Reset → backs up the old dir and re-scaffolds at the current revision.
	newTab, err := app.ResetSampleProject(tab.ID)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(newTab.ID) })
	_, statErr := os.Stat(dir + " (backup r1)")
	require.NoError(t, statErr, "old copy should be backed up")
	info = app.GetSampleInfo(newTab.ID)
	assert.False(t, info.UpgradeAvailable)
	assert.Equal(t, sample.CurrentRevision("kapimart"), info.OnDiskRevision)

	// Acknowledge clears the prompt without re-scaffolding.
	require.NoError(t, sample.SetManifestRevision(dir, 1))
	require.True(t, app.GetSampleInfo(newTab.ID).UpgradeAvailable)
	require.NoError(t, app.AcknowledgeSampleRevision(newTab.ID))
	assert.False(t, app.GetSampleInfo(newTab.ID).UpgradeAvailable)
}

// Reset reloads the OPEN tab in place — same tab ID, fresh recipe, reopened
// project-scoped handles — so surfaces polling the tab (the home hero's
// convergence plan/report) keep working immediately after the reset instead of
// ENOENT-ing against the backed-up directory path.
func TestResetSampleProject_TabStaysUsable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAPI_HOME_DIR", home)

	app := NewApp()
	tab, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	newTab, err := app.ResetSampleProject(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, tab.ID, newTab.ID, "the tab reloads in place — no close/reopen churn")

	// The tab entry is rewired to the fresh scaffold: recipe reloaded, content memory and
	// terms handles live again, and the derivation bindings answer without
	// a friendly-error detour.
	app.mu.RLock()
	op := app.projects[tab.ID]
	app.mu.RUnlock()
	require.NotNil(t, op)
	assert.NotNil(t, op.Project)
	assert.NotEmpty(t, op.memoryHandle, "project content memory reopened after reset")
	assert.NotEmpty(t, op.tbHandle, "project terms store reopened after reset")

	_, err = app.GetConvergePlan(tab.ID)
	require.NoError(t, err, "plan must be derivable immediately after a reset")
	_, err = app.GetConvergence(tab.ID)
	require.NoError(t, err, "convergence must be derivable immediately after a reset")
}

// TestCreateSampleProject_TheOsloCampaignSitsInOslo: the campaign's pages sit
// at Oslo, the pick-up page at Grünerløkka inside it, and a rule kept for
// Norway reaches both, because Oslo sits within the county of Oslo, which
// sits within Norway.
func TestCreateSampleProject_TheOsloCampaignSitsInOslo(t *testing.T) {
	t.Setenv("KAPI_HOME_DIR", t.TempDir())
	t.Setenv(host.EnvDataDir, t.TempDir())
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(app.shutdownEngine)

	tab, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	proj, err := project.Load(tab.Path)
	require.NoError(t, err)
	at := func(rel string) map[string]string {
		t.Helper()
		pt := project.GovernancePoint{Path: rel}
		rc, err := proj.ResolveGovernanceFor(pt)
		require.NoError(t, err)
		return proj.CoordinatesAt(pt, rc)
	}
	page := at(sample.OsloCampaignPage)
	assert.Equal(t, "oslo", page["market"])
	assert.Equal(t, "13-17", page["age"])
	assert.Equal(t, "marketing", page[project.ChannelAxis])
	pickup := at("campaigns/oslo/en/grunerlokka.md")
	assert.Equal(t, "grunerlokka", pickup["market"], "the pick-up page's item refines the campaign's point")
	assert.Equal(t, "13-17", pickup["age"])

	g := proj.Gazetteer()
	assert.Equal(t, []string{"world", "europe", "northern-europe", "no", "no-03", "oslo", "grunerlokka"}, g.Path("grunerlokka"))
	assert.True(t, proj.AxisSpace().Contains("market", "nordics", "oslo"))

	// A rule scoped to Norway, as a person keeps one for the whole market,
	// holds at both pages and not at a page written for the world.
	space := proj.AxisSpace()
	norway := contextop.Scope{Coordinates: map[string]string{"market": "no"}}
	assert.True(t, norway.CoversIn(space, page), "a rule for Norway holds at Oslo")
	assert.True(t, norway.CoversIn(space, pickup), "and at Grünerløkka")
	assert.False(t, norway.CoversIn(space, at("web/en/faq.md")), "and not at the default market, the world")
	assert.True(t, contextop.Scope{Coordinates: map[string]string{"market": "nordics", "age": "13-"}}.CoversIn(space, pickup),
		"a rule for the Nordics and readers from 13 holds there too")

	// A suggestion seen on the campaign's page reaches the pick-up page, which
	// sits inside the campaign's Oslo, and stays out of the website.
	engine := app.hostEngine()
	_, err = engine.RecordContextObservation(t.Context(), host.ContextObserveRequest{
		Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "oslo-test"}, Project: tab.Path,
		Text:     "The Oslo campaign names the pick-up point by its street.",
		Evidence: []contextop.Evidence{{Path: sample.OsloCampaignPage, Quote: "collect your order from any KapiMart pick-up point"}},
	})
	require.NoError(t, err)
	view, err := app.AgentContextAt(tab.ID, "campaigns/oslo/en/grunerlokka.md", 0)
	require.NoError(t, err)
	assert.Equal(t, "grunerlokka", view.Answer.Point.Coordinates["market"])
	assert.Contains(t, view.Text, "The Oslo campaign names the pick-up point by its street.")
	faq, err := app.AgentContextAt(tab.ID, "web/en/faq.md", 0)
	require.NoError(t, err)
	assert.NotContains(t, faq.Text, "The Oslo campaign names the pick-up point")

	atlas, err := app.ProjectContextAtlas(tab.ID)
	require.NoError(t, err)
	var ids []string
	for _, p := range atlas.Places {
		ids = append(ids, p.ID)
	}
	assert.Contains(t, ids, "oslo")
	assert.Contains(t, ids, "grunerlokka")
	assert.Contains(t, ids, "nordics")
}

// TestCreateSampleProject_ArrivesWithAnAssistantSession: the sample opens with
// the team's kept notes and the two assistant sessions recorded, the home row reads them (codex the
// latest, one disagreement, the voice the recipe binds), and opening the
// sample again records nothing more.
func TestCreateSampleProject_ArrivesWithAnAssistantSession(t *testing.T) {
	t.Setenv("KAPI_HOME_DIR", t.TempDir())
	t.Setenv(host.EnvDataDir, t.TempDir())
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(app.shutdownEngine)

	tab, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1)
	row := home.Projects[0]
	assert.Equal(t, "kapimart", row.Voice)
	require.NotNil(t, row.Sharing)
	assert.Equal(t, project.ContextBackendLocal, row.Sharing.Kind)
	require.NotNil(t, row.Context)
	assert.Equal(t, 1, row.Context.Conflicts)
	assert.Equal(t, 5, row.Context.Suggested, "three names, a word and a note")
	assert.Equal(t, 6, row.Context.Rules, "the team's six kept notes; a file read in is not a rule decided in review")
	require.NotNil(t, row.Context.Latest)
	assert.Equal(t, "codex", row.Context.Latest.Actor.Name)
	assert.Equal(t, 2, row.Context.Latest.Recorded)
	assert.Equal(t, map[string]int{host.DigestThemeNames: 1, host.DigestThemeWords: 1}, row.Context.Latest.NewByTheme)

	before, err := app.hostEngine().ContextOperations(t.Context(), host.ContextLogRequest{Project: tab.Path})
	require.NoError(t, err)
	again, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err)
	assert.Equal(t, tab.ID, again.ID)
	after, err := app.hostEngine().ContextOperations(t.Context(), host.ContextLogRequest{Project: tab.Path})
	require.NoError(t, err)
	assert.Len(t, after.Operations, len(before.Operations), "the sessions are recorded once")
}

// A sample an older app scaffolded may carry a recipe this version refuses (a
// key the schema no longer takes). Opening it from the workspace home or the
// recent list refreshes it to the current sample and keeps the old copy; a
// sample at the current revision, and a project that is not a sample, report
// the error and are left as they are.
func TestOpenProject_RefreshesAStaleSampleThatNoLongerLoads(t *testing.T) {
	t.Setenv(host.EnvDataDir, t.TempDir())
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	breakRecipe := func(t *testing.T, kapiPath string) {
		t.Helper()
		raw, err := os.ReadFile(kapiPath)
		require.NoError(t, err)
		broken := strings.Replace(string(raw), "    kind: geo\n", "    kind: geo\n    groups:\n      nordics: [no, se, dk, fi, is]\n", 1)
		require.NotEqual(t, string(raw), broken)
		require.NoError(t, os.WriteFile(kapiPath, []byte(broken), 0o644))
		_, err = project.Load(kapiPath)
		require.ErrorContains(t, err, `an axis takes no key "groups"`)
	}

	t.Run("an older sample is refreshed and backed up", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "KapiMart")
		require.NoError(t, sample.Scaffold("kapimart", dir))
		kapiPath := filepath.Join(dir, "kapi.yaml")
		breakRecipe(t, kapiPath)
		old := sample.CurrentRevision("kapimart") - 1
		require.NoError(t, sample.SetManifestRevision(dir, old))

		app := NewApp()
		tab, err := app.OpenProject(kapiPath)
		require.NoError(t, err)
		t.Cleanup(func() { app.CloseProject(tab.ID) })

		_, err = project.Load(kapiPath)
		require.NoError(t, err, "the refreshed recipe loads")
		assert.False(t, app.GetSampleInfo(tab.ID).UpgradeAvailable)
		backup, err := os.ReadFile(filepath.Join(fmt.Sprintf("%s (backup r%d)", dir, old), "kapi.yaml"))
		require.NoError(t, err, "the old copy is kept")
		assert.Contains(t, string(backup), "groups:")
	})

	t.Run("a current sample keeps its error", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "KapiMart")
		require.NoError(t, sample.Scaffold("kapimart", dir))
		kapiPath := filepath.Join(dir, "kapi.yaml")
		breakRecipe(t, kapiPath)

		_, err := NewApp().OpenProject(kapiPath)
		require.ErrorContains(t, err, `an axis takes no key "groups"`)
		raw, _ := os.ReadFile(kapiPath)
		assert.Contains(t, string(raw), "groups:", "a recipe a person edited is left as it is")
	})

	t.Run("a project that is not a sample keeps its error", func(t *testing.T) {
		kapiPath := filepath.Join(t.TempDir(), "kapi.yaml")
		require.NoError(t, os.WriteFile(kapiPath, []byte("version: v1\nname: x\naxes:\n  market:\n    kind: geo\n    groups: {nordics: [no]}\n"), 0o644))

		_, err := NewApp().OpenProject(kapiPath)
		require.ErrorContains(t, err, `an axis takes no key "groups"`)
	})
}
