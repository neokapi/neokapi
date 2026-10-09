package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVenueProjectCarriesTheRecipeID(t *testing.T) {
	recipe := &project.Recipe{
		Version:  coreproj.CurrentVersion,
		Name:     "docs",
		Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"nb", "fr"}},
	}
	recipe.ID = coreproj.NewID()

	p := venueProject(recipe, "docs")
	assert.Equal(t, recipe.ID, p.ID)
	assert.Equal(t, "docs", p.Name)
	assert.Equal(t, "en", p.SourceLocale)
	assert.Equal(t, []string{"nb", "fr"}, p.TargetLocales)

	recipe.ID = ""
	assert.Empty(t, venueProject(recipe, "docs").ID, "a recipe with no id sends none")
}

// anonymousVenue answers the anonymous create route with the id it was sent
// (or a minted one) and records every request body.
func anonymousVenue(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects/anonymous" {
			http.Error(w, "unexpected request "+r.URL.Path, http.StatusNotFound)
			return
		}
		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		id, _ := body["id"].(string)
		if id == "" {
			id = "minted01"
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"project_id": id, "claim_token": "clm_test"})
	}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return bodies
	}
}

// runInitConnectCommand runs `init-connect` with args the way `kapi init`
// dispatches it, with KAPI_PROJECT_DIR naming the project. The flags are
// package state, so they are cleared once the test ends.
func runInitConnectCommand(t *testing.T, root string, args ...string) error {
	t.Helper()
	t.Setenv("KAPI_PROJECT_DIR", root)
	cmd := &cobra.Command{Use: "command", SilenceUsage: true, SilenceErrors: true}
	cli.AddPersistentFlags(&cli.App{}, cmd)
	cmd.AddCommand(initConnectCmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"init-connect"}, args...))
	// cobra hands a subcommand the root's context only when it has none, so
	// a run after another test's would keep that test's canceled context.
	initConnectCmd.SetContext(t.Context())
	t.Cleanup(func() {
		for _, name := range []string{"server", "project", "email", "workspace"} {
			_ = initConnectCmd.Flags().Set(name, "")
		}
		_ = initConnectCmd.Flags().Set("anonymous", "false")
	})
	return cmd.ExecuteContext(t.Context())
}

func TestInitConnectSendsTheRecipeID(t *testing.T) {
	isolateKapi(t)
	srv, requests := anonymousVenue(t)

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := &project.Recipe{
		Version:  coreproj.CurrentVersion,
		Name:     "Docs",
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
	}
	recipe.ID = coreproj.NewID()
	_, err = project.InitProject(root, recipe)
	require.NoError(t, err)

	require.NoError(t, runInitConnectCommand(t, root, "--server", srv.URL, "--anonymous"))

	bodies := requests()
	require.Len(t, bodies, 1)
	assert.Equal(t, recipe.ID, bodies[0]["id"], "the request carries the recipe's id")
	assert.Equal(t, "Docs", bodies[0]["name"])

	connected, err := project.Load(root)
	require.NoError(t, err)
	require.True(t, connected.Recipe.HasServer())
	assert.True(t, strings.HasSuffix(connected.Recipe.Server.URL, "/"+recipe.ID),
		"the recipe points at the project under its own id: %s", connected.Recipe.Server.URL)
	assert.Equal(t, recipe.ID, connected.Recipe.ID, "connecting leaves the id as it was")
}

func TestInitConnectSendsTheNameWhenTheRecipeHasNoID(t *testing.T) {
	isolateKapi(t)
	srv, requests := anonymousVenue(t)

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	_, err = project.InitProject(root, &project.Recipe{
		Version:  coreproj.CurrentVersion,
		Name:     "Docs",
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
	})
	require.NoError(t, err)

	require.NoError(t, runInitConnectCommand(t, root, "--server", srv.URL, "--anonymous"))

	bodies := requests()
	require.Len(t, bodies, 1)
	_, hasID := bodies[0]["id"]
	assert.False(t, hasID, "a recipe with no id sends none")
	assert.Equal(t, "Docs", bodies[0]["name"])

	connected, err := project.Load(root)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(connected.Recipe.Server.URL, "/minted01"), "the venue's id is kept: %s", connected.Recipe.Server.URL)
}
