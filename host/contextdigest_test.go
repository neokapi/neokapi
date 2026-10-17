package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// TestContextDigest_SectionsFromTheLog drives the digest through the host API:
// an agent's suggestion lands under Suggested, a person's keep moves it to
// Established with how it got there, two rival suggestions land under Needs
// you, and the marker splits new from earlier without hiding anything.
func TestContextDigest_SectionsFromTheLog(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-digest")
	ctx := t.Context()

	kept := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{kept.ID}})
	require.NoError(t, err)

	spelling, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(root),
		Term: "Quickcast", InsteadOf: []string{"Quick cast", "QuickCast"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Quote: "Try Quick cast today"}},
	})
	require.NoError(t, err)
	for _, use := range []string{"sign in", "log in"} {
		_, err = app.RecordContextObservation(ctx, ContextObserveRequest{
			Actor: agentIn("s-" + use), Project: recipeOf(root),
			Term: use, InsteadOf: []string{"login"},
		})
		require.NoError(t, err)
	}

	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)

	require.Len(t, digest.Established, 1)
	assert.Equal(t, "Write use, not utilise.", digest.Established[0].Sentence)
	assert.Equal(t, []string{"kept by you"}, digest.Established[0].How)
	assert.True(t, digest.Established[0].Droppable)
	assert.True(t, digest.Established[0].Widenable)
	assert.Equal(t, ContextWidenOptions(contextop.Scope{Level: contextop.LevelProject}), digest.Established[0].WidenTo,
		"the digest offers the widenings the feed and the CLI offer")
	assert.Contains(t, digest.Established[0].WidenTo, WidenToWorkspace)
	assert.True(t, digest.Established[0].New, "nobody has looked, so everything is new")

	require.Len(t, digest.Conflicts, 1, "the two rules for login disagree")
	assert.Len(t, digest.Conflicts[0].Sides, 2)
	for _, side := range digest.Conflicts[0].Sides {
		assert.True(t, side.Keepable, "choosing a side keeps it")
	}

	require.Len(t, digest.Suggested, 1)
	assert.Equal(t, DigestThemeNames, digest.Suggested[0].Theme)
	item := digest.Suggested[0].Groups[0].Items[0]
	assert.Equal(t, spelling.ID, item.ID)
	assert.Contains(t, item.Sentence, "Write Quickcast, not Quick cast, QuickCast")
	require.NotNil(t, item.Quote)
	assert.Equal(t, "config/app.yaml", item.Quote.Path)
	assert.Equal(t, "config", item.Collection)
	assert.True(t, item.Keepable)

	assert.Equal(t, 1, digest.Numbers.Rules)
	assert.Equal(t, 1, digest.Numbers.NewThisWeek)
	assert.Equal(t, 1, digest.Numbers.Suggested)
	assert.Equal(t, 1, digest.Numbers.Conflicts)

	// A person at a command line reads it, which moves the marker. The next
	// digest still carries every item, flagged as seen.
	require.NoError(t, app.NoteContextDigestRead(digest))
	assert.False(t, ContextDigestMarker(workspace.ProjectKey(digest.Project)).IsZero())
	again, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, again.Established, 1)
	assert.False(t, again.Established[0].New)
	require.Len(t, again.Suggested, 1)
	assert.False(t, again.Suggested[0].Groups[0].Items[0].New)
	assert.Equal(t, 0, again.Numbers.New)

	var text bytes.Buffer
	require.NoError(t, again.FormatText(&text))
	assert.Contains(t, text.String(), "Needs you")
	assert.Contains(t, text.String(), "Earlier")
	assert.Contains(t, text.String(), "kapi knows 1 rule for ctxops-digest; 1 is new this week.")

	// Choosing a side drops the rival and keeps the chosen rule.
	chosen := again.Conflicts[0].Sides[0]
	res, err := app.ChooseContextSide(ctx, ContextChooseRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), ID: chosen.ID})
	require.NoError(t, err)
	require.Len(t, res.SetAside, 1)
	assert.Equal(t, contextop.KindDrop, res.SetAside[0].Kind)
	after, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, after.Conflicts)
	assert.Equal(t, 2, after.Numbers.Rules)
	require.NotEmpty(t, after.Established)
	assert.Equal(t, chosen.ID, after.Established[0].ID, "the newest rule in force comes first")
	assert.True(t, after.Established[0].New)
}

// TestContextDigest_ContentMovingAwayFromARuleIsDrift: a check counts an
// established rule's forms, the content then moves to the rejected form, and the
// next check's count shows the rule under Drift. Uses the content already held
// when the rule was kept are the baseline, not drift.
func TestContextDigest_ContentMovingAwayFromARuleIsDrift(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-drift")
	ctx := t.Context()

	rule := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{rule.ID}})
	require.NoError(t, err)

	checkWith(t, app, root)
	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, digest.Drift, "what the content held when the rule was kept is the baseline")
	require.Len(t, digest.Established, 1)
	require.NotNil(t, digest.Established[0].Usage, "a check counts an established rule's forms")
	assert.Equal(t, 1, digest.Established[0].Usage.RejectedCount)

	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"),
		[]byte("greeting: We utilise the widget every day.\nfarewell: Utilise it, and utilise it again.\n"), 0o600))
	checkWith(t, app, root)

	digest, err = app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, digest.Drift, 1)
	assert.Equal(t, rule.ID, digest.Drift[0].Rule.ID)
	assert.Equal(t, 1, digest.Drift[0].Before)
	assert.Equal(t, 3, digest.Drift[0].Rejected)
	assert.Contains(t, digest.Drift[0].Line, `"utilise" 3 times`)
	assert.True(t, digest.Drift[0].Rule.Droppable)
}

// TestUsage_UnchangedCountsRecordNothing: two checks over the same content
// record one usage signal per rule, and a changed count records a second, for
// a suggestion and for an established rule alike.
func TestUsage_UnchangedCountsRecordNothing(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-usage-once")
	ctx := t.Context()

	suggestion := proposeUtilise(t, app, root, agentIn("s1"))
	established, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(root),
		Term: "farewell", InsteadOf: []string{"Goodbye"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "farewell", Quote: "Goodbye"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{established.ID}})
	require.NoError(t, err)

	usageSignals := func(target string) int {
		t.Helper()
		log, lerr := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
		require.NoError(t, lerr)
		n := 0
		for _, op := range log.Operations {
			if op.Kind == contextop.KindSignal && op.Target == target && op.Signal != nil && op.Signal.Source == contextop.SignalUsage {
				n++
			}
		}
		return n
	}

	checkWith(t, app, root)
	checkWith(t, app, root)
	assert.Equal(t, 1, usageSignals(suggestion.ID), "an unchanged count of a suggestion is recorded once")
	assert.Equal(t, 1, usageSignals(established.ID), "an unchanged count of an established rule is recorded once")

	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"),
		[]byte("greeting: We utilise the widget every day, and utilise it again.\nfarewell: Goodbye\n"), 0o600))
	checkWith(t, app, root)
	assert.Equal(t, 2, usageSignals(suggestion.ID), "a changed count is recorded")
	assert.Equal(t, 1, usageSignals(established.ID), "a rule whose count held still records nothing")
}

// TestContextDigest_AgentReadLeavesTheMarker: an agent reading the digest does
// not move the person's marker.
func TestContextDigest_AgentReadLeavesTheMarker(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvActor, "agent")
	app, _ := contextOpsApp(t)
	require.NoError(t, app.NoteContextDigestRead(ContextDigest{Project: "prj_x"}))
	assert.True(t, ContextDigestMarker("prj_x").IsZero())
}

// TestBuildDigest_EstablishedOnEvidenceAndDrift covers what the host API
// cannot produce without a merge: an establish operation naming a merge and a
// correction, and a later usage count moving to a rejected form.
func TestBuildDigest_EstablishedOnEvidenceAndDrift(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	at := func(d int) time.Time { return now.Add(time.Duration(d) * time.Hour) }
	rule := &profile.TermRule{Term: "business", Replacement: "studio"}
	key := workspace.ProjectKey("prj_fernwell")
	agent := contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s1"}

	// Newest first, as Ledger.Records returns them.
	records := []contextop.Record{
		{ID: "07", Project: key, Kind: contextop.KindSignal, Target: "01", At: at(-1), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalUsage, Preferred: 38, Rejected: 5, Within: "docs/"}},
		{ID: "05", Project: key, Kind: contextop.KindEstablish, Target: "01", At: at(-20), Because: []string{"03", "04"}, Status: contextop.StatusEstablished},
		{ID: "04", Project: key, Kind: contextop.KindCorrect, At: at(-22), Actor: contextop.Actor{Kind: contextop.ActorPerson},
			Correction: &contextop.Correction{From: "business", To: "studio"}, Evidence: []contextop.Evidence{{Path: "docs/billing.md"}},
			Status: contextop.StatusSuggested},
		{ID: "03", Project: key, Kind: contextop.KindSignal, Target: "01", At: at(-23), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalMerge, PR: 412}},
		{ID: "02", Project: key, Kind: contextop.KindSignal, Target: "01", At: at(-24), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalUsage, Preferred: 41, Rejected: 2, Within: "docs/"}},
		{ID: "01", Project: key, Kind: contextop.KindObserve, Actor: agent, At: at(-48), Status: contextop.StatusEstablished, Established: true,
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: rule}},
	}
	d := buildDigest(records, key, at(-10), now, nil)

	require.Len(t, d.Established, 1)
	assert.Equal(t, []string{"merged in #412", "your correction in docs/billing.md"}, d.Established[0].How)
	assert.False(t, d.Established[0].New, "it was established before the marker")

	require.Len(t, d.Drift, 1)
	assert.Equal(t, 5, d.Drift[0].Rejected)
	assert.Equal(t, 2, d.Drift[0].Before)
	assert.Equal(t, `docs/ says "studio" 38 times and "business" 5 times`, d.Drift[0].Line)
	assert.Equal(t, DigestThemeWords, d.Established[0].Theme)
}

func TestDigestUsageLine(t *testing.T) {
	u := digestUsage("studio", []string{"business"}, 41, 2, "docs/")
	assert.Equal(t, `docs/ says "studio" 41 times and "business" twice`, u.Line)
	assert.Equal(t, `The project says "Quickcast" once`, digestUsage("Quickcast", []string{"Quick cast"}, 1, 0, "").Line)
}

// TestContextStandings_EveryProjectWithItsLatestSession: one project holds an
// agent's suggestions and a person's keep, the other nothing. Both are listed,
// the first with its numbers and the agent's session as the latest, which the
// keep does not displace.
func TestContextStandings_EveryProjectWithItsLatestSession(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, _ := contextOpsApp(t)
	busy := contextOpsProject(t, "ctxops-standing-busy")
	quiet := contextOpsProject(t, "ctxops-standing-quiet")
	ctx := t.Context()

	kept := proposeUtilise(t, app, busy, agentIn("s1"))
	_, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(busy),
		Term: "Quickcast", InsteadOf: []string{"Quick cast"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Quote: "Try Quick cast today"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(busy), IDs: []string{kept.ID}})
	require.NoError(t, err)
	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	_, err = ws.Register(ctx, workspace.ProjectKey(projectIDFor("ctxops-standing-quiet")), "ctxops-standing-quiet", quiet)
	require.NoError(t, err)

	standings, err := app.ContextStandings(ctx)
	require.NoError(t, err)
	byName := map[string]ContextProjectStanding{}
	for _, s := range standings {
		byName[s.ProjectName] = s
	}
	require.Contains(t, byName, "ctxops-standing-busy")
	require.Contains(t, byName, "ctxops-standing-quiet", "a project with nothing recorded is listed too")

	b := byName["ctxops-standing-busy"]
	assert.Equal(t, 1, b.Rules)
	assert.Equal(t, 1, b.Suggested)
	assert.Equal(t, 0, b.Conflicts)
	assert.Equal(t, 2, b.New, "nobody has looked, so the rule and the suggestion are new")
	assert.Equal(t, 1, b.Terms, "the kept rule is a concept in the terms store; the suggestion is not")
	require.NotNil(t, b.Latest)
	assert.Equal(t, "s2", b.Latest.Actor.Session, "the person's keep leaves the agent's session as the latest")
	assert.True(t, b.Latest.Working, "the session recorded a moment ago")
	assert.Equal(t, 1, b.Latest.Recorded)
	assert.Equal(t, map[string]int{DigestThemeNames: 1}, b.Latest.NewByTheme)

	q := byName["ctxops-standing-quiet"]
	assert.Zero(t, q.Rules)
	assert.Zero(t, q.Terms)
	assert.Zero(t, q.New)
	assert.Nil(t, q.Latest)
}

// TestContextStandings_CountTheTermsStore: a project's terms figure is the count
// its terms store reports, the number the Terms section of its context lists.
// A store with no concepts counts none, and so do a project whose store the
// workspace does not hold, which reading the standings leaves uncreated, and a
// store that cannot be read, which fails nothing. Two projects count their own
// stores, so their sum counts every concept once.
func TestContextStandings_CountTheTermsStore(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, workspaceRoot := contextOpsApp(t)
	stocked := contextOpsProject(t, "ctxops-terms-stocked")
	other := contextOpsProject(t, "ctxops-terms-other")
	empty := contextOpsProject(t, "ctxops-terms-empty")
	ctx := t.Context()

	addConcepts := func(root string, ids ...string) *projector.Terms {
		t.Helper()
		p, err := app.Projector(ctx, root)
		require.NoError(t, err)
		for _, id := range ids {
			require.NoError(t, p.Terms().AddConcept(ctx, terms.Concept{
				ID:    id,
				Terms: []terms.Term{{Text: id, Locale: "en", Status: model.TermPreferred}},
			}))
		}
		return p.Terms()
	}
	stockedTerms := addConcepts(stocked, "cart", "checkout", "wishlist")
	otherTerms := addConcepts(other, "basket")
	_, err := app.ProjectDB(ctx, empty)
	require.NoError(t, err)

	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	missing := workspace.ProjectKey(projectIDFor("ctxops-terms-missing"))
	_, err = ws.Register(ctx, missing, "ctxops-terms-missing", "")
	require.NoError(t, err)
	broken := workspace.ProjectKey(projectIDFor("ctxops-terms-broken"))
	_, err = ws.Register(ctx, broken, "ctxops-terms-broken", "")
	require.NoError(t, err)
	brokenStore := filepath.Join(workspaceRoot, workspace.ProjectsDirName, workspace.FileNameFor(broken)+".db")
	require.NoError(t, os.WriteFile(brokenStore, []byte("not a database"), 0o600))

	standings, err := app.ContextStandings(ctx)
	require.NoError(t, err, "a store that cannot be read fails nothing")
	byName := map[string]ContextProjectStanding{}
	for _, s := range standings {
		byName[s.ProjectName] = s
	}

	listed, err := stockedTerms.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, listed)
	assert.Equal(t, listed, byName["ctxops-terms-stocked"].Terms, "the figure is what the terms store lists")
	otherListed, err := otherTerms.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, otherListed, byName["ctxops-terms-other"].Terms)
	total := 0
	for _, s := range standings {
		total += s.Terms
	}
	assert.Equal(t, listed+otherListed, total, "each project counts its own store")

	assert.Zero(t, byName["ctxops-terms-empty"].Terms, "a store with no concepts counts none")
	assert.Zero(t, byName["ctxops-terms-missing"].Terms, "a project with no store counts none")
	assert.Zero(t, byName["ctxops-terms-broken"].Terms, "a store that cannot be read counts none")
	held, err := ws.HasContext(ctx, missing)
	require.NoError(t, err)
	assert.False(t, held, "reading the standings creates no store")
}

// TestLatestActivity covers what the host API stamps with the wall clock: a
// session gone quiet, a marker that hides earlier items, a dropped suggestion,
// and a person's work grouped by day.
func TestLatestActivity(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ago := func(m int) time.Time { return now.Add(-time.Duration(m) * time.Minute) }
	key := workspace.ProjectKey("prj_standing")
	other := workspace.ProjectKey("prj_other")
	codex := contextop.Actor{Kind: contextop.ActorAgent, Name: "codex", Session: "c1"}
	claude := contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "k1"}
	person := contextop.Actor{Kind: contextop.ActorPerson}
	name := contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: "Kapi Mart", Replacement: "KapiMart"}}
	word := contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: "basket", Replacement: "cart"}}
	note := contextop.Subject{Kind: contextop.SubjectNote, Text: "The store copy addresses the shopper as you."}
	observe := func(id string, actor contextop.Actor, at time.Time, s contextop.Subject, status contextop.Status) contextop.Record {
		return contextop.Record{ID: id, Project: key, Kind: contextop.KindObserve, Actor: actor, At: at, Subject: s, Status: status}
	}

	tests := []struct {
		name    string
		records []contextop.Record // newest first
		since   time.Time
		want    *ContextActivity
	}{
		{
			name: "nothing recorded",
			records: []contextop.Record{
				{ID: "01", Project: other, Kind: contextop.KindObserve, Actor: codex, At: ago(1), Subject: name, Status: contextop.StatusSuggested},
			},
		},
		{
			name: "a session still recording is working",
			records: []contextop.Record{
				observe("03", codex, ago(1), name, contextop.StatusSuggested),
				observe("02", claude, ago(30), word, contextop.StatusSuggested),
			},
			want: &ContextActivity{Actor: codex, At: ago(1), Working: true, Recorded: 1, NewByTheme: map[string]int{DigestThemeNames: 1}},
		},
		{
			name: "a session silent for two minutes is not",
			records: []contextop.Record{
				observe("02", codex, ago(2), name, contextop.StatusSuggested),
				observe("01", codex, ago(5), word, contextop.StatusSuggested),
			},
			want: &ContextActivity{Actor: codex, At: ago(2), Recorded: 2, NewByTheme: map[string]int{DigestThemeNames: 1, DigestThemeWords: 1}},
		},
		{
			name: "a keep after the session does not displace it",
			records: []contextop.Record{
				{ID: "04", Project: key, Kind: contextop.KindKeep, Target: "02", Actor: person, At: ago(1)},
				{ID: "03", Project: key, Kind: contextop.KindCorrect, Actor: claude, At: ago(20), Subject: word, Status: contextop.StatusSuggested,
					Correction: &contextop.Correction{From: "basket", To: "cart"}},
				observe("02", claude, ago(25), name, contextop.StatusEstablished),
			},
			want: &ContextActivity{Actor: claude, At: ago(20), Recorded: 1, Corrected: 1, NewByTheme: map[string]int{DigestThemeNames: 1, DigestThemeWords: 1}},
		},
		{
			name: "the marker and a drop leave items out of what is new",
			records: []contextop.Record{
				observe("03", claude, ago(10), note, contextop.StatusSuggested),
				observe("02", claude, ago(20), word, contextop.StatusDropped),
				observe("01", claude, ago(40), name, contextop.StatusSuggested),
			},
			since: ago(30),
			want:  &ContextActivity{Actor: claude, At: ago(10), Recorded: 3, NewByTheme: map[string]int{DigestThemeWriting: 1}},
		},
		{
			name: "a person's work groups by day and is never working",
			records: []contextop.Record{
				observe("02", person, ago(1), name, contextop.StatusSuggested),
				observe("01", person, now.Add(-36*time.Hour), word, contextop.StatusSuggested),
			},
			want: &ContextActivity{Actor: person, At: ago(1), Recorded: 1, NewByTheme: map[string]int{DigestThemeNames: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, latestActivity(tt.records, key, tt.since, now))
		})
	}
}

// TestContextWorkspaceRules_WidenedAsKept: a rule kept and widened to the
// workspace is listed with the project it was decided in and how, and a rule
// kept in the project alone is not.
func TestContextWorkspaceRules_WidenedAsKept(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-widened")
	ctx := t.Context()

	none, err := app.ContextWorkspaceRules(ctx)
	require.NoError(t, err)
	assert.Empty(t, none)

	widened := proposeUtilise(t, app, root, agentIn("s1"))
	_, err = app.KeepContextOperations(ctx, ContextKeepRequest{
		Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root),
		IDs: []string{widened.ID}, WidenTo: WidenToWorkspace,
	})
	require.NoError(t, err)
	local, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(root),
		Term: "farewell", InsteadOf: []string{"Goodbye"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "farewell", Quote: "Goodbye"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{local.ID}})
	require.NoError(t, err)

	rules, err := app.ContextWorkspaceRules(ctx)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	r := rules[0]
	assert.Equal(t, widened.ID, r.ID)
	assert.Equal(t, contextop.ShortID(widened.ID), r.Short)
	assert.Equal(t, "Write use, not utilise.", r.Sentence)
	assert.Equal(t, "ctxops-widened", r.ProjectName)
	assert.Equal(t, []string{"kept by you"}, r.How)
	assert.False(t, r.At.IsZero())
}

// TestContextDigest_ACorrectionAndItsOppositeAreTwoSides: a correction that
// suggests a rule is evidence against the opposite suggestion and a rival to
// it at once. The digest lists one disagreement with both rules as its sides,
// so a person chooses between them.
func TestContextDigest_ACorrectionAndItsOppositeAreTwoSides(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-correction-rival")
	ctx := t.Context()

	corrected, err := app.RecordContextCorrection(ctx, ContextCorrectRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), From: "log in", To: "sign in", Suggest: true,
	})
	require.NoError(t, err)
	opposite, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(root), Term: "log in", InsteadOf: []string{"sign in"},
	})
	require.NoError(t, err)

	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, digest.Conflicts, 1)
	c := digest.Conflicts[0]
	assert.False(t, c.ByEvidence)
	ids := []string{}
	for _, side := range c.Sides {
		ids = append(ids, side.ID)
		assert.True(t, side.Keepable)
	}
	assert.ElementsMatch(t, []string{corrected.ID, opposite.ID}, ids)
	assert.Equal(t, 1, digest.Numbers.Conflicts)
}
