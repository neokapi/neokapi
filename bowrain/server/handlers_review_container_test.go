package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// containerProject is a French project with a React component's strings
// beside another component's, and a markdown page with two sections.
func containerProject(t *testing.T, s *Server, wsID string) (string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	proj := &platstore.Project{
		Name: "Container", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
		Properties:      map[string]string{"translate_after": "none"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))

	jsx := func(id, comp, element, src, tgt string) *model.Block {
		b := pendingFrBlock(id, src, tgt)
		b.Properties = map[string]string{
			"component": comp, "element": element, "jsxPath": element,
			"file": "src/components/" + comp + ".tsx", "hash": "h-" + id,
		}
		return b
	}
	heading := func(id, level, src, tgt string) *model.Block {
		b := establishedFrBlock(id, src, tgt)
		b.Type = "heading"
		b.Properties = map[string]string{"level": level}
		return b
	}
	items := map[string][]*model.Block{
		"OfflineLaunch.kbf.json": {
			jsx("a1", "Header", "h1", "Bowrain", "Bowrain"),
			jsx("o1", "OfflineLaunch", "h2", "Can't reach your server", "Impossible de joindre le serveur"),
			jsx("o2", "OfflineLaunch", "p", "Bowrain keeps trying.", "Bowrain réessaie."),
			jsx("o3", "OfflineLaunch", "Button", "Try again", "Réessayer"),
			jsx("z1", "Footer", "p", "Made in Oslo", "Fait à Oslo"),
		},
		"guide.md": {
			heading("h1", "1", "Guide", "Guide"),
			pendingFrBlock("p1", "Intro text.", "Texte d'intro."),
			heading("h2", "2", "Install", "Installer"),
			pendingFrBlock("p2", "Run the installer.", "Lancez l'installateur."),
			pendingFrBlock("p3", "Restart.", "Redémarrez."),
			heading("h3", "2", "Use", "Utiliser"),
			pendingFrBlock("p4", "Open the app.", "Ouvrez l'app."),
		},
	}
	ids := map[string]string{}
	for name, blocks := range items {
		require.NoError(t, s.ContentStore.StoreItem(ctx, proj.ID, "main", &platstore.Item{Name: name, Format: "txt", ItemType: "file"}))
		require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", name, blocks))
		stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: name})
		require.NoError(t, err)
		byText := map[string]string{}
		for _, sb := range stored {
			key := sb.SourceID
			if key == "" {
				key = sb.Block.ID
			}
			byText[sb.Block.SourceText()] = key
			ids[sb.Block.SourceText()] = sb.Block.ID
		}
		// A push declares the reading order; the test does the same, keyed as
		// the store keys a row.
		keys := make([]string, 0, len(blocks))
		for _, b := range blocks {
			keys = append(keys, byText[b.SourceText()])
		}
		require.NoError(t, s.ContentStore.SetBlockOrder(ctx, proj.ID, "main", name, keys))
	}
	return proj.ID, ids
}

func readContainer(t *testing.T, s *Server, wsID, projID, bid string) reviewContainerResponse {
	t.Helper()
	rec := deckCall(t, s, s.HandleGetReviewContainer, wsID, projID, bid, "u-reviewer", testReviewer, nil, url.Values{"target_locale": {"fr"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewContainerResponse](t, rec)
}

func unitTexts(c reviewContainerResponse) []string {
	var out []string
	for _, u := range c.Units {
		out = append(out, model.RunsDisplayText(u.SourceRuns))
	}
	return out
}

func TestReviewContainer_AComponentsStrings(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)

	got := readContainer(t, s, wsID, projID, ids["Bowrain keeps trying."])
	assert.Equal(t, "component", got.Kind)
	assert.Equal(t, "OfflineLaunch", got.Title)
	assert.Equal(t, ids["Bowrain keeps trying."], got.Focus)
	assert.Equal(t, []string{"Can't reach your server", "Bowrain keeps trying.", "Try again"}, unitTexts(got),
		"only the component's strings, in order")
	assert.Equal(t, []string{"heading", "paragraph", "button"}, []string{got.Units[0].Role, got.Units[1].Role, got.Units[2].Role})
	assert.Equal(t, 2, got.Units[0].Level)
	assert.True(t, got.Units[1].Waiting)
	assert.NotEmpty(t, got.Units[1].TargetRevision)
	assert.NotEmpty(t, got.Units[1].SourceRevision)
	assert.Equal(t, "Bowrain réessaie.", model.RunsDisplayText(got.Units[1].TargetRuns))
}

func TestReviewContainer_ADocumentsSection(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)

	got := readContainer(t, s, wsID, projID, ids["Restart."])
	assert.Equal(t, "section", got.Kind)
	assert.Equal(t, "Install", got.Title)
	assert.Equal(t, []string{"Install", "Run the installer.", "Restart."}, unitTexts(got),
		"from its heading to the next heading of the same level")
	assert.Equal(t, "heading", got.Units[0].Role)
	assert.False(t, got.Units[0].Waiting, "an approved heading is not waiting")

	// A level-1 section runs over its level-2 sections.
	top := readContainer(t, s, wsID, projID, ids["Intro text."])
	assert.Equal(t, "Guide", top.Title)
	assert.Len(t, top.Units, 7)

	// A heading is the start of its own section.
	head := readContainer(t, s, wsID, projID, ids["Use"])
	assert.Equal(t, []string{"Use", "Open the app."}, unitTexts(head))
}

func TestReviewContainer_NeedsATargetLocale(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)
	rec := deckCall(t, s, s.HandleGetReviewContainer, wsID, projID, ids["Restart."], "u-reviewer", testReviewer, nil, url.Values{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReviewContainer_NamesTheLivePreviewOfAComponent(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)
	ctx := t.Context()
	col := &platstore.Collection{
		ProjectID: projID, Name: "app", Stream: "main",
		PreviewKind: "storybook", PreviewURL: "https://example.org/storybook/",
	}
	require.NoError(t, s.ContentStore.CreateCollection(ctx, col))
	require.NoError(t, s.ContentStore.StoreItem(ctx, projID, "main", &platstore.Item{
		Name: "OfflineLaunch.kbf.json", Format: "txt", ItemType: "file", CollectionID: col.ID,
	}))

	got := readContainer(t, s, wsID, projID, ids["Bowrain keeps trying."])
	assert.Equal(t, col.ID, got.CollectionID)
	require.NotNil(t, got.Preview)
	assert.Equal(t, containerPreview{
		Kind: "storybook", URL: "https://example.org/storybook/", SourceFile: "src/components/OfflineLaunch.tsx",
	}, *got.Preview)
	require.Len(t, got.Units, 3)
	assert.Equal(t, "h-o2", got.Units[1].MessageKey)
	assert.Equal(t, "Bowrain réessaie.", got.Units[1].MessageText)

	// A document's section has no component to run.
	doc := readContainer(t, s, wsID, projID, ids["Restart."])
	assert.Nil(t, doc.Preview)
	assert.Empty(t, doc.Units[1].MessageKey)
}
