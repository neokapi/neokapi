package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/screens"
)

// The Decide parity fixture: one project, one ledger state, and the view
// Decide shows for them. bowrain/server's Decide test stores the same blocks
// at the same points, pushes the same log, and holds the server's view to
// the same file, so the desktop and the server answer one ledger state with
// one JSON document.
//
//	project/          the recipe and its source files
//	log.jsonl         the ledger, as a segment of the project's operations
//	pushed.json       the collections and blocks a push of the project carries
//	decide-view.json  the view Decide shows the member reading
//
// Regenerate decide-view.json and pushed.json after an intended change with
//
//	KAPI_UPDATE_GOLDEN=1 go test ./host -run TestDecideParity
//
// and log.jsonl, which records a new ledger, with KAPI_UPDATE_DECIDE_LOG=1.
var decideParityDir = filepath.Join("screens", "testdata", "decide-parity")

// decideParityReader is the member reading the view: what they recorded
// reads as "you".
var decideParityReader = contextop.Actor{Kind: contextop.ActorPerson, Name: "member-a"}

// DecideParityPushed is what a push of the parity project carries: each
// collection at its point, and each file's blocks.
type DecideParityPushed struct {
	Collections []DecideParityCollection `json:"collections"`
	Items       []DecideParityItem       `json:"items"`
}

type DecideParityCollection struct {
	Name        string            `json:"name"`
	Coordinates map[string]string `json:"coordinates"`
}

type DecideParityItem struct {
	Name       string   `json:"name"`
	Collection string   `json:"collection"`
	Blocks     []string `json:"blocks"`
}

func TestDecideParity(t *testing.T) {
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS(filepath.Join(decideParityDir, "project"))))
	recipe := recipeOf(root)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	key := workspace.ProjectKey(proj.ID)
	logPath := filepath.Join(decideParityDir, "log.jsonl")

	if os.Getenv("KAPI_UPDATE_DECIDE_LOG") != "" {
		recordDecideParityLog(t, app, recipe, key, logPath)
	} else {
		data, err := os.ReadFile(logPath)
		require.NoError(t, err)
		ops, err := workspace.DecodeSegment(data, key)
		require.NoError(t, err)
		ws, err := app.Workspace(ctx)
		require.NoError(t, err)
		_, err = ws.Register(ctx, key, proj.Name, NormalizeCheckoutPath(root))
		require.NoError(t, err)
		_, err = ws.Record(ctx, ops...)
		require.NoError(t, err)
	}

	app.ActAs(decideParityReader)
	view, err := app.DecideRules(ctx, RulesRequest{Project: recipe, WithCounts: true})
	require.NoError(t, err)

	// The view as the desktop's RulesDecideView shapes it.
	out := screens.RulesDecideViewDTO{
		Project: string(key), ProjectName: proj.Name, Decisions: []screens.RuleDecisionDTO{},
		Rules: view.Counts.Rules, Suggested: view.Counts.Suggested, Narrowed: view.Counts.Narrowed,
	}
	for _, d := range view.Decisions {
		out.Decisions = append(out.Decisions, screens.RuleDecisionFrom(d.Model()))
	}
	require.NotEmpty(t, out.Decisions)

	pushed := DecideParityPushed{}
	for _, c := range view.st.places.Collections {
		pushed.Collections = append(pushed.Collections, DecideParityCollection{Name: c.Name, Coordinates: c.Coordinates})
	}
	for _, b := range view.blocks {
		i := slices.IndexFunc(pushed.Items, func(it DecideParityItem) bool { return it.Name == b.Path })
		if i < 0 {
			pushed.Items = append(pushed.Items, DecideParityItem{Name: b.Path, Collection: proj.CollectionForPath(b.Path)})
			i = len(pushed.Items) - 1
		}
		pushed.Items[i].Blocks = append(pushed.Items[i].Blocks, b.Text)
	}

	golden(t, filepath.Join(decideParityDir, "decide-view.json"), out)
	golden(t, filepath.Join(decideParityDir, "pushed.json"), pushed)
}

// golden holds value's JSON to the file at path, or writes it there under
// KAPI_UPDATE_GOLDEN.
func golden(t *testing.T, path string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')
	if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "fixture missing; run with KAPI_UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got), "%s differs from what Decide reads; regenerate with KAPI_UPDATE_GOLDEN=1 if the change is intended", path)
}

// recordDecideParityLog records the parity ledger through the calls kapi
// makes and writes it to path as one segment: a name and two word rules that
// disagree, suggested by an assistant; a note the reader suggested; and a
// rule another person kept.
func recordDecideParityLog(t *testing.T, app *App, recipe string, key workspace.ProjectKey, path string) {
	t.Helper()
	ctx := t.Context()
	agent := contextop.Actor{Kind: contextop.ActorAgent, Name: "codex@build-01", Session: "s-parity"}
	observe := func(req ContextObserveRequest) ContextOperation {
		t.Helper()
		req.Project = recipe
		op, err := app.RecordContextObservation(ctx, req)
		require.NoError(t, err)
		return op
	}
	observe(ContextObserveRequest{
		Actor: agent, Term: "Sign in", InsteadOf: []string{"Sign In"},
		Evidence: []contextop.Evidence{{Path: "docs/a.md", Quote: "Please Sign In first."}},
	})
	observe(ContextObserveRequest{
		Actor: agent, Term: "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/b.md", Quote: "We utilise the widget."}},
	})
	observe(ContextObserveRequest{
		Actor: agent, Term: "employ", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/b.md", Quote: "We utilise the widget."}},
	})
	observe(ContextObserveRequest{
		Actor: decideParityReader, Text: "Help pages address a reader who is stuck.",
		Evidence: []contextop.Evidence{{Path: "help/c.md"}},
	})
	widget := observe(ContextObserveRequest{
		Actor: agent, Term: "widget", InsteadOf: []string{"gadget"},
		Evidence: []contextop.Evidence{{Path: "docs/b.md", Quote: "We utilise the widget."}},
	})
	_, err := app.DecideRule(ctx, RuleDecideRequest{
		Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"}, Project: recipe, ID: widget.ID, Answer: RuleAnswerKeep,
	})
	require.NoError(t, err)

	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	ops, err := ws.Select(ctx, workspace.OpQuery{Project: key, KindPrefix: contextop.OpKindPrefix})
	require.NoError(t, err)
	data, err := workspace.EncodeSegment(ops)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
