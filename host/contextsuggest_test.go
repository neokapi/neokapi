package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// suggestProject writes a project over docs/*.md and i18n/*.json with the
// files given, and an optional extra recipe block under defaults.
func suggestProject(t *testing.T, name, defaults string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("kapi.yaml", `version: v1
id: `+projectIDFor(name)+`
name: `+name+`
defaults:
  source_language: en
`+defaults+`collections:
  - name: docs
    content:
      - path: "docs/*.md"
  - name: app
    source_only: true
    content:
      - path: "i18n/*.json"
`)
	for rel, body := range files {
		write(rel, body)
	}
	return root
}

func suggestionsOf(res ContextSuggestResult, kind string) []SuggestedFinding {
	var out []SuggestedFinding
	for _, s := range res.Suggestions {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

func TestSuggestFromFiles_Spelling(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files map[string]string
		// want maps each expected sentence to its forms and counts.
		want map[string][]SuggestedForm
	}{
		{
			name: "a hyphenated and a closed spelling across markdown and JSON",
			files: map[string]string{
				"docs/a.md":    "# Account\n\nWe send an email when your plan changes.\n\nReply to the email to reach us.\n",
				"i18n/en.json": `{"notice": "Check your e-mail for the code.", "footer": "Questions? Send an email."}`,
			},
			want: map[string][]SuggestedForm{
				"Write email, not e-mail or e mail.": {
					{Form: "email", Count: 3, Blocks: 3, Files: []string{"docs/a.md", "i18n/en.json"}},
					{Form: "e-mail", Count: 1, Blocks: 1, Files: []string{"i18n/en.json"}},
				},
			},
		},
		{
			name: "a two-word phrase beside its hyphenated and closed spellings",
			files: map[string]string{
				"docs/a.md": "Open the page and sign in.\n\nYou sign in once a day.\n\nThe sign-in page is slow.\n\nUse signin as the route name.\n",
			},
			want: map[string][]SuggestedForm{
				"Write sign in, not sign-in or signin.": {
					{Form: "sign in", Count: 2, Blocks: 2, Files: []string{"docs/a.md"}},
					{Form: "sign-in", Count: 1, Blocks: 1, Files: []string{"docs/a.md"}},
					{Form: "signin", Count: 1, Blocks: 1, Files: []string{"docs/a.md"}},
				},
			},
		},
		{
			name: "a capital at the start of a sentence or a heading is the same form",
			files: map[string]string{
				"docs/a.md": "# Email\n\nEmail is how we reach you. Your email stays private.\n\nEMAIL US\n",
			},
			want: map[string][]SuggestedForm{},
		},
		{
			name: "code spans, URLs and numbers are not words",
			files: map[string]string{
				"docs/a.md": "Send an email today.\n\nThe field is `e-mail` and the page is https://example.com/e-mail.\n\nWrite to e-mail2 or email3.\n",
			},
			want: map[string][]SuggestedForm{},
		},
		{
			name: "a capital inside a word is part of its spelling",
			files: map[string]string{
				"docs/a.md": "Push the branch to GitHub.\n\nOpen GitHub and review.\n\nThe Github app is installed.\n",
			},
			want: map[string][]SuggestedForm{
				"Write GitHub, not Github, Git Hub or Git-Hub.": {
					{Form: "GitHub", Count: 2, Blocks: 2, Files: []string{"docs/a.md"}},
					{Form: "Github", Count: 1, Blocks: 1, Files: []string{"docs/a.md"}},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := contextOpsApp(t)
			root := suggestProject(t, "spelling", "", tt.files)
			res, err := app.SuggestFromFiles(context.Background(), ContextSuggestRequest{Project: recipeOf(root), DryRun: true})
			require.NoError(t, err)
			got := map[string][]SuggestedForm{}
			for _, s := range suggestionsOf(res, SuggestKindSpelling) {
				got[s.Sentence] = s.Forms
				assert.Equal(t, SuggestFiled, s.Outcome, "a dry run reports what it would file")
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSuggestFromFiles_SpellingFiledByKapi(t *testing.T) {
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "filedbykapi", "", map[string]string{
		"docs/a.md": "Send an email.\n\nReply by email.\n\nOr send an e-mail.\n",
	})
	res, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	spelling := suggestionsOf(res, SuggestKindSpelling)
	require.Len(t, spelling, 1)
	assert.Equal(t, SuggestFiled, spelling[0].Outcome)
	assert.Equal(t, 3, spelling[0].Blocks)
	require.NotNil(t, spelling[0].Quote)
	assert.Equal(t, "docs/a.md", spelling[0].Quote.Path)
	assert.Equal(t, "Or send an e-mail.", spelling[0].Quote.Quote, "the quote shows the form the rule avoids")

	op, err := app.ContextSubject(ctx, recipeOf(root), spelling[0].ID)
	require.NoError(t, err)
	assert.Equal(t, SuggestActor, op.Actor)
	assert.Equal(t, contextop.KindObserve, op.Kind)
	assert.Equal(t, contextop.StatusSuggested, op.Status, "it advises and fails nothing until a person keeps it")
	assert.False(t, op.Established)
	rule, ok := op.Rule()
	require.True(t, ok)
	assert.Equal(t, "email", rule.Replacement)
	assert.False(t, rule.Advisory)

	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	var item *DigestItem
	for _, theme := range digest.Suggested {
		for _, g := range theme.Groups {
			for i := range g.Items {
				if g.Items[i].ID == spelling[0].ID {
					item = &g.Items[i]
				}
			}
		}
	}
	require.NotNil(t, item, "the review queue lists it")
	assert.Equal(t, DigestThemeNames, item.Theme)
	assert.True(t, item.Keepable)
	require.NotNil(t, item.Usage, "the queue shows how the files write each form")
	assert.Equal(t, 2, item.Usage.PreferredCount)
	assert.Equal(t, 1, item.Usage.RejectedCount)
	var text strings.Builder
	require.NoError(t, digest.FormatText(&text))
	assert.Contains(t, text.String(), "noticed by kapi\n")
}

func TestSuggestFromFiles_PackWords(t *testing.T) {
	three := map[string]string{
		"docs/a.md": "Setup is simple.\n\nIt is easy to start.\n\nObviously, it works.\n",
	}
	two := map[string]string{
		"docs/a.md": "Setup is simple.\n\nIt is easy to start.\n",
	}
	for _, tt := range []struct {
		name     string
		defaults string
		files    map[string]string
		want     []string
	}{
		{
			name:  "three words a pack avoids and no voice",
			files: three,
			want:  []string{"The files use 3 words the technical-docs starter pack avoids: simple once, easy once and obviously once, in 3 blocks across 1 file."},
		},
		{name: "two words are not enough", files: two},
		{
			name:     "a bound voice reports the words already",
			defaults: "  voice:\n    pack: technical-docs\n",
			files:    three,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := contextOpsApp(t)
			root := suggestProject(t, "packwords", tt.defaults, tt.files)
			res, err := app.SuggestFromFiles(context.Background(), ContextSuggestRequest{Project: recipeOf(root), DryRun: true})
			require.NoError(t, err)
			var got []string
			for _, s := range suggestionsOf(res, SuggestKindPackWords) {
				got = append(got, s.Sentence)
				assert.Equal(t, contextop.SubjectNote, s.Subject.Kind)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSuggestFromFiles_Address(t *testing.T) {
	// blocks writes n blocks that say you and m that use another pronoun.
	blocks := func(you, other int, extra string) map[string]string {
		var b strings.Builder
		for i := range you {
			fmt.Fprintf(&b, "Step %s: you open the panel.\n\n", strings.Repeat("a", i+1))
		}
		for i := range other {
			fmt.Fprintf(&b, "Note %s: they keep the panel open.\n\n", strings.Repeat("b", i+1))
		}
		b.WriteString(extra)
		return map[string]string{"docs/a.md": b.String()}
	}
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "seven of ten blocks with a pronoun say you",
			files: blocks(7, 3, ""),
			want:  "The files address the reader as you (person_pov: second): 7 of 10 blocks with a personal pronoun say you, and none says we.",
		},
		{name: "six of ten is below the share", files: blocks(6, 4, "")},
		{name: "one block that says we is enough to stay silent", files: blocks(8, 0, "We keep the panel open.\n")},
		{name: "two blocks are too few to say anything", files: blocks(2, 0, "")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := contextOpsApp(t)
			root := suggestProject(t, "address", "", tt.files)
			res, err := app.SuggestFromFiles(context.Background(), ContextSuggestRequest{Project: recipeOf(root), DryRun: true})
			require.NoError(t, err)
			found := suggestionsOf(res, SuggestKindAddress)
			if tt.want == "" {
				assert.Empty(t, found)
				return
			}
			require.Len(t, found, 1)
			assert.Equal(t, tt.want, found[0].Sentence)
			require.NotNil(t, found[0].Address)
			assert.Equal(t, "second", found[0].Address.PersonPOV)
		})
	}
}

func TestSuggestFromFiles_Idempotent(t *testing.T) {
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "idempotent", "", map[string]string{
		"docs/a.md": "Send an email.\n\nReply by email.\n\nOr send an e-mail.\n\nSetup is simple. It is easy. Obviously it works.\n\n" +
			"You open it.\n\nYou close it.\n\nYou keep it.\n",
	})
	first, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, first.Suggestions, 3)
	for _, s := range first.Suggestions {
		assert.Equal(t, SuggestFiled, s.Outcome, s.Sentence)
	}
	before, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)

	second, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, second.Suggestions, 3)
	for i, s := range second.Suggestions {
		assert.Equal(t, SuggestUnchanged, s.Outcome, s.Sentence)
		assert.Equal(t, first.Suggestions[i].ID, s.ID)
	}
	after, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Len(t, after.Operations, len(before.Operations), "a second run over the same files records nothing")
}

func TestSuggestFromFiles_DroppedStaysDropped(t *testing.T) {
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "dropped", "", map[string]string{
		"docs/a.md": "Send an email.\n\nReply by email.\n\nOr send an e-mail.\n",
	})
	first, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, first.Suggestions, 1)
	_, err = app.DecideContextReview(ctx, ContextReviewRequest{
		Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), Drop: []string{first.Suggestions[0].ID},
	})
	require.NoError(t, err)

	// The files gain a spelling; the person's drop still stands.
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "a.md"),
		[]byte("Send an email.\n\nReply by email.\n\nOr send an e-mail.\n\nOr an E-mail.\n\nOr e mail.\n"), 0o600))
	again, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, again.Suggestions, 1)
	assert.Equal(t, SuggestDropped, again.Suggestions[0].Outcome)
	assert.Equal(t, first.Suggestions[0].ID, again.Suggestions[0].ID)

	suggested, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root), Status: contextop.StatusSuggested})
	require.NoError(t, err)
	assert.Empty(t, suggested.Operations, "nothing was filed again")
}

func TestSuggestFromFiles_GainedFormUpdatesInPlace(t *testing.T) {
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "gainedform", "", map[string]string{
		"docs/a.md": "Open the page and sign in.\n\nYou sign in once a day.\n\nThe sign-in page is slow.\n",
	})
	first, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	spelling := suggestionsOf(first, SuggestKindSpelling)
	require.Len(t, spelling, 1)
	assert.Equal(t, "Write sign in, not sign-in.", spelling[0].Sentence)

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "a.md"),
		[]byte("Open the page and sign in.\n\nYou sign in once a day.\n\nThe sign-in page is slow.\n\nThe signin route.\n"), 0o600))
	dry, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, SuggestUpdated, suggestionsOf(dry, SuggestKindSpelling)[0].Outcome)

	second, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	updated := suggestionsOf(second, SuggestKindSpelling)
	require.Len(t, updated, 1)
	assert.Equal(t, SuggestUpdated, updated[0].Outcome)
	assert.Equal(t, "Write sign in, not sign-in or signin.", updated[0].Sentence)

	old, err := app.ContextSubject(ctx, recipeOf(root), spelling[0].ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusWithdrawn, old.Status, "the earlier suggestion is taken back")

	// One suggestion about the word is waiting, not two.
	waiting, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root), Status: contextop.StatusSuggested, Subjects: true})
	require.NoError(t, err)
	require.Len(t, waiting.Operations, 1)
	assert.Equal(t, updated[0].ID, waiting.Operations[0].ID)
}

func TestSuggestFromFiles_DryRunRecordsNothing(t *testing.T) {
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "dryrun", "", map[string]string{
		"docs/a.md": "Send an email.\n\nOr send an e-mail.\n",
	})
	res, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 1)
	assert.Empty(t, res.Suggestions[0].ID)
	log, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, log.Operations)
}

func TestSuggestFromFiles_KnownWordIsLeftAlone(t *testing.T) {
	app, _ := contextOpsApp(t)
	ctx := context.Background()
	root := suggestProject(t, "knownword", "", map[string]string{
		"docs/a.md": "Send an email.\n\nReply by email.\n\nOr send an e-mail.\n",
	})
	_, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s1"}, Project: recipeOf(root),
		Term: "e-mail", InsteadOf: []string{"email"},
	})
	require.NoError(t, err)
	res, err := app.SuggestFromFiles(ctx, ContextSuggestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 1)
	assert.Equal(t, SuggestKnown, res.Suggestions[0].Outcome, "an agent's suggestion about the word stands without a rival")
	assert.Empty(t, res.Suggestions[0].ID)
}
