package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/ai/tools"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coreproject "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// These tests pin all eight context-bearing fields of the interactive translate
// config: the voice profile, the term rules, the protected terms, the content
// memory, the point, the reuse setting, and the neighbourhood policy with its
// window. They also pin the point as a coordinate the corpus reads rather than
// a string nobody looks at: the prior version seeded at this collection's point
// comes back, and the same unit approved at another point does not.

// translatePointFixture builds a project whose item sits in a collection at a
// declared point, governed by a voice profile bound on that collection, with
// terminology, protected terms, a content memory and three blocks in document
// order.
func translatePointFixture(t *testing.T) (platstore.ContentStore, editorVoiceContext, *platstore.Project, *memory.InMemoryStore) {
	t.Helper()
	ctx := t.Context()

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)

	proj := &platstore.Project{
		ID:                    "p1",
		Name:                  "Proj",
		WorkspaceID:           "ws-1",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		Properties:            map[string]string{"dnt_terms": "Kapi"},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))

	col := &platstore.Collection{
		ProjectID: proj.ID,
		Name:      "docs",
		Kind:      platstore.CollectionUploaded,
		ItemLabel: "page",
		Stream:    "main",
		Context: map[string]string{
			coreproject.ProductAxis: "acme",
			coreproject.ChannelAxis: "web",
		},
		ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: "bp-docs"},
	}
	require.NoError(t, cs.CreateCollection(ctx, col))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "hello.txt", Format: "txt", ItemType: "file", CollectionID: col.ID,
	}))

	// Three blocks, in document order: the middle one has neighbours either
	// side, which is what the context window is measured in.
	var blocks []*model.Block
	for _, text := range []string{"Save", "the software is ready", "Cancel"} {
		b := &model.Block{ID: "b-" + text, Translatable: true}
		b.SetSourceText(text)
		blocks = append(blocks, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "hello.txt", blocks))

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "software", Locale: "en", Status: model.TermPreferred},
			{Text: "logiciel", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	tm := memory.NewInMemoryStore()

	wsStores := newWorkspaceStores()
	wsStores.termsFactory = func() terms.Store { return &testTermStore{tb} }
	wsStores.memoryFactory = func() memory.Store { return tm }

	voiceCtx := editorVoiceContext{
		Voice: &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{
			"bp-docs": {ID: "bp-docs", Name: "Docs Voice"},
			"bp-ws":   {ID: "bp-ws", Name: "Workspace Voice"},
		}},
		// The workspace binds a different voice, so a profile resolved at the
		// collection proves the ladder reached the item's own point.
		WorkspaceDefault: editorFakeWorkspaceDefault{id: "bp-ws"},
		Stores:           wsStores,
	}
	return cs, voiceCtx, proj, tm
}

// TestEditorTranslateConfigCarriesEveryContextField asserts the whole governing
// context reaches the tool: the eight fields the framework fills on a project
// run, with the values this project actually declares.
func TestEditorTranslateConfigCarriesEveryContextField(t *testing.T) {
	cs, voiceCtx, proj, _ := translatePointFixture(t)

	cfg := editorTranslateConfig(t.Context(), cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"})

	require.NotNil(t, cfg.Profile)
	assert.Equal(t, "bp-docs", cfg.Profile.ID,
		"the voice bound on the item's own collection governs, not the workspace default")
	assert.Equal(t, []coreprofile.TermRule{{Term: "software", Replacement: "logiciel"}}, cfg.TermRules)
	assert.Equal(t, []string{"Kapi"}, cfg.DNT)
	assert.NotNil(t, cfg.Memory, "the workspace content memory answers what a block said before")
	assert.Equal(t, memory.NewPoint("acme", "web", "docs"), cfg.Point,
		"the point is the three rungs kapi writes beside every answer it learns")
	assert.Equal(t, tools.ReusePrior, cfg.Reuse)
	assert.Equal(t, tools.ContextNeighbours, cfg.Context)
	assert.Equal(t, tools.DefaultContextWindow, cfg.ContextWindow)

	assert.Equal(t, model.LocaleID("en"), cfg.SourceLocale)
	assert.Equal(t, model.LocaleID("fr"), cfg.TargetLocale)
}

// TestEditorTranslateConfigMemoryAnswersAtThePoint proves the content memory
// and the point work together rather than being two strings on a struct: a
// block's prior answer is returned when it was approved where this content
// sits, and withheld when it was approved somewhere else.
func TestEditorTranslateConfigMemoryAnswersAtThePoint(t *testing.T) {
	ctx := t.Context()
	cs, voiceCtx, proj, tm := translatePointFixture(t)

	cfg := editorTranslateConfig(ctx, cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"})
	require.NotNil(t, cfg.Memory)

	// What the tool will compute for this profile + these rules, and therefore
	// what a prior answer has to have been approved under to be offered.
	_, _, fingerprint := coreprofile.GovernanceContext(cfg.Profile, cfg.TermRules)
	require.NotEmpty(t, fingerprint)

	now := time.Now()
	seed := func(id, point, target string) {
		require.NoError(t, tm.Add(ctx, memory.Entry{
			ID:   id,
			Unit: "u-ready",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "the software is ready"}}},
				"fr": {{Text: &model.TextRun{Text: target}}},
			},
			HintSrcLang: "en",
			Point:       point,
			Origins:     []memory.Origin{{Source: "approval", AddedAt: now, ContextFingerprint: fingerprint}},
			CreatedAt:   now,
			UpdatedAt:   now,
		}))
	}
	seed("here", cfg.Point, "le logiciel est prêt")
	seed("elsewhere", memory.NewPoint("acme", "print", "brochures"), "le logiciel est disponible")

	got, ok := cfg.Memory.PriorVersion(ctx, corememory.VersionRequest{
		Unit:       "u-ready",
		Point:      cfg.Point,
		Source:     "en",
		Target:     "fr",
		GovernedBy: fingerprint,
	})
	require.True(t, ok, "the answer approved at this point is offered as reference")
	assert.Equal(t, "le logiciel est prêt", got.Target)

	_, ok = cfg.Memory.PriorVersion(ctx, corememory.VersionRequest{
		Unit:       "u-ready",
		Point:      memory.NewPoint("acme", "mobile", "app"),
		Source:     "en",
		Target:     "fr",
		GovernedBy: fingerprint,
	})
	assert.False(t, ok, "an answer approved elsewhere must not steer this content")
}

// TestEditorAndWorkerAssembleOneConfig pins the unification: the editor and the
// worker read the same stores and must produce the same governing context, so a
// translation a person starts is governed exactly as a queued one is.
func TestEditorAndWorkerAssembleOneConfig(t *testing.T) {
	ctx := t.Context()
	cs, voiceCtx, proj, tm := translatePointFixture(t)

	fromEditor := editorTranslateConfig(ctx, cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"})

	fromWorker := jobs.BuildTranslateConfig(ctx, jobs.TranslateBinding{
		Store:            cs,
		Voice:            voiceCtx.Voice,
		WorkspaceDefault: voiceCtx.WorkspaceDefault,
		Terms:            editorTerms(ctx, voiceCtx, "acme"),
		Memory:           tm,
		Project:          proj,
		WorkspaceID:      "ws-1",
		ProjectID:        "p1",
		Stream:           "main",
		ItemName:         "hello.txt",
		TargetLocale:     "fr",
	})

	assert.Equal(t, fromEditor.Profile, fromWorker.Profile)
	assert.Equal(t, fromEditor.TermRules, fromWorker.TermRules)
	assert.Equal(t, fromEditor.DNT, fromWorker.DNT)
	assert.Equal(t, fromEditor.Point, fromWorker.Point)
	assert.Equal(t, fromEditor.Reuse, fromWorker.Reuse)
	assert.Equal(t, fromEditor.Context, fromWorker.Context)
	assert.Equal(t, fromEditor.ContextWindow, fromWorker.ContextWindow)
	assert.NotNil(t, fromWorker.Memory)
}

// TestCollectionPointFallsBackToTheGovernanceKey covers a collection stored
// before the coordinates travelled on the push wire: the channel is read from
// the key core/profile writes on the collection, and a collection that declares
// nothing at all sits at the project's default point.
func TestCollectionPointFallsBackToTheGovernanceKey(t *testing.T) {
	assert.Equal(t, memory.NewPoint("acme", "web", "docs"), jobs.CollectionPoint(&platstore.Collection{
		Name:            "docs",
		Context:         map[string]string{coreproject.ProductAxis: "acme"},
		ConnectorConfig: map[string]string{coreprofile.PropertyChannel: "web"},
	}))
	assert.Equal(t, memory.NewPoint("", "", "docs"), jobs.CollectionPoint(&platstore.Collection{Name: "docs"}))
	assert.Empty(t, jobs.CollectionPoint(nil), "no collection is the project's default point")
}

// TestPlatformConfigMatchesAFlowRunConfig answers the question the issue asks:
// does a server translation of a unit resolve the configuration a flow run of
// the same unit resolves?
//
// The flow carries its bindings as a config map and applies it through
// core/schema (host/flow.go: applyBindings writes "profile", "point" and
// "term_rules"; grantMemory writes the content memory under its own key; the
// recipe writes "context" and "contextWindow"). Building that map for this
// unit's governance and applying it the way the flow does gives the config a
// flow run would hand the tool, and the platform must hand it the same one.
func TestPlatformConfigMatchesAFlowRunConfig(t *testing.T) {
	ctx := t.Context()
	cs, voiceCtx, proj, _ := translatePointFixture(t)

	platform := editorTranslateConfig(ctx, cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"})

	var flow tools.AITranslateConfig
	require.NoError(t, schema.ApplyConfig(map[string]any{
		"point":         memory.NewPoint("acme", "web", "docs"),
		"term_rules":    []coreprofile.TermRule{{Term: "software", Replacement: "logiciel"}},
		"dnt":           []string{"Kapi"},
		"context":       tools.ContextNeighbours,
		"contextWindow": tools.DefaultContextWindow,
		"reuse":         tools.ReusePrior,
	}, &flow))

	assert.Equal(t, flow.Point, platform.Point)
	assert.Equal(t, flow.TermRules, platform.TermRules)
	assert.Equal(t, flow.DNT, platform.DNT)
	assert.Equal(t, flow.Context, platform.Context)
	assert.Equal(t, flow.ContextWindow, platform.ContextWindow)
	assert.Equal(t, flow.Reuse, platform.Reuse)

	// The two the flow injects rather than serializes, because each is a live
	// handle: the platform supplies both.
	assert.NotNil(t, platform.Profile)
	assert.NotNil(t, platform.Memory)
}
