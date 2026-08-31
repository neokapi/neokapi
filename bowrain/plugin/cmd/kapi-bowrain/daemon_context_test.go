package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/source"
)

// A push arrives by two routes and both must declare the recipe's context:
// `kapi-bowrain push` runs the cobra command, and `kapi push` is dispatched to
// this daemon over RPC. Only the cobra route ever did.
//
// The daemon route sent no context hash, which the server reads — correctly —
// as "this push makes no claim about the declared context", so it reported the
// context unchanged and the client put no entries in the manifest. A push then
// carried every block and reconciled zero collections. That is what the
// production dogfood sync did nightly: 21,894 blocks, no collections.
//
// This pins the seam rather than the symptom: after the daemon prepares a push,
// the connector must be carrying a context to declare.
func TestDaemonPushDeclaresTheRecipeContext(t *testing.T) {
	// The daemon hangs off rootCmd rather than the `command` subtree, so it
	// never ran the App initialization that subtree does — leaving
	// commands.app nil and BuildPushContext returning nothing at all. Set the
	// App the way runDaemon now does.
	prev := app
	app = &cli.App{}
	app.InitRegistries()
	cli.ApplyAppInitializers(app)
	t.Cleanup(func() { app = prev })

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"),
		[]byte("name: Test Voice\ndescription: How it sounds.\n"), 0o644))

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Profiles: map[string]coreproj.Profile{
			"kapi": {
				Channels: []coreproj.Channel{{ID: "docs"}, {ID: "app"}},
				Voice:    &coreproj.VoiceBinding{ProfileFile: "voice.yaml"},
			},
		},
		Collections: []coreproj.Collection{
			{
				Name:    "kapi-docs",
				Channel: "kapi/docs",
				Content: []coreproj.ContentItem{{Path: "docs/**/*.json"}},
			},
			{
				Name:    "kapi-app",
				Channel: "kapi/app",
				Content: []coreproj.ContentItem{{Path: "app/**/*.json"}},
			},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	entry := &projectEntry{
		project:   proj,
		connector: source.NewLocalConnector(app, proj, app.FormatReg),
	}

	require.NoError(t, declareContext(t.Context(), entry, false))

	assert.True(t, entry.connector.PushContextChanged(),
		"the daemon must hand the connector a context to declare; a nil one sends no "+
			"context hash and the server reads that as 'makes no claim'")

	// And the context it declares is the recipe's, not an empty fold.
	pushCtx, _, err := transfer.BuildPushContext(t.Context(), app, proj, false)
	require.NoError(t, err)
	require.NotNil(t, pushCtx)
	assert.Len(t, pushCtx.Entries, 2, "one entry per named collection")
	assert.NotEmpty(t, pushCtx.Hash)
}
