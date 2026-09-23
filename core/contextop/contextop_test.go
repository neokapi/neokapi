package contextop_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openWorkspace opens a real local workspace in a directory of its own. Every
// test here drives the log through the backend it will use in production.
func openWorkspace(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.OpenLocal(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func person(name string) contextop.Actor {
	return contextop.Actor{Kind: contextop.ActorPerson, Name: name}
}

func agent(name, session string) contextop.Actor {
	return contextop.Actor{Kind: contextop.ActorAgent, Name: name, Session: session}
}

func termRule(term, replacement, severity string) contextop.Subject {
	return contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{
		Term: term, Replacement: replacement, Severity: severity,
	}}
}

func TestLedger_AppendAndRead(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	written, err := ledger.Append(ctx, contextop.Record{
		Project:  "prj_docs",
		Actor:    agent("claude", "s1"),
		Kind:     contextop.KindObserve,
		Subject:  termRule("utilise", "use", "major"),
		Evidence: []contextop.Evidence{{Path: "docs/guide.md", Unit: "p1", Quote: "we utilise it"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "1", written.ID, "the log's position is the operation's id")
	assert.Equal(t, contextop.StatusSuggested, written.Status, "a proposal starts as a candidate")
	assert.False(t, written.At.IsZero(), "the operation is stamped from Go's clock")
	assert.Equal(t, contextop.LevelProject, written.Scope.Level, "a rule starts scoped to its project")

	read, err := ledger.Get(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, written.Subject, read.Subject)
	assert.Equal(t, written.Evidence, read.Evidence)
	assert.Equal(t, written.Actor, read.Actor)

	_, err = ledger.Get(ctx, "99")
	assert.ErrorIs(t, err, contextop.ErrNotFound)
}

func TestLedger_FoldsStatusFromLaterOperations(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		act  contextop.Kind
		want contextop.Status
	}{
		{"keeping establishes a suggestion", contextop.KindKeep, contextop.StatusEstablished},
		{"dropping sets it aside", contextop.KindDrop, contextop.StatusDropped},
		{"reverting undoes it", contextop.KindRevert, contextop.StatusReverted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)
			proposed, err := ledger.Append(ctx, contextop.Record{
				Project: "prj_docs", Actor: agent("claude", "s1"),
				Kind: contextop.KindObserve, Subject: termRule("utilise", "use", ""),
			})
			require.NoError(t, err)

			_, err = ledger.Append(ctx, contextop.Record{
				Project: "prj_docs", Actor: person("asgeir"), Kind: tt.act, Target: proposed.ID,
			})
			require.NoError(t, err)

			folded, err := ledger.Get(ctx, proposed.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, folded.Status)
			assert.Equal(t, tt.want.Answers(), folded.Status.Answers())
		})
	}
}

func TestLedger_ConfirmCarriesEditsAndScope(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	proposed, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindObserve, Subject: termRule("utilise", "use", "minor"),
	})
	require.NoError(t, err)

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: proposed.ID,
		Subject: termRule("utilise", "use", "critical"),
		Scope:   contextop.Scope{Level: contextop.LevelWorkspace},
	})
	require.NoError(t, err)

	folded, err := ledger.Get(ctx, proposed.ID)
	require.NoError(t, err)
	rule, ok := folded.Rule()
	require.True(t, ok)
	assert.Equal(t, "critical", rule.Severity, "a confirmation's edit replaces the proposed rule")
	assert.Equal(t, contextop.LevelWorkspace, folded.Scope.Level, "keeping can widen in the same step")
}

func TestLedger_RevertingASessionUndoesEverythingItRecorded(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	for _, term := range []string{"utilise", "leverage", "synergy"} {
		_, err := ledger.Append(ctx, contextop.Record{
			Project: "prj_docs", Actor: agent("claude", "s1"),
			Kind: contextop.KindObserve, Subject: termRule(term, "use", ""),
		})
		require.NoError(t, err)
	}
	kept, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s2"),
		Kind: contextop.KindObserve, Subject: termRule("utilize", "use", ""),
	})
	require.NoError(t, err)

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"), Kind: contextop.KindRevert, TargetSession: "s1",
	})
	require.NoError(t, err)

	reverted, err := ledger.Records(ctx, contextop.Filter{Session: "s1", Subjects: true})
	require.NoError(t, err)
	require.Len(t, reverted, 3)
	for _, r := range reverted {
		assert.Equal(t, contextop.StatusReverted, r.Status)
	}

	other, err := ledger.Get(ctx, kept.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusSuggested, other.Status, "another session is untouched")
}

func TestLedger_RevertReachesThroughADecision(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	proposed, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"),
		Kind: contextop.KindObserve, Subject: termRule("utilise", "use", ""),
	})
	require.NoError(t, err)
	confirmed, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: proposed.ID,
	})
	require.NoError(t, err)

	behind, err := ledger.Subject(ctx, confirmed.ID)
	require.NoError(t, err)
	assert.Equal(t, proposed.ID, behind.ID, "naming a decision reaches the rule it decided")

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"), Kind: contextop.KindRevert, Target: confirmed.ID,
	})
	require.NoError(t, err)
	folded, err := ledger.Get(ctx, proposed.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusReverted, folded.Status)
}

func TestLedger_Filters(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	_, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindObserve, Subject: termRule("utilise", "use", ""),
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_web", Actor: person("asgeir"),
		Kind: contextop.KindObserve, Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "we address the reader as you"},
	})
	require.NoError(t, err)

	tests := []struct {
		name   string
		filter contextop.Filter
		want   int
	}{
		{"everything", contextop.Filter{}, 2},
		{"one project", contextop.Filter{Project: "prj_docs"}, 1},
		{"one session", contextop.Filter{Session: "s1"}, 1},
		{"one actor by name", contextop.Filter{Actor: "asgeir"}, 1},
		{"one actor by kind", contextop.Filter{Actor: "agent"}, 1},
		{"one status", contextop.Filter{Status: contextop.StatusSuggested}, 2},
		{"one kind", contextop.Filter{Kinds: []contextop.Kind{contextop.KindCorrect}}, 0},
		{"a limit", contextop.Filter{Limit: 1}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ledger.Records(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}

	newest, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	require.Len(t, newest, 2)
	assert.Equal(t, "2", newest[0].ID, "a log reads newest first")
}

func TestLedger_Session(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	for _, term := range []string{"utilise", "leverage"} {
		_, err := ledger.Append(ctx, contextop.Record{
			Project: "prj_docs", Actor: agent("claude", "s1"),
			Kind: contextop.KindObserve, Subject: termRule(term, "use", ""),
		})
		require.NoError(t, err)
	}
	_, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindObserve, Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "the product is written Kapi"},
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: "1",
	})
	require.NoError(t, err)

	summary, err := ledger.Session(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Operations)
	assert.Equal(t, workspace.ProjectKey("prj_docs"), summary.Project)
	assert.Equal(t, 3, summary.ByKind[contextop.KindObserve])
	assert.Equal(t, 1, summary.ByStatus[contextop.StatusEstablished])
	assert.Equal(t, 2, summary.ByStatus[contextop.StatusSuggested])
	assert.False(t, summary.First.IsZero())

	empty, err := ledger.Session(ctx, "s-unknown")
	require.NoError(t, err)
	assert.Zero(t, empty.Operations, "a session that did nothing here summarizes as nothing")

	_, err = ledger.Session(ctx, "  ")
	assert.Error(t, err)
}

func TestLedger_RejectsAnUnknownKindAndAMissingTarget(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	_, err := ledger.Append(ctx, contextop.Record{Project: "p", Kind: "ponder"})
	require.ErrorContains(t, err, "not an operation kind")

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "p", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: "42",
	})
	assert.ErrorIs(t, err, contextop.ErrNotFound)
}

func TestParseID(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "7", want: 7},
		{in: "#7", want: 7},
		{in: "  7 ", want: 7},
		{in: "0", wantErr: true},
		{in: "-1", wantErr: true},
		{in: "seven", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := contextop.ParseID(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.in != "  7 " && tt.in != "#7", contextop.FormatID(got) == tt.in)
		})
	}
}

func TestSubjectDescribe(t *testing.T) {
	tests := []struct {
		name    string
		subject contextop.Subject
		want    string
	}{
		{"a term rule with a replacement", termRule("utilise", "use", ""), `term "use", not "utilise"`},
		{"a term rule without one", termRule("utilise", "", ""), `term "utilise"`},
		{
			"a term rule with other forms",
			contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{
				Term: "Quick cast", Forms: []string{"QuickCast"}, Replacement: "Quickcast",
			}},
			`term "Quickcast", not "Quick cast", "QuickCast"`,
		},
		{
			"a content-memory pair",
			contextop.Subject{Kind: contextop.SubjectMemory, Memory: &contextop.MemoryPair{
				Source: "Save", Target: "Lagre", TargetLocale: "nb-NO",
			}},
			`memory "Save" into nb-NO`,
		},
		{"a note", contextop.Subject{Kind: contextop.SubjectNote, Text: "we say you"}, `note "we say you"`},
		{"nothing", contextop.Subject{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.subject.Describe())
		})
	}
}
