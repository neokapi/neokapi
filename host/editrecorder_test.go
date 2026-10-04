package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/workspace"
)

// recorderProject is a project with two extracted source files, its workspace
// of its own, and the recorder bound to it.
func recorderProject(t *testing.T) (*App, string, change.Recorder) {
	t.Helper()
	a, root, recipe := renameProject(t)
	a.SetWorkspaceRoot(filepath.Join(t.TempDir(), "workspace"))
	for _, name := range []string{"intro", "outro"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "src", name+".en.json"), []byte(`{"greeting":"Hello world"}`), 0o644))
	}
	extractOnce(t, a, recipe)
	rec, err := a.EditRecorder(t.Context(), root)
	require.NoError(t, err)
	return a, root, rec
}

func textRuns(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }

// greetingEdit is the record of a change to the greeting's source and its
// Norwegian translation in one document, as the service hands it over.
func greetingEdit(doc string) []change.Transition {
	b := &model.Block{ID: "greeting", Name: "greeting", Key: "u-greeting", SourceLocale: "en"}
	b.SetSourceRuns(textRuns("Hello there"))
	b.SetTargetText("nb", "Hei der")
	en, _ := model.ParseEditionKey("en")
	nb, _ := model.ParseEditionKey("nb")
	return []change.Transition{
		{
			Ref:  change.Ref{Doc: doc, Block: "greeting"},
			Role: change.RoleAuthoritative, Before: textRuns("Hello world"), After: textRuns("Hello there"),
			BeforeRev: model.RunsRevision(en, textRuns("Hello world")), AfterRev: model.RunsRevision(en, textRuns("Hello there")),
			Block: b,
		},
		{
			Ref:  change.Ref{Doc: doc, Block: "greeting", Edition: nb},
			Role: change.RoleDerived, After: textRuns("Hei der"),
			BeforeRev: model.AbsentRevision, AfterRev: model.RunsRevision(nb, textRuns("Hei der")),
			Basis: model.RunsRevision(en, textRuns("Hello there")),
			Block: b,
		},
	}
}

func editOps(t *testing.T, a *App, root string) []workspace.Op {
	t.Helper()
	p, err := a.Projector(t.Context(), root)
	require.NoError(t, err)
	ws := a.ensureProjectStores()
	ws.mu.Lock()
	bound := ws.bound[mustAbs(t, root)]
	ws.mu.Unlock()
	ops, err := bound.ws.Select(t.Context(), workspace.OpQuery{Project: p.Key(), KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	return ops
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	return abs
}

func TestRecorderKeepsRunsForPeopleAndAgentsAndHashesForTools(t *testing.T) {
	tests := []struct {
		name     string
		actor    change.Actor
		origin   string
		keepRuns bool
	}{
		{name: "an agent", actor: change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s_01"}, origin: "apply", keepRuns: true},
		{name: "a person", actor: change.Actor{Kind: change.ActorPerson, Name: "asgeir"}, origin: "desktop", keepRuns: true},
		{name: "a flow", actor: change.Actor{Kind: change.ActorTool, Name: "translate"}, origin: "flow:up", keepRuns: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, root, rec := recorderProject(t)
			ctx := t.Context()
			after := "sha256:after"
			set := &change.Set{Schema: change.SchemaID, Note: "Greet more warmly"}
			id, err := rec.Record(ctx, change.Record{
				Actor: tc.actor, Origin: tc.origin, Set: set, Fingerprint: "gov_1",
				Docs:        []change.DocResult{{Doc: "src/intro.en.json", Home: "file", Written: true, Before: "sha256:before", After: &after}},
				Transitions: greetingEdit("src/intro.en.json"),
			})
			require.NoError(t, err)
			require.NotEmpty(t, id)

			ops := editOps(t, a, root)
			require.Len(t, ops, 1)
			assert.Equal(t, id, ops[0].ID)
			var e projector.Edit
			require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
			docs, err := a.DocumentIndex(ctx, root)
			require.NoError(t, err)
			assert.Equal(t, docs.Key("src/intro.en.json"), e.Doc.Key, "the document is named by its key")
			assert.Equal(t, "src/intro.en.json", e.Doc.Path)
			assert.Equal(t, "file", e.Home)
			assert.Equal(t, tc.actor, e.Actor)
			assert.Equal(t, tc.origin, e.Origin.By)
			assert.Equal(t, "gov_1", e.Fingerprint)
			assert.Equal(t, "Greet more warmly", e.Note)
			assert.Equal(t, "sha256:before", e.DocBefore)
			assert.Equal(t, "sha256:after", e.DocAfter)
			require.Len(t, e.Transitions, 2)
			src, nb := e.Transitions[0], e.Transitions[1]
			assert.Equal(t, "en", src.Edition, "the document's own edition is named by its language")
			assert.Equal(t, "nb", nb.Edition)
			assert.Equal(t, "u-greeting", src.Key)
			assert.Equal(t, model.ComputeContentHash("Hello there"), src.ContentHash, "identity evidence comes from the block")
			assert.NotEmpty(t, src.ContextHash)
			assert.Equal(t, greetingEdit("")[1].Basis, nb.Basis)
			if tc.keepRuns {
				assert.NotEmpty(t, src.RunsBefore)
				assert.NotEmpty(t, src.RunsAfter)
				assert.Empty(t, nb.RunsBefore, "a created edition has nothing before it")
				assert.NotEmpty(t, nb.RunsAfter)
				assert.NotEmpty(t, e.ChangeSet)
			} else {
				assert.Empty(t, src.RunsBefore+src.RunsAfter+nb.RunsAfter, "a flow's record is hash-only")
				assert.Empty(t, e.ChangeSet)
			}

			db, err := a.ProjectDB(ctx, root)
			require.NoError(t, err)
			last, found, err := db.History().LastWrite(ctx, e.Doc.Key, "greeting", "nb")
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, string(tc.actor.Kind), last.Actor, "who last wrote this, for every writer")
			assert.Equal(t, tc.actor.Name, last.ActorName)
			assert.Equal(t, tc.origin, last.Origin)
		})
	}
}

func TestRecorderWritesOneOperationPerDocument(t *testing.T) {
	a, root, rec := recorderProject(t)
	ctx := t.Context()
	transitions := append(greetingEdit("src/intro.en.json"), greetingEdit("src/outro.en.json")...)
	id, err := rec.Record(ctx, change.Record{
		Actor:  change.Actor{Kind: change.ActorAgent, Name: "claude"},
		Origin: "apply", Set: &change.Set{Schema: change.SchemaID},
		Docs: []change.DocResult{
			{Doc: "src/outro.en.json", Home: "file", Written: true},
			{Doc: "src/intro.en.json", Home: "file", Written: true},
		},
		Transitions: transitions,
		Overridden: []change.Finding{
			{Rule: "terms.vocabulary", Message: "in intro", Fails: true, At: &change.Ref{Doc: "src/intro.en.json", Block: "greeting"}},
			{Rule: "voice.pattern", Message: "anywhere"},
		},
	})
	require.NoError(t, err)

	ops := editOps(t, a, root)
	require.Len(t, ops, 2)
	var outro, intro projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &outro))
	require.NoError(t, json.Unmarshal(ops[1].Payload, &intro))
	assert.Equal(t, id, ops[0].ID, "the record's id is the first document's operation")
	assert.Equal(t, "src/outro.en.json", outro.Doc.Path, "documents are recorded in the order the result lists them")
	assert.NotEqual(t, outro.Doc.Key, intro.Doc.Key)
	assert.Equal(t, outro.ChangeSet, intro.ChangeSet, "both name the one change set")
	assert.Len(t, intro.Overridden, 2)
	assert.Len(t, outro.Overridden, 1, "a finding placed in another document stays with it")
}

// TestRecorderLeavesAWorkspaceHomeWriteToItsCommit: the workspace home
// records a write when it commits it, with the edition the write leaves, so
// the recorder records nothing more for a document result in that home and
// records the change set's other documents as it records any.
func TestRecorderLeavesAWorkspaceHomeWriteToItsCommit(t *testing.T) {
	a, root, rec := recorderProject(t)
	ctx := t.Context()
	key := reconcile.DocumentKeyFor("parked/intro")
	id, err := rec.Record(ctx, change.Record{
		Actor: change.Actor{Kind: change.ActorTool, Name: "translate"}, Origin: "flow:up",
		Docs:        []change.DocResult{{Doc: key, Home: "workspace", Written: true}},
		Transitions: greetingEdit(key),
	})
	require.NoError(t, err)
	assert.Empty(t, id, "the commit recorded the write, so nothing more is")
	assert.Empty(t, editOps(t, a, root))
}

// redactingProject is recorderProject whose recipe declares redaction with
// the detectors given, and a rules file that withholds a product name.
func redactingProject(t *testing.T, detectors ...string) (*App, string, change.Recorder) {
	t.Helper()
	a, root, _ := recorderProject(t)
	recipe := filepath.Join(root, project.RecipeFileName)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Redaction = &project.RedactionSpec{Enabled: true, Rules: "redaction.yaml", Detectors: detectors}
	require.NoError(t, project.Save(recipe, proj))
	rules := &redaction.RulesFile{Rules: []redaction.Rule{{Term: "Falcon", Category: "product"}}}
	require.NoError(t, rules.Save(filepath.Join(root, "redaction.yaml")))
	rec, err := a.EditRecorder(t.Context(), root)
	require.NoError(t, err)
	return a, root, rec
}

// falconEdit is an agent's change to the greeting that names the withheld
// product before and after, and in its note.
func falconEdit(doc string) change.Record {
	b := &model.Block{ID: "greeting", Name: "greeting", SourceLocale: "en"}
	b.SetSourceRuns(textRuns("Falcon ships today"))
	en, _ := model.ParseEditionKey("en")
	return change.Record{
		Actor: change.Actor{Kind: change.ActorAgent, Name: "claude"}, Origin: "apply",
		Set:  &change.Set{Schema: change.SchemaID, Note: "Say when Falcon ships"},
		Docs: []change.DocResult{{Doc: doc, Home: "file", Written: true}},
		Transitions: []change.Transition{{
			Ref:  change.Ref{Doc: doc, Block: "greeting"},
			Role: change.RoleAuthoritative, Before: textRuns("Falcon is coming"), After: textRuns("Falcon ships today"),
			BeforeRev: model.RunsRevision(en, textRuns("Falcon is coming")), AfterRev: model.RunsRevision(en, textRuns("Falcon ships today")),
			Block: b,
		}},
	}
}

// boundWorkspace is the workspace the project rooted at root records into.
func boundWorkspace(t *testing.T, a *App, root string) *workspace.Workspace {
	t.Helper()
	ws := a.ensureProjectStores()
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.bound[mustAbs(t, root)].ws
}

// TestRecorderKeepsWithheldValuesOutOfTheRecord: a project that declares
// redaction records an agent's edit with the withheld value replaced in the
// runs it keeps and in its note, keeps the original in the project vault, and
// leaves the change set out. Nothing the operation or its blobs hold names
// the value.
func TestRecorderKeepsWithheldValuesOutOfTheRecord(t *testing.T) {
	a, root, rec := redactingProject(t, "rules")
	ctx := t.Context()
	_, err := rec.Record(ctx, falconEdit("src/intro.en.json"))
	require.NoError(t, err)

	ops := editOps(t, a, root)
	require.Len(t, ops, 1)
	assert.NotContains(t, string(ops[0].Payload), "Falcon")
	var e projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
	assert.Empty(t, e.ChangeSet, "the change set as sent is left out")
	assert.Equal(t, "Say when [REDACTED:Product] ships", e.Note)
	require.Len(t, e.Transitions, 1)
	tr := e.Transitions[0]
	require.NotEmpty(t, tr.RunsBefore)
	require.NotEmpty(t, tr.RunsAfter)

	ws := boundWorkspace(t, a, root)
	refs := workspace.BlobRefs(ops[0])
	require.Len(t, refs, 2)
	for _, ref := range refs {
		blob, err := ws.Blob(ctx, ref)
		require.NoError(t, err)
		assert.NotContains(t, string(blob), "Falcon", "blob %s", ref)
		assert.Contains(t, string(blob), "REDACTED", "the kept runs hold the placeholder")
	}

	vault, err := os.ReadFile(project.LayoutAt(root).RedactionVaultPath())
	require.NoError(t, err)
	assert.Contains(t, string(vault), "Falcon", "the original is kept in the project vault")
}

// TestRecorderUnderEntityDetectionKeepsNoRuns: entity detection needs the
// annotations of a read, which the runs of a record do not carry, so a
// project that detects entities records revisions and hashes only.
func TestRecorderUnderEntityDetectionKeepsNoRuns(t *testing.T) {
	a, root, rec := redactingProject(t, "rules", "entities")
	_, err := rec.Record(t.Context(), falconEdit("src/intro.en.json"))
	require.NoError(t, err)
	ops := editOps(t, a, root)
	require.Len(t, ops, 1)
	assert.NotContains(t, string(ops[0].Payload), "Falcon")
	assert.Empty(t, workspace.BlobRefs(ops[0]), "nothing kept, nothing in a blob")
	var e projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
	assert.Empty(t, e.Note)
	require.Len(t, e.Transitions, 1)
	assert.Empty(t, e.Transitions[0].RunsBefore+e.Transitions[0].RunsAfter)
}
