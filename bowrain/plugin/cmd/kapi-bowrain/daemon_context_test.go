package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v1"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/source"
)

// A push arrives by two routes: `kapi-bowrain push` runs the cobra command, and
// `kapi push` is dispatched to this daemon over RPC. Both run
// commands.PushProject, so both declare the recipe's context. A push with no
// context hash reads to the server as "this push makes no claim about the
// declared context", and would carry every block while reconciling no
// collections.
//
// After the daemon pushes, the connector must be carrying a context to declare.
func TestDaemonPushDeclaresTheRecipeContext(t *testing.T) {
	daemonTestApp(t)
	root := t.TempDir()
	voicePath := filepath.Join(root, coreproj.RelStatePath(coreproj.ProfilesDirName, "kapi", "voice.yaml"))
	require.NoError(t, os.MkdirAll(filepath.Dir(voicePath), 0o755))
	require.NoError(t, os.WriteFile(voicePath,
		[]byte("name: Test Voice\ndescription: How it sounds.\n"), 0o644))

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Profiles: map[string]coreproj.Profile{
			"kapi": {
				Channels: []coreproj.Channel{{ID: "docs"}, {ID: "app"}},
				Voice:    &coreproj.VoiceBinding{Profile: "test-voice"},
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
	// The recipe binds the voice by name; the import puts it in the store.
	_, err = app.ImportProjectContext(t.Context(), proj.Layout.RecipePath, host.ContextImportRequest{})
	require.NoError(t, err)

	d, entry := daemonWithProject(root, proj)
	resp, err := d.Push(t.Context(), &pb.PushRequest{Project: &pb.ProjectRef{Root: root}, DryRun: true})
	require.NoError(t, err)
	assert.Contains(t, resp.GetReport(), "Would push", "the push report travels back to kapi")

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

// A recipe's pre-push automations run on the daemon route too. A failing
// pre-push gate stops the push, and what the gate printed travels back with
// the failure for kapi to show.
func TestDaemonPushRunsThePrePushAutomations(t *testing.T) {
	daemonTestApp(t)
	root := t.TempDir()
	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []coreproj.Collection{{
			Path:   "src/*.xlf",
			Format: &coreproj.FormatSpec{Name: "xliff"},
			Target: "out/{lang}/*.xlf",
		}},
		Flows: map[string]*flow.StepsSpec{
			"guard": {Steps: []flow.FlowStep{
				{Tool: "dnt-check", Config: map[string]any{"terms": []string{"Acme Cloud"}}},
			}},
		},
		Automations: []bproject.AutomationSpec{{
			Name:    "checks-gate",
			Trigger: bproject.HookPrePush,
			Actions: []bproject.ActionConfig{{
				Type:   bproject.ActionRunFlow,
				Config: map[string]string{"flow": "guard", "fail_on_error": "true"},
			}},
		}},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	src := filepath.Join(root, "src", "app.xlf")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="nb" datatype="plaintext" original="app">
    <body>
      <trans-unit id="save">
        <source>Save now with Acme Cloud</source>
        <target>Lagre nå med Toppskyen</target>
      </trans-unit>
    </body>
  </file>
</xliff>
`), 0o644))

	d, _ := daemonWithProject(root, proj)
	resp, err := d.Push(t.Context(), &pb.PushRequest{Project: &pb.ProjectRef{Root: root}, DryRun: true})
	require.NoError(t, err, "a failed automation rides on the response")
	assert.Contains(t, resp.GetAutomationError(), "pre-push automation")
	assert.Contains(t, resp.GetReport(), "Running automation: checks-gate")
	assert.NotContains(t, resp.GetReport(), "Would push", "a failed pre-push gate stops the push")
	assert.Zero(t, resp.GetBlocksPushed())
}

// daemonTestApp sets the process App the way runDaemon does, isolated from the
// developer's kapi installation.
func daemonTestApp(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_CONFIG_DIR", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	prev := app
	app = &cli.App{}
	app.InitRegistries()
	app.AssumeYes = true
	cli.ApplyAppInitializers(app)
	t.Cleanup(func() { app = prev })
}

// daemonWithProject is a daemon already serving the project at root over a
// connector with no server, so a dry-run push runs offline.
func daemonWithProject(root string, proj *bproject.Project) (*daemonService, *projectEntry) {
	d := newDaemonService(nil)
	entry := &projectEntry{
		project:   proj,
		connector: source.NewLocalConnector(app, proj, app.FormatReg),
	}
	d.projects[root] = entry
	return d, entry
}
