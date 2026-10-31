package rulesview

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

const key = workspace.ProjectKey("acme/docs")

var (
	now    = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	claude = contextop.Actor{Kind: contextop.ActorAgent, Name: "claude"}
	you    = contextop.Actor{Kind: contextop.ActorPerson}
	docs   = contextop.Scope{Level: contextop.LevelProject, Coordinates: map[string]string{"channel": "docs"}}
)

func at(hours int) time.Time { return now.Add(time.Duration(hours) * time.Hour) }

func term(replacement, avoided string, forms ...string) contextop.Subject {
	return contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Replacement: replacement, Term: avoided, Forms: forms}}
}

// suggested is an observation waiting for a person.
func suggested(id string, s contextop.Subject, actor contextop.Actor, hours int) contextop.Record {
	return contextop.Record{
		ID: id, Project: key, Kind: contextop.KindObserve, Actor: actor, At: at(hours),
		Status: contextop.StatusSuggested, Subject: s, Scope: docs,
		Evidence: []contextop.Evidence{{Path: "docs/" + id + ".md", Quote: "seen in " + id}},
	}
}

func TestQuestions(t *testing.T) {
	established := suggested("est", term("studio", "business"), claude, -48)
	established.Status, established.Established = contextop.StatusEstablished, true
	withdrawn := suggested("wd", term("Acme", "ACME"), claude, -30)
	withdrawn.Status = contextop.StatusWithdrawn
	sideA := suggested("sa", term("sign in", "log in"), claude, -20)
	sideA.Status, sideA.ContestedBy = contextop.StatusContested, []string{"sb"}
	sideB := suggested("sb", term("sign on", "log in"), contextop.Actor{Kind: contextop.ActorAgent, Name: "codex"}, -21)
	sideB.Status, sideB.ContestedBy = contextop.StatusContested, []string{"sa"}

	tests := []struct {
		name    string
		records []contextop.Record
		want    []Decision
	}{
		{
			name:    "a name with several forms asks which one the files use",
			records: []contextop.Record{suggested("n1", term("Quickcast", "Quick cast", "QuickCast", "Quick cast"), claude, -1)},
			want: []Decision{{
				ID: "n1", Short: contextop.ShortID("n1"), Kind: DecisionName, Question: "Which one do your files use?",
				Sentence: "Write Quickcast, not Quick cast, QuickCast or Quick cast.", SuggestedBy: "claude",
				Options: []Option{{Form: "Quickcast", Scope: docs}, {Form: "Quick cast", Scope: docs}, {Form: "QuickCast", Scope: docs}},
			}},
		},
		{
			name:    "a spelling filed by kapi is a name with two forms",
			records: []contextop.Record{suggested("s1", term("e-mail", "email"), SuggestActor, -1)},
			want: []Decision{{
				ID: "s1", Short: contextop.ShortID("s1"), Kind: DecisionName, Question: "Which one do your files use?",
				Sentence: "Write e-mail, not email.", SuggestedBy: "kapi",
				Options: []Option{{Form: "e-mail", Scope: docs}, {Form: "email", Scope: docs}},
			}},
		},
		{
			name:    "a word rule asks whether to keep it",
			records: []contextop.Record{suggested("w1", term("studio", "business"), you, -1)},
			want: []Decision{{
				ID: "w1", Short: contextop.ShortID("w1"), Kind: DecisionWord, Question: "Keep this as a rule?",
				Sentence: "Write studio, not business.", SuggestedBy: "you",
			}},
		},
		{
			name:    "a tone measure is a note",
			records: []contextop.Record{suggested("t1", contextop.Subject{Kind: contextop.SubjectNote, Text: "Address the reader as you."}, claude, -1)},
			want: []Decision{{
				ID: "t1", Short: contextop.ShortID("t1"), Kind: DecisionNote, Question: "Keep this as a rule?",
				Sentence: "Address the reader as you.", SuggestedBy: "claude",
			}},
		},
		{
			name:    "a disagreement offers each side",
			records: []contextop.Record{sideA, sideB},
			want: []Decision{{
				Kind: DecisionDisagreement, Question: "Which one holds?",
				Reason: "These rules say different things about the same word. Choose one, and the others are set aside.",
				Options: []Option{
					{ID: "sa", Short: contextop.ShortID("sa"), Sentence: "Write sign in, not log in.", Subject: sideA.Subject, Scope: docs, NoticedBy: claude},
					{ID: "sb", Short: contextop.ShortID("sb"), Sentence: "Write sign on, not log in.", Subject: sideB.Subject, Scope: docs, NoticedBy: sideB.Actor},
				},
			}},
		},
		{
			name:    "an established rule and a withdrawn suggestion ask nothing",
			records: []contextop.Record{established, withdrawn},
			want:    []Decision{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest := BuildDigest(tt.records, key, time.Time{}, now, nil)
			got := Questions(digest, tt.records)
			require.Len(t, got, len(tt.want))
			for i, want := range tt.want {
				g := got[i]
				// The question carries its subject and scope for the counts.
				assert.Equal(t, docs, g.Scope)
				assert.NotNil(t, g.Seen)
				g.Subject, g.Scope, g.NoticedBy, g.Seen = contextop.Subject{}, contextop.Scope{}, contextop.Actor{}, nil
				assert.Equal(t, want, g)
			}
		})
	}
}

func TestQuestionsOrderDisagreementsFirst(t *testing.T) {
	a := suggested("a", term("sign in", "log in"), claude, -5)
	a.Status, a.ContestedBy = contextop.StatusContested, []string{"b"}
	b := suggested("b", term("sign on", "log in"), claude, -6)
	b.Status, b.ContestedBy = contextop.StatusContested, []string{"a"}
	records := []contextop.Record{suggested("w", term("studio", "business"), claude, -1), a, b}
	got := Questions(BuildDigest(records, key, time.Time{}, now, nil), records)
	require.Len(t, got, 2)
	assert.Equal(t, DecisionDisagreement, got[0].Kind)
	assert.Equal(t, DecisionWord, got[1].Kind)
	assert.True(t, NeedsBlocks(got))
	assert.False(t, NeedsBlocks([]Decision{{Kind: DecisionNote, Subject: contextop.Subject{Kind: contextop.SubjectNote}}}))
}

// counter counts over a docs file, a second docs file and a blog file the
// docs scope does not cover.
func counter() Counter {
	blocks := []Block{
		{Path: "docs/a.md", Coordinates: map[string]string{"channel": "docs"}, Text: "Open Quick cast and sign in."},
		{Path: "docs/a.md", Coordinates: map[string]string{"channel": "docs"}, Text: "Quick cast records your screen."},
		{Path: "docs/b.md", Coordinates: map[string]string{"channel": "docs"}, Text: "Quickcast is free; sign in, then sign on."},
		{Path: "blog/c.md", Coordinates: map[string]string{"channel": "blog"}, Text: "Quick cast on the blog."},
	}
	return Counter{
		Space:   axis.Space{},
		Applies: func(contextop.Scope) (string, bool) { return "docs", true },
		Blocks:  slices.Values(blocks),
	}
}

func TestCounterFill(t *testing.T) {
	c := counter()

	t.Run("each form of a name counts its blocks and files", func(t *testing.T) {
		d := suggestion(Item{ID: "n", Subject: term("Quickcast", "Quick cast", "QuickCast")}, docs)
		c.Fill(&d)
		assert.Equal(t, "docs", d.Applies)
		require.Len(t, d.Options, 3)
		assert.Equal(t, [2]int{1, 1}, [2]int{d.Options[0].Blocks, d.Options[0].Files})
		assert.Equal(t, [2]int{2, 1}, [2]int{d.Options[1].Blocks, d.Options[1].Files})
		assert.Equal(t, [2]int{0, 0}, [2]int{d.Options[2].Blocks, d.Options[2].Files})
		assert.Equal(t, `If you keep "Quickcast": the other 2 forms become findings in 2 blocks. Applies to docs.`, d.Options[0].Consequence)
		assert.Equal(t, `If you keep "Quick cast": the other 2 forms become findings in 1 block. Applies to docs.`, d.Options[1].Consequence)
	})

	t.Run("a disagreement chooses the side the files use most", func(t *testing.T) {
		d := Decision{Kind: DecisionDisagreement, Options: []Option{
			{ID: "on", Subject: term("sign on", "log in"), Scope: docs},
			{ID: "in", Subject: term("sign in", "log in"), Scope: docs},
		}}
		c.Fill(&d)
		assert.Equal(t, 1, d.Chosen)
		assert.Equal(t, 1, d.Options[0].Blocks)
		assert.Equal(t, 2, d.Options[1].Blocks)
		assert.Equal(t, 2, d.Options[1].Files)
		assert.Equal(t, `If you keep "sign in": no block uses "log in". Applies to docs.`, d.Options[1].Consequence)
	})

	t.Run("a word rule counts the blocks it would report", func(t *testing.T) {
		d := suggestion(Item{ID: "w", Subject: term("Quickcast", "Quick cast")}, contextop.Scope{})
		d.Kind = DecisionWord
		c.Fill(&d)
		assert.Equal(t, `If you keep it: "Quick cast" becomes a finding in 3 blocks. Applies to docs.`, d.Consequence)
	})

	t.Run("an advisory rule reports and fails nothing", func(t *testing.T) {
		s := term("studio", "Quick cast", "sign on")
		s.Term.Advisory = true
		d := Decision{Kind: DecisionWord, Subject: s, Scope: docs}
		c.Fill(&d)
		assert.Equal(t, `If you keep it: "Quick cast" or "sign on" become findings in 3 blocks. They report and fail nothing. Applies to docs.`, d.Consequence)
	})

	t.Run("a note and a content-memory pair are stated, not counted", func(t *testing.T) {
		note := Decision{Kind: DecisionNote, Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "Short sentences."}}
		c.Fill(&note)
		assert.Equal(t, "If you keep it: it is stated to writers and assistants where it applies. No check reads it. Applies to docs.", note.Consequence)
		pair := Decision{Kind: DecisionMemory, Subject: contextop.Subject{Kind: contextop.SubjectMemory, Memory: &contextop.MemoryPair{Source: "Save", Target: "Lagre", TargetLocale: "nb"}}}
		c.Fill(&pair)
		assert.Equal(t, "If you keep it: the content memory holds this wording. Applies to docs.", pair.Consequence)
	})

	t.Run("no blocks count no use", func(t *testing.T) {
		d := suggestion(Item{ID: "n", Subject: term("Quickcast", "Quick cast")}, docs)
		Counter{Applies: c.Applies}.Fill(&d)
		assert.Equal(t, 0, d.Options[1].Blocks)
		assert.Equal(t, `If you keep "Quickcast": no block uses the other form. Applies to docs.`, d.Options[0].Consequence)
	})
}

func TestConsequenceOfATypedForm(t *testing.T) {
	d := suggestion(Item{ID: "n", Subject: term("Quickcast", "Quick cast", "QuickCast")}, docs)
	// Keeping a form the rule avoided makes the suggested form one to avoid.
	assert.Equal(t, `If you keep "Quick cast": the other 2 forms become findings in 1 block. Applies to docs.`, counter().Consequence(d, "Quick cast"))
	assert.Equal(t, `If you keep "QUICKCAST": the other 2 forms become findings in 2 blocks. Applies to docs.`, counter().Consequence(d, "QUICKCAST"))
}

func TestDecisionLabelAndAvoided(t *testing.T) {
	name := Decision{Kind: DecisionName, Subject: term("Quickcast", "Quick cast", "QuickCast")}
	assert.Equal(t, "Quickcast", name.Label())
	assert.Equal(t, 0, name.Avoided())
	word := Decision{Kind: DecisionWord, Subject: term("studio", "business", "biz", "")}
	assert.Equal(t, "business", word.Label())
	assert.Equal(t, 2, word.Avoided())
	pair := Decision{Kind: DecisionMemory, Subject: contextop.Subject{Kind: contextop.SubjectMemory, Memory: &contextop.MemoryPair{Source: "Save"}}}
	assert.Equal(t, "Save", pair.Label())
}

func TestBuildDigest(t *testing.T) {
	marker := at(-10)
	collectionFor := func(path string) string {
		if len(path) > 5 && path[:5] == "docs/" {
			return "docs"
		}
		return "blog"
	}
	inBlog := suggested("w2", term("studio", "agency"), claude, -2)
	inBlog.Evidence[0].Path = "blog/w2.md"
	kept := suggested("k1", term("Acme", "ACME"), claude, -50)
	kept.Status, kept.Established = contextop.StatusEstablished, true
	keep := contextop.Record{ID: "k1-keep", Project: key, Kind: contextop.KindKeep, Target: "k1", Actor: you, At: at(-40), Status: contextop.StatusEstablished}
	byEvidence := suggested("c1", term("Log in", "Sign in"), claude, -1)
	byEvidence.Status, byEvidence.Established, byEvidence.ContestedBy = contextop.StatusContested, true, []string{"c1-fix"}
	byEvidence.Standing = &contextop.Standing{AgainstBy: []string{"c1-fix"}}
	fix := contextop.Record{
		ID: "c1-fix", Project: key, Kind: contextop.KindCorrect, Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: "bob"}, At: at(-1),
		Correction: &contextop.Correction{From: "Log in", To: "Sign in"}, Evidence: []contextop.Evidence{{Path: "docs/login.md"}},
		Status: contextop.StatusSuggested,
	}
	other := suggested("x1", term("Quickcast", "Quick cast"), claude, -1)
	other.Project = "acme/other"
	records := []contextop.Record{
		fix, byEvidence, other,
		suggested("n1", term("Quickcast", "Quick cast"), claude, -1),
		inBlog,
		suggested("w1", term("studio", "business"), claude, -20),
		suggested("t1", contextop.Subject{Kind: contextop.SubjectNote, Text: "Short sentences."}, claude, -3),
		keep, kept,
	}

	d := BuildDigest(records, key, marker, now, collectionFor)

	require.Len(t, d.Conflicts, 1)
	assert.True(t, d.Conflicts[0].ByEvidence)
	assert.Equal(t, `bob's changed "Log in" back to "Sign in" in docs/login.md.`, d.Conflicts[0].Reason)

	require.Len(t, d.Established, 1)
	assert.Equal(t, []string{"kept by you"}, d.Established[0].How)
	assert.Equal(t, at(-40), d.Established[0].EstablishedAt)
	assert.False(t, d.Established[0].New)
	assert.Equal(t, []string{WidenToWorkspace, "channel"}, d.Established[0].WidenTo)

	var themesSeen []string
	for _, th := range d.Suggested {
		themesSeen = append(themesSeen, th.Theme)
	}
	assert.Equal(t, []string{ThemeNames, ThemeWords, ThemeWriting}, themesSeen, "the themes in digest order, the other project's left out")

	words := d.Suggested[1]
	require.Len(t, words.Groups, 2, "words sit in two collections, so each has a group")
	assert.Equal(t, "blog", words.Groups[0].Collection)
	assert.Equal(t, "docs", words.Groups[1].Collection)
	assert.True(t, words.Groups[0].Items[0].New)
	assert.False(t, words.Groups[1].Items[0].New, "recorded before the marker")
	assert.Empty(t, d.Suggested[0].Groups[0].Collection, "one collection is not named")

	assert.Equal(t, Numbers{Rules: 1, NewThisWeek: 1, Suggested: 4, Conflicts: 1, New: 4}, d.Numbers)
}

func TestBuildDigestCapsEarlierEstablished(t *testing.T) {
	var records []contextop.Record
	for i := range earlierEstablished + 3 {
		r := suggested(string(rune('a'+i)), term("w"+string(rune('a'+i)), "x"), claude, -100-i)
		r.Status, r.Established = contextop.StatusEstablished, true
		records = append(records, r)
	}
	d := BuildDigest(records, key, at(-1), now, nil)
	assert.Len(t, d.Established, earlierEstablished)
	assert.Equal(t, earlierEstablished+3, d.Numbers.Rules)
}

func TestSeenByAndActorPhrase(t *testing.T) {
	alice := contextop.Actor{Kind: contextop.ActorPerson, Name: "alice"}
	records := []contextop.Record{{ID: "1", Actor: alice}, {ID: "2", Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: "bob"}}}
	seen := SeenBy(records, alice)
	assert.Equal(t, "you", ActorPhrase(seen[0].Actor, false))
	assert.Equal(t, "bob's", ActorPhrase(seen[1].Actor, true))
	assert.Equal(t, "alice", records[0].Actor.Name, "the records are copied")
	assert.Equal(t, "kapi", ActorPhrase(SuggestActor, false))
	assert.Equal(t, "kapi check", ActorPhrase(contextop.Actor{Kind: contextop.ActorTool, Name: "check"}, false))
	assert.Equal(t, "an agent's", ActorPhrase(contextop.Actor{Kind: contextop.ActorAgent}, true))
}
