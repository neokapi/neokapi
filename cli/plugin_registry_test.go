package cli

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterCommandFactory_AppliedInOrder(t *testing.T) {
	ResetPluginRegistriesForTest()
	defer ResetPluginRegistriesForTest()

	var calls []string
	RegisterCommandFactory(func(parent *cobra.Command, app *App) {
		calls = append(calls, "first")
		parent.AddCommand(&cobra.Command{Use: "first"})
	})
	RegisterCommandFactory(func(parent *cobra.Command, app *App) {
		calls = append(calls, "second")
		parent.AddCommand(&cobra.Command{Use: "second"})
	})

	root := &cobra.Command{Use: "root"}
	app := &App{}
	ApplyCommandFactories(root, app)

	assert.Equal(t, []string{"first", "second"}, calls)
	require.Len(t, root.Commands(), 2)
	assert.Equal(t, "first", root.Commands()[0].Use)
	assert.Equal(t, "second", root.Commands()[1].Use)
}

func TestRegisterAppInitializer_RunsAll(t *testing.T) {
	ResetPluginRegistriesForTest()
	defer ResetPluginRegistriesForTest()

	RegisterAppInitializer(func(a *App) {
		a.SourceLang = "en"
	})
	RegisterAppInitializer(func(a *App) {
		a.TargetLang = "fr"
	})

	app := &App{}
	ApplyAppInitializers(app)

	assert.Equal(t, "en", app.SourceLang)
	assert.Equal(t, "fr", app.TargetLang)
}

func TestRegisterMCPToolFactory_RunsAll(t *testing.T) {
	ResetPluginRegistriesForTest()
	defer ResetPluginRegistriesForTest()

	var seenServers []*mcp.Server
	RegisterMCPToolFactory(func(s *mcp.Server, app *App) {
		seenServers = append(seenServers, s)
	})
	RegisterMCPToolFactory(func(s *mcp.Server, app *App) {
		seenServers = append(seenServers, s)
	})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	ApplyMCPToolFactories(srv, &App{})

	require.Len(t, seenServers, 2)
	assert.Same(t, srv, seenServers[0])
	assert.Same(t, srv, seenServers[1])
}

func TestResetPluginRegistries_ClearsState(t *testing.T) {
	ResetPluginRegistriesForTest()
	RegisterCommandFactory(func(*cobra.Command, *App) {})
	RegisterAppInitializer(func(*App) {})
	RegisterMCPToolFactory(func(*mcp.Server, *App) {})

	ResetPluginRegistriesForTest()

	root := &cobra.Command{Use: "root"}
	app := &App{}
	ApplyCommandFactories(root, app)
	ApplyAppInitializers(app)
	ApplyMCPToolFactories(mcp.NewServer(&mcp.Implementation{Name: "test"}, nil), app)

	assert.Empty(t, root.Commands())
}

func TestApply_NoFactoriesIsSafe(t *testing.T) {
	ResetPluginRegistriesForTest()
	defer ResetPluginRegistriesForTest()

	root := &cobra.Command{Use: "root"}
	app := &App{}

	assert.NotPanics(t, func() {
		ApplyCommandFactories(root, app)
		ApplyAppInitializers(app)
		ApplyMCPToolFactories(mcp.NewServer(&mcp.Implementation{Name: "test"}, nil), app)
	})
}

func TestNewMCPCmd_DefaultName(t *testing.T) {
	app := &App{}
	cmd := app.NewMCPCmd("")
	assert.Equal(t, "mcp", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	cmdNamed := app.NewMCPCmd("bowrain")
	assert.Equal(t, "mcp", cmdNamed.Use)
}
