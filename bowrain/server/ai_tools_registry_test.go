package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platformAITools are the model-backed tools a flow step and the add-step
// picker must be able to name on the platform.
var platformAITools = []registry.ToolID{
	"translate", "review", "voice-check", "voice-infer", "term-extract",
}

func serverInfo(t *testing.T, srv *Server) InfoResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var info InfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	return info
}

// The registry the server answers /info from holds the AI tools, so the web
// and desktop flow editors can offer a translate or review step.
func TestInfoListsTheAITools(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	info := serverInfo(t, srv)

	listed := make(map[registry.ToolID]registry.ToolInfo, len(info.Tools))
	for _, tool := range info.Tools {
		listed[tool.Name] = tool
	}
	for _, name := range platformAITools {
		tool, ok := listed[name]
		if !assert.True(t, ok, "/info does not list %q", name) {
			continue
		}
		assert.True(t, tool.HasSchema, "%q is listed without a schema", name)
		assert.NotEmpty(t, tool.Description, "%q carries no description", name)
	}
}

// Every AI tool answers on the schema route, so a step's options form renders
// from the tool's own document rather than from a hand-written list.
func TestSchemaRouteServesTheAITools(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))

	for _, name := range platformAITools {
		t.Run(string(name), func(t *testing.T) {
			rec := getToolSchema(t, srv, string(name))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var body struct {
				ID         string         `json:"$id"`
				Title      string         `json:"title"`
				Properties map[string]any `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, string(name), body.ID)
			assert.NotEmpty(t, body.Title)
			assert.NotEmpty(t, body.Properties)
		})
	}
}

// A flow definition naming a translate step passes the gates a run applies
// before a block is read.
func TestTranslateStepValidatesOnTheServerRegistry(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))

	def, err := srv.flowCatalog().Get(context.Background(), "", "translate")
	require.NoError(t, err)
	require.NoError(t, def.Validate())
	require.NoError(t, def.ValidateDataFlow(srv.ToolRegistry))
	require.NoError(t, def.CheckPlacement(srv.ToolRegistry))
}

// The flow runner is granted the platform's provider, so a translate step of a
// run calls the platform backend rather than the placeholder provider the
// registry entry carries as its zero-argument default.
func TestFlowRunnerIsGrantedThePlatformProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PlatformProvider = string(aiprovider.Demo)
	srv := shutdownOnCleanup(t, NewServer(cfg))

	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	require.NoError(t, cs.CreateProject(context.Background(), &store.Project{
		ID: "p1", Name: "Flow", WorkspaceID: "ws-1", DefaultSourceLanguage: "en",
	}))
	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)
	srv.wireFlowAIProvider()

	built, err := srv.Services.Flow.NewToolForRun(context.Background(),
		srv.Services.Flow.BeginAIRun(context.Background(), "p1", ""), "translate", nil, "fr")
	require.NoError(t, err)
	require.Equal(t, "translate", built.Name())

	// The demo backend answers offline. A step left on the registry default
	// would reach for a cloud provider with no credential and fail here, so a
	// clean run is the grant.
	block := model.NewBlock("b1", "Hello there.")
	block.Translatable = true
	out, err := tool.RunOnParts(context.Background(), built,
		[]*model.Part{{Type: model.PartBlock, Resource: block}})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.NotEmpty(t, block.TargetText("fr"))
}

// The platform's AI backend is what a flow's model-backed step calls.
func TestFlowAIProviderResolvesThePlatformBackend(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PlatformProvider = string(aiprovider.Demo)
	srv := shutdownOnCleanup(t, NewServer(cfg))

	prov, err := srv.flowAIProvider(context.Background(), "", "")
	require.NoError(t, err)
	require.NotNil(t, prov)
	assert.Equal(t, aiprovider.Demo, prov.Name())
}

// A self-hosted instance with no platform backend grants nothing, which leaves
// each step to the provider its own config names.
func TestFlowAIProviderGrantsNothingWhenUnconfigured(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PlatformProvider = ""
	srv := shutdownOnCleanup(t, NewServer(cfg))

	prov, err := srv.flowAIProvider(context.Background(), "", "")
	require.NoError(t, err)
	assert.Nil(t, prov)
}
