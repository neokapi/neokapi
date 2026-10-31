package host

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/credentials/providerenv"
	"github.com/neokapi/neokapi/core/project"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The deeper look asks the demo provider, which answers term-extract with the
// capitalised names in a text and voice-infer with counts over the corpus, so
// every proposal below is deterministic.

// deeperLookFiles name a product (Harbor Help, a compound with spellings to
// avoid), a place in it (Dockyard, one word with none) and address the reader
// as you, with one contraction.
var deeperLookFiles = map[string]string{
	"docs/a.md": "# Harbor Help\n\nHarbor Help answers your questions. You can ask it anything.\n\n" +
		"Open the Dockyard to see your boats. You can't miss it.\n\n" +
		"The Dockyard lists every berth.\n\nYou reach Harbor Help from the Dockyard.\n",
}

// useDemoProvider makes the demo provider the configured one, and clears every
// provider key the environment could hold, so nothing on this machine is
// asked.
func useDemoProvider(t *testing.T) {
	t.Helper()
	clearProviderEnv(t)
	t.Setenv("KAPI_AI_PROVIDER", string(aiprovider.Demo))
	t.Setenv("KAPI_AI_MODEL", "")
	aiprovider.SetDemoNoticeWriter(nopWriter{})
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, vars := range providerenv.Vars {
		for _, v := range vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// deeperOf returns the proposals of one kind.
func deeperOf(res DeeperLookResult, kind string) []SuggestedFinding {
	var out []SuggestedFinding
	for _, s := range res.Suggestions {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

// measureOf returns the voice proposal about one measure.
func measureOf(t *testing.T, res DeeperLookResult, measure string) SuggestedFinding {
	t.Helper()
	for _, s := range deeperOf(res, SuggestKindVoice) {
		if s.Measure == measure {
			return s
		}
	}
	t.Fatalf("no proposal about %s in %+v", measure, res.Suggestions)
	return SuggestedFinding{}
}

func TestDeeperLook_FilesTermsAndVoiceAsKapiWithTheModel(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperfiles", "", deeperLookFiles)

	res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Equal(t, "demo", res.Provider)
	assert.Equal(t, aiprovider.DemoModelName, res.Model)
	assert.Equal(t, 1, res.Files)
	assert.Equal(t, 5, res.Blocks)
	assert.Equal(t, 5, res.Asked)
	require.NotNil(t, res.Usage, "the demo provider reports a token count")
	assert.Positive(t, res.Usage.TotalTokens())

	terms := deeperOf(res, SuggestKindTerm)
	require.Len(t, terms, 2, "%+v", terms)
	// Both are written in three blocks, so they come in the order of their
	// letters.
	assert.Equal(t, `The files use the term "Dockyard": ⟦demo⟧ a capitalised name in the text.`, terms[0].Sentence)
	assert.Equal(t, "Write Harbor Help, not Harbor-Help or HarborHelp.", terms[1].Sentence)
	for _, term := range terms {
		assert.Equal(t, SuggestFiled, term.Outcome, term.Sentence)
		assert.Equal(t, 3, term.Blocks, term.Sentence)
		assert.Equal(t, []string{"docs/a.md"}, term.Files)
		require.NotNil(t, term.Quote)
		assert.Equal(t, "docs/a.md", term.Quote.Path)
	}

	rule, err := app.ContextSubject(ctx, recipeOf(root), terms[1].ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.Actor{Kind: contextop.ActorTool, Name: aiprovider.DemoModelName}, rule.Actor)
	assert.Equal(t, contextop.KindObserve, rule.Kind)
	assert.Equal(t, contextop.StatusSuggested, rule.Status, "it advises and fails nothing until a person keeps it")
	got, ok := rule.Rule()
	require.True(t, ok)
	assert.Equal(t, "Harbor Help", got.Replacement)
	assert.Equal(t, "⟦demo⟧ a capitalised name in the text", got.Note)
	require.NotEmpty(t, rule.Evidence)
	assert.Equal(t, "Harbor Help", rule.Evidence[0].Quote)

	note, err := app.ContextSubject(ctx, recipeOf(root), terms[0].ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.SubjectNote, note.Subject.Kind)

	assert.Equal(t, "second", measureOf(t, res, "person_pov").Value)
	assert.Equal(t, "The files address the reader as you (person_pov: second).", measureOf(t, res, "person_pov").Sentence)
	assert.Equal(t, "The files use contractions in places (contractions: sometimes).", measureOf(t, res, "contractions").Sentence)
	assert.Equal(t, "The files keep sentences short (sentence_length: short).", measureOf(t, res, "sentence_length").Sentence)
	assert.Equal(t, "The files write in the active voice (active_voice: true).", measureOf(t, res, "active_voice").Sentence)
	assert.Equal(t, "The files read as casual (formality: casual).", measureOf(t, res, "formality").Sentence)
	contraction := measureOf(t, res, "contractions")
	require.NotNil(t, contraction.Quote)
	assert.Equal(t, "Open the Dockyard to see your boats. You can't miss it.", contraction.Quote.Quote,
		"the quote shows a block with a contraction")
	assert.Equal(t, 1, contraction.Blocks)
	length := measureOf(t, res, "sentence_length")
	require.NotNil(t, length.Quote)
	assert.Equal(t, "Harbor Help answers your questions. You can ask it anything.", length.Quote.Quote,
		"a measure of the whole text quotes a sentence, not the heading")

	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	var text strings.Builder
	require.NoError(t, digest.FormatText(&text))
	assert.Contains(t, text.String(), "noticed by kapi "+aiprovider.DemoModelName+"\n", "the queue names the model beside kapi")

	var printed strings.Builder
	require.NoError(t, res.FormatText(&printed))
	assert.Contains(t, printed.String(), "Write Harbor Help, not Harbor-Help or HarborHelp. (3 blocks in 1 file)")
	assert.Regexp(t, `(?m)^demo-stub read 5 blocks in 1 file \(\d+ tokens\):\n`, printed.String(),
		"the model is named before the lines it led to")
	assert.Contains(t, printed.String(), "\nDecide with kapi context review.\n")
}

func TestDeeperLook_RerunFilesNothing(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperrerun", "", deeperLookFiles)

	first, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, first.Suggestions, 7)
	assert.Equal(t, 7, first.FiledCount())
	before, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)

	second, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, second.Suggestions, 7)
	assert.Zero(t, second.FiledCount())
	for i, s := range second.Suggestions {
		assert.Equal(t, SuggestUnchanged, s.Outcome, s.Sentence)
		assert.Equal(t, first.Suggestions[i].ID, s.ID)
	}
	after, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Len(t, after.Operations, len(before.Operations), "a second run over the same files records nothing")

	var printed strings.Builder
	require.NoError(t, second.FormatText(&printed))
	assert.Regexp(t, `(?m)^Unchanged\s+\S+\s+Write Harbor Help`, printed.String())
}

func TestDeeperLook_DroppedStaysDropped(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperdropped", "", deeperLookFiles)

	first, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	terms := deeperOf(first, SuggestKindTerm)
	require.NotEmpty(t, terms)
	_, err = app.DecideContextReview(ctx, ContextReviewRequest{
		Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), Drop: []string{terms[0].ID},
	})
	require.NoError(t, err)

	again, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	dropped := deeperOf(again, SuggestKindTerm)[0]
	assert.Equal(t, SuggestDropped, dropped.Outcome)
	assert.Equal(t, terms[0].ID, dropped.ID)
	assert.Zero(t, again.FiledCount())
}

func TestDeeperLook_DoesNotRepeatThePassOverTheFiles(t *testing.T) {
	t.Run("the pass first", func(t *testing.T) {
		useDemoProvider(t)
		app, _ := contextOpsApp(t)
		ctx := context.Background()
		root := suggestProject(t, "deeperafterpass", "", deeperLookFiles)

		pass, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		require.Len(t, suggestionsOf(pass, SuggestKindAddress), 1)

		res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		assert.Equal(t, SuggestKnown, measureOf(t, res, "person_pov").Outcome,
			"the pass already filed the reader addressed as you")
		assert.Empty(t, measureOf(t, res, "person_pov").ID)
	})

	t.Run("the deeper look first", func(t *testing.T) {
		useDemoProvider(t)
		app, _ := contextOpsApp(t)
		ctx := context.Background()
		root := suggestProject(t, "deeperbeforepass", "", deeperLookFiles)

		_, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		pass, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		address := suggestionsOf(pass, SuggestKindAddress)
		require.Len(t, address, 1)
		assert.Equal(t, SuggestKnown, address[0].Outcome, "the deeper look already filed it")
	})

	t.Run("a spelling the pass filed", func(t *testing.T) {
		useDemoProvider(t)
		app, _ := contextOpsApp(t)
		ctx := context.Background()
		root := suggestProject(t, "deeperspelling", "", map[string]string{
			"docs/a.md": "Open the Dockyard today.\n\nThe Dockyard is new.\n\nOur Dock yard moved.\n",
		})
		pass, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		require.Len(t, suggestionsOf(pass, SuggestKindSpelling), 1)

		res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		for _, term := range deeperOf(res, SuggestKindTerm) {
			assert.NotEqual(t, SuggestFiled, term.Outcome, "%s: the pass's spelling covers the word", term.Sentence)
		}
	})
}

func TestDeeperLook_DryRunRecordsNothing(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperdry", "", deeperLookFiles)

	res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 7)
	assert.NotNil(t, res.Usage, "a dry run still asks the model")
	log, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, log.Operations)

	var printed strings.Builder
	require.NoError(t, res.FormatText(&printed))
	assert.Regexp(t, `(?m)^Would file\s+Write Harbor Help`, printed.String())
	assert.NotContains(t, printed.String(), "Decide with")
}

func TestDeeperLook_NoProviderRefuses(t *testing.T) {
	clearProviderEnv(t)
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "deepernone", "", deeperLookFiles)

	_, err := app.DeeperLook(context.Background(), DeeperLookRequest{Project: recipeOf(root)})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoAIProvider)
	assert.Contains(t, err.Error(), "no AI provider configured")
}

func TestDeeperLook_RecipePresetNamesTheProvider(t *testing.T) {
	clearProviderEnv(t)
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "deeperpreset", "  tools:\n    term-extract:\n      provider: demo\n    voice-infer:\n      provider: demo\n", deeperLookFiles)

	res, err := app.DeeperLook(context.Background(), DeeperLookRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "demo", res.Provider)
	assert.NotEmpty(t, res.Suggestions)
}

func TestDeeperLook_RedactsBeforeTheModelReads(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "deeperredact",
		"  redaction:\n    enabled: true\n    rules: redact.yaml\n    detectors: [rules]\n", deeperLookFiles)
	require.NoError(t, os.WriteFile(filepath.Join(root, "redact.yaml"),
		[]byte("version: v1\nrules:\n  - term: Dockyard\n    category: place\n"), 0o600))

	res, err := app.DeeperLook(context.Background(), DeeperLookRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	terms := deeperOf(res, SuggestKindTerm)
	require.Len(t, terms, 1, "the model never read the redacted name: %+v", terms)
	assert.Equal(t, "Write Harbor Help, not Harbor-Help or HarborHelp.", terms[0].Sentence)
	_, err = os.Stat(project.LayoutAt(root).RedactionVaultPath())
	require.ErrorIs(t, err, fs.ErrNotExist, "a dry run keeps no originals in the project's vault")

	_, err = app.DeeperLook(context.Background(), DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	_, err = os.Stat(project.LayoutAt(root).RedactionVaultPath())
	assert.NoError(t, err, "a run keeps the originals in the project's vault")
}

func TestDeeperLook_AnotherModelsSuggestionStands(t *testing.T) {
	useDemoProvider(t)
	t.Setenv("KAPI_AI_MODEL", "alpha")
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperanothermodel", "", deeperLookFiles)

	first, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Equal(t, "alpha", first.Model)
	was := measureOf(t, first, "contractions")
	require.Equal(t, "sometimes", was.Value)

	// The files lose their one contraction, and another model reads them.
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "a.md"),
		[]byte(strings.Replace(deeperLookFiles["docs/a.md"], "You can't miss it.", "You will find it.", 1)), 0o600))
	t.Setenv("KAPI_AI_MODEL", "beta")
	other, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	got := measureOf(t, other, "contractions")
	assert.Equal(t, SuggestUnchanged, got.Outcome, "only its author withdraws a suggestion")
	assert.Equal(t, was.ID, got.ID)
	assert.Equal(t, was.Sentence, got.Sentence, "the line shows the record that stands, not the new model's reading")
	assert.Equal(t, "sometimes", got.Value)

	// The model that filed it reads the changed files and files it again.
	t.Setenv("KAPI_AI_MODEL", "alpha")
	again, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	updated := measureOf(t, again, "contractions")
	assert.Equal(t, SuggestUpdated, updated.Outcome)
	assert.Equal(t, "The files write without contractions (contractions: never).", updated.Sentence)
}

func TestDeeperLook_DroppedAddressStaysDroppedInBothOrders(t *testing.T) {
	drop := func(t *testing.T, app *App, root, id string) {
		t.Helper()
		_, err := app.DecideContextReview(context.Background(), ContextReviewRequest{
			Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), Drop: []string{id},
		})
		require.NoError(t, err)
	}

	t.Run("the deeper look's note dropped, then the pass", func(t *testing.T) {
		useDemoProvider(t)
		app, _ := contextOpsApp(t)
		ctx := context.Background()
		root := suggestProject(t, "deeperdropaddressfirst", "", deeperLookFiles)

		res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		pov := measureOf(t, res, "person_pov")
		drop(t, app, root, pov.ID)

		pass, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		address := suggestionsOf(pass, SuggestKindAddress)
		require.Len(t, address, 1)
		assert.Equal(t, SuggestDropped, address[0].Outcome)
		assert.Equal(t, pov.ID, address[0].ID)
	})

	t.Run("the pass's note dropped, then the deeper look", func(t *testing.T) {
		useDemoProvider(t)
		app, _ := contextOpsApp(t)
		ctx := context.Background()
		root := suggestProject(t, "deeperdropaddresssecond", "", deeperLookFiles)

		pass, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		address := suggestionsOf(pass, SuggestKindAddress)
		require.Len(t, address, 1)
		drop(t, app, root, address[0].ID)

		res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		pov := measureOf(t, res, "person_pov")
		assert.Equal(t, SuggestDropped, pov.Outcome)
		assert.Equal(t, address[0].ID, pov.ID)
	})
}

func TestDeeperLookChunks_CutsALongBlock(t *testing.T) {
	long := strings.Repeat("berth ", 2000) // 12,000 runes
	c := suggestCorpus{blocks: []suggestBlock{
		{file: "a.md", plain: long},
		{file: "a.md", plain: "The Dockyard lists every berth."},
	}}
	chunks, _, asked := deeperLookChunks(c, nil)
	assert.Equal(t, 2, asked)
	require.Len(t, chunks, 2)
	for _, chunk := range chunks {
		assert.LessOrEqual(t, utf8.RuneCountInString(chunk.SourceText()), deeperLookChunkRunes)
	}
	assert.Equal(t, "The Dockyard lists every berth.", chunks[1].SourceText())
}

func TestDeeperLook_AKeyForAnotherProviderRefusesBeforeFiling(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_API_KEY", "sk-test-not-a-key")
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperotherkey", "", deeperLookFiles)

	_, _, err := app.DeeperLookModel(ctx, recipeOf(root))
	require.ErrorIs(t, err, ErrNoAIProvider, "the run would build anthropic and find no key for it")

	_, err = app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root)})
	require.ErrorIs(t, err, ErrNoAIProvider)
	log, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, log.Operations)
}

func TestDeeperLook_ABoundVoiceFilesNoVoiceMeasures(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "deeperboundvoice", "  voice:\n    pack: technical-docs\n", deeperLookFiles)

	res, err := app.DeeperLook(context.Background(), DeeperLookRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	assert.Empty(t, deeperOf(res, SuggestKindVoice), "the bound voice states its measures already")
	assert.Len(t, deeperOf(res, SuggestKindTerm), 2)
}

func TestDeeperLook_DryRunDoesNotRepeatThePass(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "deeperdrypass", "", deeperLookFiles)

	pass, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	require.Len(t, suggestionsOf(pass, SuggestKindAddress), 1)

	res, err := app.DeeperLook(ctx, DeeperLookRequest{Project: recipeOf(root), DryRun: true, Pass: &pass})
	require.NoError(t, err)
	assert.Equal(t, SuggestKnown, measureOf(t, res, "person_pov").Outcome,
		"the pass would file it in the same run")
}
