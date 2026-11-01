package screens_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/host/screens"
)

// fixturesDir holds one JSON document per screen. The views' tests in
// packages/project-screens read the same files through the TypeScript types.
var fixturesDir = filepath.Join("..", "..", "packages", "project-screens", "fixtures")

// TestFixtures pins the JSON each screen reads. Regenerate the files after an
// intentional change to a shape with:
//
//	KAPI_UPDATE_GOLDEN=1 go test ./host/screens
func TestFixtures(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"map-summary.json", mapSummary()},
		{"reader-file.json", readerFile()},
		{"rules-decide-view.json", rulesDecideView()},
		{"kept-rules.json", keptRules()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := json.MarshalIndent(c.value, "", "  ")
			require.NoError(t, err)
			got = append(got, '\n')
			path := filepath.Join(fixturesDir, c.name)
			if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
				require.NoError(t, os.MkdirAll(fixturesDir, 0o755))
				require.NoError(t, os.WriteFile(path, got, 0o644))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "fixture missing; run with KAPI_UPDATE_GOLDEN=1 to create it")
			assert.Equal(t, string(want), string(got),
				"%s differs from the shape host/screens marshals. If the change is intended, regenerate with KAPI_UPDATE_GOLDEN=1 and update packages/project-screens/src/types.ts", path)
		})
	}
}

var (
	recordedAt  = time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC)
	agent       = contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code@build-01", Session: "s-7f3a"}
	personAlone = contextop.Actor{Kind: contextop.ActorPerson}
)

// quickcastSides are the two sides of a disagreement about one product name.
func quickcastSides() []screens.DigestItem {
	return []screens.DigestItem{
		{
			ID: "01JBQ8X4K2M7Z9", Short: "q7z9", Status: contextop.StatusContested, Theme: "names",
			Sentence: "Write Quickcast, not Quick cast or QuickCast.",
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{
				Term: "Quick cast", Replacement: "Quickcast", Forms: []string{"QuickCast"},
			}},
			Quote:      &contextop.Evidence{Path: "docs/billing.md", Unit: "b2", Quote: "Quickcast keeps your credits."},
			Collection: "docs",
			Standing:   "seen in 3 sessions · 14 of 17 uses in docs/",
			Usage: &screens.DigestUsage{
				Preferred: "Quickcast", PreferredCount: 14, Rejected: []string{"Quick cast", "QuickCast"}, RejectedCount: 3,
				Within: "docs/", Line: `docs/ says "Quickcast" 14 times and "Quick cast" or "QuickCast" 3 times`,
			},
			NoticedBy: agent, At: recordedAt, New: true, Scope: "project acme/docs",
			Droppable: true,
		},
		{
			ID: "01JBQ9A1C3D5E7", Short: "e5e7", Status: contextop.StatusContested, Theme: "names",
			Sentence: "Write QuickCast, not Quickcast.",
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{
				Term: "Quickcast", Replacement: "QuickCast",
			}},
			Quote:     &contextop.Evidence{Path: "docs/release-notes.md", Quote: "QuickCast now reads PDF files."},
			NoticedBy: personAlone, At: recordedAt.Add(2 * time.Hour), Scope: "project acme/docs",
			Droppable: true,
		},
	}
}

func mapSummary() screens.MapSummaryDTO {
	findings := 4
	sides := quickcastSides()
	return screens.MapSummaryDTO{
		ProjectKey:      "acme/docs",
		Files:           3,
		Blocks:          42,
		Rules:           7,
		Findings:        &findings,
		CheckedAt:       "2026-10-30T12:00:00Z",
		ReadFiles:       3,
		ReadAt:          "2026-10-30T11:58:12Z",
		Suggested:       1,
		Conflicts:       1,
		Awaiting:        2,
		TargetLanguages: []string{"de", "nb"},
		FileRows: []screens.MapFileDTO{
			{Path: "/work/acme-docs/docs/billing.md", Relative: "docs/billing.md", Format: "markdown", Collection: "docs", Blocks: 18, Findings: 3},
			{Path: "/work/acme-docs/docs/release-notes.md", Relative: "docs/release-notes.md", Format: "markdown", Collection: "docs", Blocks: 15, Findings: 1},
			{Path: "/work/acme-docs/locales/en.json", Relative: "locales/en.json", Format: "json", Collection: "app", Blocks: 9, Findings: 0},
		},
		Suggestions: []screens.MapSuggestionDTO{
			{
				Group:    "conflict",
				Item:     sides[0],
				Conflict: &screens.DigestConflict{Sides: sides, Reason: "Two suggestions name different forms for Quickcast."},
				Blocks:   2,
				Files:    2,
			},
			{
				Group: "suggested",
				Item: screens.DigestItem{
					ID: "01JBQB7F2G4H6J", Short: "h6j1", Status: contextop.StatusSuggested, Theme: "writing",
					Sentence:  "Keep sentences in release notes under 25 words.",
					Subject:   contextop.Subject{Kind: contextop.SubjectNote, Text: "Keep sentences in release notes under 25 words."},
					Quote:     &contextop.Evidence{Path: "docs/release-notes.md", Unit: "b7", Quote: "QuickCast now reads PDF files."},
					NoticedBy: agent, At: recordedAt.Add(time.Hour), New: true, Scope: "project acme/docs",
					Keepable: true, Droppable: true,
				},
				Blocks: 1,
				Files:  1,
			},
		},
	}
}

func readerFile() screens.ReaderFileDTO {
	date := model.PlaceholderRun{ID: "1", Type: "var", Data: "{date}", Equiv: "date"}
	count := model.PlaceholderRun{ID: "1", Type: "var", Data: "{count}", Equiv: "count"}
	return screens.ReaderFileDTO{
		Path:   "docs/billing.md",
		Abs:    "/work/acme-docs/docs/billing.md",
		Format: "markdown",
		Blocks: []screens.ReaderBlockDTO{
			{
				ID: "b1",
				Runs: []model.Run{
					model.TextR("Your credits reset on "), model.PhR(date), model.TextR(". Ask Quick cast support."),
				},
				Findings: []screens.ReaderFindingDTO{{
					ID: "f-3c1a9e", BlockID: "b1",
					Position: model.Anchor{Kind: model.AnchorRange, Start: model.RunPos{Run: 2, Offset: 6}, End: model.RunPos{Run: 2, Offset: 16}},
					Span:     &screens.ReaderSpan{Start: 28, End: 38},
					Kind:     "name", Label: "Write Quickcast, not Quick cast", Fails: true, Fixable: true,
				}},
			},
			{
				ID: "b2",
				Runs: []model.Run{{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
					"one":   {model.TextR("You have "), model.PhR(count), model.TextR(" credit left.")},
					"other": {model.TextR("You have "), model.PhR(count), model.TextR(" credits left, use them easily.")},
				}}}},
				Findings: []screens.ReaderFindingDTO{{
					ID: "f-8d02b4", BlockID: "b2",
					Position: model.Anchor{Kind: model.AnchorForm, Path: model.RunPath{{Kind: model.StepIndex, Index: 0}}, Key: "other"},
					Kind:     "word", Label: `Avoid "easily"`, Fails: false, Suggested: true, Fixable: false,
				}},
			},
			{
				ID:   "b3",
				Runs: []model.Run{model.TextR("Billing runs on the first day of each month.")},
				Findings: []screens.ReaderFindingDTO{{
					ID: "f-51e7c0", BlockID: "b3", Position: model.Anchor{Kind: model.AnchorBlock},
					Kind: "tone", Label: "The translation reads less formal than the voice profile asks.", Fails: false, Fixable: false, Locale: "nb",
				}},
			},
		},
		Document: []screens.ReaderFindingDTO{{
			ID: "f-0a4f2d", Kind: "hygiene", Label: "The file has no top-level heading.", Fails: false, Fixable: false,
		}},
		Findings: 4,
		Fixable:  1,
		Verdict:  "failed",
		Rules: []screens.ReaderRuleDTO{
			{Kind: "tone", Rule: "Formal, plain register; short sentences.", Status: "established", From: "voice profile"},
			{Kind: "name", Rule: "Write Quickcast, not Quick cast.", Status: "established", From: "terms", By: "maria", At: "2026-10-21T14:02:00Z"},
			{Kind: "note", Rule: "Keep sentences in release notes under 25 words.", Status: "suggested", From: "workspace"},
		},
		RulesTotal: 3,
		Attention:  []string{"The terms store acme-terms did not load: the file is missing."},
		Refused:    []screens.ReaderRefusalDTO{{FindingID: "f-77b3e1", Reason: "The block changed after the check read it."}},
	}
}

func rulesDecideView() screens.RulesDecideViewDTO {
	agentDTO := screens.ContextActorDTO{Kind: "agent", Name: "claude-code", Session: "s-7f3a", Host: "build-01"}
	nobody := screens.ContextActorDTO{Kind: "person"}
	return screens.RulesDecideViewDTO{
		Project:     "acme/docs",
		ProjectName: "Acme docs",
		Decisions: []screens.RuleDecisionDTO{
			{
				ID: "01JBQC2K8M1N3P", Short: "n3p4", Kind: "name",
				Question: "Which one do your files use?",
				Sentence: "Write Invoicely, not Invoice-ly.",
				Label:    "Invoicely", Avoided: 1,
				Options: []screens.RuleOptionDTO{
					{Form: "Invoicely", Blocks: 12, Files: 4, Consequence: `If you keep "Invoicely": the other form becomes a finding in 2 blocks. Applies to the whole project.`},
					{Form: "Invoice-ly", Blocks: 2, Files: 1, Consequence: `If you keep "Invoice-ly": the other form becomes a finding in 12 blocks. Applies to the whole project.`},
				},
				Chosen:  0,
				Other:   true,
				Seen:    &screens.ContextEvidenceDTO{Path: "docs/billing.md", Unit: "b4", Quote: "Invoicely sends the receipt."},
				Applies: "the whole project", NoticedBy: agentDTO,
			},
			{
				Kind:     "disagreement",
				Question: "Which one do your files use?",
				Reason:   "Two suggestions name different forms for Quickcast.",
				Sentence: "Write Quickcast, not Quick cast or QuickCast.",
				Label:    "Quickcast",
				Options: []screens.RuleOptionDTO{
					{ID: "01JBQ8X4K2M7Z9", Short: "q7z9", Sentence: "Write Quickcast, not Quick cast or QuickCast.", Blocks: 14, Files: 5,
						Consequence: `If you keep it: "Quick cast" or "QuickCast" become findings in 3 blocks. Applies to the whole project.`, NoticedBy: &agentDTO},
					{ID: "01JBQ9A1C3D5E7", Short: "e5e7", Sentence: "Write QuickCast, not Quickcast.", Blocks: 2, Files: 1,
						Consequence: `If you keep it: "Quickcast" becomes a finding in 14 blocks. Applies to the whole project.`, NoticedBy: &nobody},
				},
				Chosen:  0,
				Seen:    &screens.ContextEvidenceDTO{Path: "docs/billing.md", Unit: "b2", Quote: "Quickcast keeps your credits."},
				Applies: "the whole project", NoticedBy: agentDTO,
			},
			{
				ID: "01JBQB7F2G4H6J", Short: "h6j1", Kind: "note",
				Question:    "Keep this as a rule?",
				Sentence:    "Keep sentences in release notes under 25 words.",
				Label:       "Keep sentences in release notes under 25 words.",
				Options:     []screens.RuleOptionDTO{},
				Consequence: "If you keep it: it is stated to writers and assistants where it applies. No check reads it. Applies to docs/release-notes.md.",
				Applies:     "docs/release-notes.md", NoticedBy: nobody,
				Assignee: &screens.ContextActorDTO{Kind: "person", Name: "Mira Holt"}, AssigneeID: "user-mira",
			},
		},
		Rules:     7,
		Suggested: 3,
		Narrowed:  1,
	}
}

// keptRules is Kept by: a rule a person kept, one a teammate kept from the
// terms store, and a note an assistant kept.
func keptRules() []screens.KeptRuleDTO {
	return []screens.KeptRuleDTO{
		{
			Kind: "word", Rule: "Write sign in, not login.", From: "terms", Applies: "the whole project",
			KeptBy: screens.ContextActorDTO{Kind: "person"}, KeptAt: recordedAt.Format(time.RFC3339),
		},
		{
			Kind: "name", Rule: "Write Quickcast, not Quick cast or QuickCast.", Applies: "docs",
			KeptBy: screens.ContextActorDTO{Kind: "person", Name: "ada"}, KeptAt: recordedAt.Add(-48 * time.Hour).Format(time.RFC3339),
		},
		{
			Kind: "note", Rule: "Address the reader as you.", Applies: "the whole project",
			KeptBy: screens.ContextActorDTO{Kind: "agent", Name: "claude-code", Session: "s-7f3a", Host: "build-01"},
			KeptAt: recordedAt.Add(-24 * time.Hour).Format(time.RFC3339),
		},
	}
}
