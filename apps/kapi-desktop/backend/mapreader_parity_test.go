package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
	"github.com/neokapi/neokapi/terms"
)

// The Map and Reader parity fixture: the sample samples/audience-context as
// the desktop reads it, and the Map and the Reader the desktop draws for it.
// bowrain/server's Map and Reader test stores the same blocks under the same
// voice and terms, and holds the server's answers to the same files.
//
//	pushed.json        the collections, the items with their blocks, and the
//	                   voice and terms at each collection's point
//	map-summary.json   the Map, after a checks run over the whole project
//	reader-file.json   the Reader of readerParityFile
//
// The fields the server cannot know are blanked in both files: the paths on
// disk, and when the files were read and checked. Regenerate after an
// intended change with
//
//	KAPI_UPDATE_GOLDEN=1 go test ./backend -run TestMapReaderParity
var mapParityDir = filepath.Join("..", "..", "..", "host", "screens", "testdata", "map-parity")

// readerParityFile is the file the fixture's Reader reads.
const readerParityFile = "site/locales/en/child.json"

// mapParityKey is the fixture project's key.
const mapParityKey workspace.ProjectKey = "prj_mapparityaaaaaaaaaaa"

// recordMapParityLog records the fixture's ledger through the calls kapi
// makes and writes the operations it added to path as one segment: an
// assistant's term rule a person kept, which the terms store already holds,
// and a note a person kept. Regenerate it with KAPI_UPDATE_MAP_LOG=1.
func recordMapParityLog(t *testing.T, app *host.App, recipe, path string) {
	t.Helper()
	ctx := t.Context()
	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	before, err := ws.Select(ctx, workspace.OpQuery{Project: mapParityKey})
	require.NoError(t, err)
	var after int64
	for _, op := range before {
		after = max(after, op.Seq)
	}
	person := contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"}
	term, err := app.RecordContextObservation(ctx, host.ContextObserveRequest{
		Project: recipe, Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "codex@build-01", Session: "s-map"},
		Term: "video appointment", InsteadOf: []string{"video call"},
		Evidence: []contextop.Evidence{{Path: readerParityFile, Quote: "Ask a trusted adult to book a video call with you."}},
	})
	require.NoError(t, err)
	_, err = app.DecideRule(ctx, host.RuleDecideRequest{Actor: person, Project: recipe, ID: term.ID, Answer: host.RuleAnswerKeep})
	require.NoError(t, err)
	note, err := app.RecordContextObservation(ctx, host.ContextObserveRequest{
		Project: recipe, Actor: person, Text: "Every page names the support line as the way to get help.",
	})
	require.NoError(t, err)
	_, err = app.DecideRule(ctx, host.RuleDecideRequest{Actor: person, Project: recipe, ID: note.ID, Answer: host.RuleAnswerKeep})
	require.NoError(t, err)

	ops, err := ws.Select(ctx, workspace.OpQuery{Project: mapParityKey, After: after})
	require.NoError(t, err)
	require.NotEmpty(t, ops)
	data, err := workspace.EncodeSegment(ops)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// MapParityPushed is pushed.json.
type MapParityPushed struct {
	SourceLanguage string           `json:"source_language"`
	Collections    []MapParityPoint `json:"collections"`
	Items          []MapParityItem  `json:"items"`
	Concepts       []terms.Concept  `json:"concepts"`
}

// MapParityPoint is one collection with the voice at its point.
type MapParityPoint struct {
	Name string `json:"name"`
	// Coordinates are the collection's point, as a push stores them.
	Coordinates map[string]string         `json:"coordinates"`
	Voice       *coreprofile.VoiceProfile `json:"voice"`
	// CarriedFrom and Carried are the word rules the voice's file carries,
	// which the profile's JSON does not hold.
	CarriedFrom string                 `json:"carried_from,omitempty"`
	Carried     []coreprofile.TermRule `json:"carried,omitempty"`
}

// MapParityItem is one file and its source blocks.
type MapParityItem struct {
	Name       string           `json:"name"`
	Format     string           `json:"format"`
	Collection string           `json:"collection"`
	Blocks     []MapParityBlock `json:"blocks"`
}

// MapParityBlock is one block as the check reads it.
type MapParityBlock struct {
	ID           string      `json:"id"`
	Translatable bool        `json:"translatable"`
	Runs         []model.Run `json:"runs"`
}

func TestMapReaderParity(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "..", "samples", "audience-context"))))
	recipe := filepath.Join(dir, "kapi.yaml")
	// The sample passes its checks; one sentence breaks a word rule and the
	// voice's prohibited wording, so the comparison has findings to make.
	child := filepath.Join(dir, filepath.FromSlash(readerParityFile))
	data, err := os.ReadFile(child)
	require.NoError(t, err)
	broken := strings.Replace(string(data), "Ask a trusted adult to book a video appointment with you.",
		"Ask a trusted adult to book a video call with you. It is risk-free.", 1)
	require.NotEqual(t, string(data), broken)
	require.NoError(t, os.WriteFile(child, []byte(broken), 0o644))
	// A fixed project key, so the recorded ledger reads back as this
	// project's.
	recipeData, err := os.ReadFile(recipe)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipe, []byte(strings.Replace(string(recipeData),
		"version: v1\n", "version: v1\nid: "+string(mapParityKey)+"\n", 1)), 0o644))

	// The sample's context read into the workspace the app reads, as `kapi
	// store import ./context` reads it.
	importer := &host.App{}
	importer.InitRegistries()
	importer.ActAs(contextop.Actor{Kind: contextop.ActorPerson})
	_, err = importer.ProjectDB(t.Context(), dir)
	require.NoError(t, err)
	_, err = importer.ImportProjectContext(t.Context(), recipe, host.ContextImportRequest{Dir: filepath.Join(dir, "context")})
	require.NoError(t, err)

	// The ledger: a kept term rule the terms store also holds, and a kept
	// note, so the Map and the Reader count each rule once.
	logPath := filepath.Join(mapParityDir, "log.jsonl")
	if os.Getenv("KAPI_UPDATE_MAP_LOG") != "" {
		recordMapParityLog(t, importer, recipe, logPath)
	} else {
		segment, err := os.ReadFile(logPath)
		require.NoError(t, err)
		ops, err := workspace.DecodeSegment(segment, mapParityKey)
		require.NoError(t, err)
		ws, err := importer.Workspace(t.Context())
		require.NoError(t, err)
		_, err = ws.Record(t.Context(), ops...)
		require.NoError(t, err)
	}
	importer.Shutdown()

	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, err = app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	_, err = app.RunChecks(tab.ID, ProjectFilter{})
	require.NoError(t, err)

	summary, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, summary.Findings)
	require.Positive(t, *summary.Findings, "the sample holds findings to compare")
	reader, err := app.ReaderFile(tab.ID, readerParityFile)
	require.NoError(t, err)
	require.Positive(t, reader.Findings)

	// What a push of the project carries, with the voice and terms the
	// desktop's checks ran under at each collection's point.
	op := app.getOpenProject(tab.ID)
	points := app.newPointResolver(op, false)
	pushed := MapParityPushed{SourceLanguage: string(op.Project.Defaults.SourceLanguage)}
	var concepts []terms.Concept
	for _, row := range summary.FileRows {
		profile := points.at(t.Context(), row.Collection, row.Relative)
		require.NoError(t, points.err)
		require.NotNil(t, profile, "a voice governs %s", row.Relative)
		// The voice as stored, without the times this run stored it at.
		voice := *profile.VoiceOnly()
		voice.CreatedAt, voice.UpdatedAt = time.Time{}, time.Time{}
		at := project.GovernancePoint{Collection: row.Collection, Path: row.Relative}
		rc, err := op.Project.ResolveGovernanceFor(at)
		require.NoError(t, err)
		point := MapParityPoint{Name: row.Collection, Coordinates: op.Project.CoordinatesAt(at, rc), Voice: &voice}
		if carried := profile.CarriedTerms(); len(carried.Rules) > 0 {
			point.CarriedFrom, point.Carried = carried.From, carried.Rules
		}
		pushed.Collections = append(pushed.Collections, point)

		tb := points.termsAt(t.Context(), row.Collection, row.Relative)
		require.NoError(t, points.err)
		require.NotNil(t, tb, "terms govern %s", row.Relative)
		list, err := tb.Concepts(t.Context())
		require.NoError(t, err)
		if concepts == nil {
			concepts = list
		}
		assert.Equal(t, concepts, list, "one terms store governs every file")

		var blocks []*model.Block
		_, err = app.runChecks(tab.ID, ProjectFilter{}, checkScope{file: row.Relative, blocks: &blocks})
		require.NoError(t, err)
		item := MapParityItem{Name: row.Relative, Format: row.Format, Collection: row.Collection}
		for _, b := range blocks {
			item.Blocks = append(item.Blocks, MapParityBlock{ID: b.ID, Translatable: b.Translatable, Runs: b.SourceRuns()})
		}
		pushed.Items = append(pushed.Items, item)
	}
	for i := range concepts {
		concepts[i].CreatedAt, concepts[i].UpdatedAt = time.Time{}, time.Time{}
	}
	pushed.Concepts = concepts

	golden(t, filepath.Join(mapParityDir, "pushed.json"), pushed)
	golden(t, filepath.Join(mapParityDir, "map-summary.json"), MapParityView(*summary))
	golden(t, filepath.Join(mapParityDir, "reader-file.json"), ReaderParityView(*reader))
}

// MapParityView is the Map without the fields the server cannot know: the
// paths on disk, the workspace key of a project the server has not bound,
// and when the files were read and checked.
func MapParityView(m screens.MapSummaryDTO) screens.MapSummaryDTO {
	m.ProjectKey, m.ReadAt, m.CheckedAt = "", "", ""
	rows := make([]screens.MapFileDTO, len(m.FileRows))
	for i, r := range m.FileRows {
		r.Path = ""
		rows[i] = r
	}
	m.FileRows = rows
	return m
}

// ReaderParityView is the Reader without the path on disk.
func ReaderParityView(r screens.ReaderFileDTO) screens.ReaderFileDTO {
	r.Abs = ""
	return r
}

// golden holds value's JSON to the file at path, or writes it there under
// KAPI_UPDATE_GOLDEN.
func golden(t *testing.T, path string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')
	if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "fixture missing; run with KAPI_UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got), "%s differs from what the desktop reads; regenerate with KAPI_UPDATE_GOLDEN=1 if the change is intended", path)
}
