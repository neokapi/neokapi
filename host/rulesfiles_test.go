package host

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/agentrules"
	"github.com/neokapi/neokapi/core/contextop"
)

var updateRulesGolden = flag.Bool("update-rules-golden", false,
	"rewrite host/testdata/rulesfiles from what WriteRulesFiles writes")

// spacesRecipe is a product that renamed Workspaces to Spaces where customers
// read: the help pages and the release notes. The API reference keeps
// `workspace`, because the endpoint names are a published contract, and the
// legal terms keep the old name until the contracts are renewed, under a
// voice of their own.
const spacesRecipe = `version: v1
id: ` + "prj_rulesfilesspacesaaaaaaa" + `
name: spaces
defaults:
  source_language: en
profiles:
  customer:
    channels: [help, changelog]
  api:
    channels: [reference]
  legal:
    channels: [terms]
collections:
  - name: help
    channel: customer/help
    source_only: true
    content:
      - path: "help/**/*.md"
  - name: changelog
    channel: customer/changelog
    source_only: true
    content:
      - path: "changelog/*.md"
  - name: api-reference
    channel: api/reference
    source_only: true
    content:
      - path: "api/**/*.md"
  - name: legal
    channel: legal/terms
    source_only: true
    content:
      - path: "legal/*.md"
`

// spacesProject writes the project, reads its voices in, and records its
// rules the way a person does: a project-wide rule seen in the README, and the
// rename seen in the help pages, kept and applied to every customer channel.
func spacesProject(t *testing.T) (*App, string) {
	t.Helper()
	app, _ := contextOpsApp(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for rel, body := range map[string]string{
		"kapi.yaml": spacesRecipe,
		".kapi/voice.yaml": "id: house\nname: House\ndescription: Plain and direct, for busy team leads.\n" +
			"tone:\n  guidelines: Lead with the answer.\n",
		".kapi/profiles/legal/voice.yaml": "id: legal\nname: Legal\ndescription: Exact and formal.\n",
		"README.md":                       "Teams log in and send e-mail from a Workspace.\n",
		"help/getting-started.md":         "Each board belongs to a Workspace.\n\nThe Workspace admin invites people.\n",
		"changelog/3.4.md":                "Workspaces are now faster.\n",
		"api/workspaces.md":               "GET /workspaces lists every workspace.\n",
		"legal/terms.md":                  "The Workspace is provided as is.\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	_, err = app.ImportProjectContext(t.Context(), recipeOf(root), ContextImportRequest{})
	require.NoError(t, err)

	email, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "email", InsteadOf: []string{"e-mail"},
		Evidence: []contextop.Evidence{{Path: "README.md", Quote: "send e-mail"}},
	})
	require.NoError(t, err)
	space, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "Space", InsteadOf: []string{"Workspace"},
		Evidence: []contextop.Evidence{{Path: "help/getting-started.md", Quote: "belongs to a Workspace"}},
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{
		Actor: person, Project: recipeOf(root), Keep: []string{email.ID},
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{
		Actor: person, Project: recipeOf(root), Keep: []string{space.ID}, WidenTo: "channel",
	})
	require.NoError(t, err)
	// A compound name renamed with it: the rule also avoids the new name's
	// own misspellings, "Space-Admin" and "SpaceAdmin", which are never the
	// wording to keep where the rename does not hold.
	admin, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "Space Admin", InsteadOf: []string{"Workspace admin"},
		Evidence: []contextop.Evidence{{Path: "help/getting-started.md", Quote: "The Workspace admin invites people"}},
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{
		Actor: person, Project: recipeOf(root), Keep: []string{admin.ID}, WidenTo: "channel",
	})
	require.NoError(t, err)
	return app, root
}

// rulesFilesIn reads every rules file under root, by project-relative path.
func rulesFilesIn(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".kapi" {
			return filepath.SkipDir
		}
		if !d.IsDir() && (d.Name() == "AGENTS.md" || d.Name() == "CLAUDE.md") {
			body, rerr := os.ReadFile(path)
			require.NoError(t, rerr)
			out[relSlash(root, path)] = string(body)
		}
		return nil
	}))
	return out
}

// TestWriteRulesFiles_ScopedRename is the golden sample: the root states the
// project-wide rule and names the folders with rules of their own; help/ and
// changelog/ say Space; api/ and legal/ say the old name is correct there, and
// legal/ carries its own voice.
func TestWriteRulesFiles_ScopedRename(t *testing.T) {
	app, root := spacesProject(t)

	res, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	got := rulesFilesIn(t, root)

	golden := filepath.Join("testdata", "rulesfiles", "spaces")
	if *updateRulesGolden {
		require.NoError(t, os.RemoveAll(golden))
		for rel, body := range got {
			path := filepath.Join(golden, filepath.FromSlash(rel)+".golden")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		}
	}
	want := map[string]string{}
	require.NoError(t, filepath.WalkDir(golden, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, rerr := os.ReadFile(path)
		require.NoError(t, rerr)
		want[strings.TrimSuffix(relSlash(golden, path), ".golden")] = string(body)
		return nil
	}))
	assert.Equal(t, sortedKeys(want), sortedKeys(got), "the folders that get rules files")
	for rel, body := range want {
		assert.Equal(t, body, got[rel], rel)
	}

	paths := make([]string, 0, len(res.Files))
	for _, f := range res.Files {
		assert.Equal(t, RulesFileCreated, f.Action, f.Path)
		paths = append(paths, f.Path)
	}
	assert.Equal(t, sortedKeys(got), paths, "the result names every file it wrote, in order")

	// A second write changes no byte.
	again, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	assert.Empty(t, again.Changed())
	assert.Equal(t, got, rulesFilesIn(t, root))
}

// TestWriteRulesFiles_TheAnswerSaysTheOldNameIsCorrect: the answer for a file
// where the rename does not hold says plainly that the old name is correct
// there, which is what an agent asked to rename it needs to read.
func TestWriteRulesFiles_TheAnswerSaysTheOldNameIsCorrect(t *testing.T) {
	app, root := spacesProject(t)
	cmd := NewEnvCommand(t.Context(), "context")
	cmd.Flags().String(projectFlagName, recipeOf(root), "")

	tests := []struct {
		path   string
		want   string
		absent string
	}{
		{path: "legal/terms.md", want: `- "Workspace" is correct here; do not rename it to "Space". That rename holds only in changelog/ and help/.`},
		{path: "api/workspaces.md", want: `- "Workspace" is correct here; do not rename it to "Space".`},
		{path: "api/workspaces.md", want: `- "Workspace admin" is correct here; do not rename it to "Space Admin".`, absent: "SpaceAdmin"},
		{path: "help/getting-started.md", want: `Space, not "Workspace"`, absent: "is correct here"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := ContextPointRequest{Path: filepath.Join(root, tt.path)}
			src, release := app.ContextSourcesAt(cmd, req)
			defer release()
			ans, err := ResolveContextAt(t.Context(), src, req)
			require.NoError(t, err)
			var b bytes.Buffer
			require.NoError(t, ans.FormatText(&b))
			assert.Contains(t, b.String(), tt.want)
			assert.NotContains(t, b.String(), "can differ")
			if tt.absent != "" {
				assert.NotContains(t, b.String(), tt.absent)
			}
		})
	}
}

// TestWriteRulesFiles_KeepsWhatAPersonWrote: the person's own text in a rules
// file survives every write, a folder that stops having rules of its own loses
// kapi's section, and a file that held nothing else is deleted.
func TestWriteRulesFiles_KeepsWhatAPersonWrote(t *testing.T) {
	app, root := spacesProject(t)
	own := "# Spaces\n\nRun the tests before you push.\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(own), 0o644))
	apiOwn := "# API\n\nEndpoint names are a contract.\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "api", "CLAUDE.md"), []byte(apiOwn), 0o644))

	_, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(claude), own+"\n"+agentrules.StartLine), "kapi's section follows the person's text")

	// The legal profile stops existing: legal/ keeps the project's voice and
	// no rule held elsewhere differs there any more than it does at the root,
	// and the api collection is dropped altogether.
	recipe := strings.Replace(spacesRecipe, `  - name: api-reference
    channel: api/reference
    source_only: true
    content:
      - path: "api/**/*.md"
`, "", 1)
	require.NoError(t, os.WriteFile(recipeOf(root), []byte(recipe), 0o644))
	res, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)

	actions := map[string]RulesFileAction{}
	for _, f := range res.Files {
		actions[f.Path] = f.Action
	}
	assert.Equal(t, RulesFileDeleted, actions["api/AGENTS.md"], "a file kapi created and no longer needs goes")
	assert.Equal(t, RulesFileRemoved, actions["api/CLAUDE.md"], "a file with the person's text keeps it")
	body, err := os.ReadFile(filepath.Join(root, "api", "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, apiOwn, string(body))
	_, err = os.Stat(filepath.Join(root, "api", "AGENTS.md"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

// TestWriteRulesFiles_ThisRepositorysClaudeFile: the hand-written CLAUDE.md at
// the root of this repository, which carries the voice pointer earlier kapi
// versions wrote, keeps every line of its own when the section replaces the
// pointer.
func TestWriteRulesFiles_ThisRepositorysClaudeFile(t *testing.T) {
	repo, err := os.ReadFile(filepath.Join("..", "CLAUDE.md"))
	require.NoError(t, err)
	app, root := spacesProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), repo, 0o644))

	_, err = app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)

	without, removed, err := agentrules.Remove(got)
	require.NoError(t, err)
	require.True(t, removed)
	original, _, err := agentrules.Remove(repo)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(without), "every hand-written line survives")
	assert.NotContains(t, string(got), "<!-- kapi:voice", "the pointer gives way to the section")
}

// TestWriteRulesFiles_ClaudeImportingAgents: a CLAUDE.md that imports
// AGENTS.md reaches the section through it and is given none of its own.
func TestWriteRulesFiles_ClaudeImportingAgents(t *testing.T) {
	app, root := spacesProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("@AGENTS.md\n"), 0o644))
	_, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "@AGENTS.md\n", string(body))
}

// TestRefreshRulesFiles_FollowsADecision: a project whose root holds kapi's
// section has its files refreshed by a review decision, and one without is
// left alone.
func TestRefreshRulesFiles_FollowsADecision(t *testing.T) {
	app, root := spacesProject(t)
	assert.Empty(t, rulesFilesIn(t, root), "a project that never wrote its rules files gets none from a decision")

	_, err := app.WriteRulesFiles(t.Context(), recipeOf(root))
	require.NoError(t, err)
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "sign in", InsteadOf: []string{"log in"},
		Evidence: []contextop.Evidence{{Path: "README.md"}},
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{Actor: person, Project: recipeOf(root), Keep: []string{op.ID}})
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), `sign in, not "log in"`)
}
