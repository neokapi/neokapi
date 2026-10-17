package contextop_test

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(n int) time.Time { return time.Date(2026, 3, n, 9, 0, 0, 0, time.UTC) }

// atlasPlaces declares the Nordics as a territory and Oslo as a city within
// its county.
var atlasPlaces = func() *place.Gazetteer {
	g, err := place.New(map[string]place.Place{
		"nordics": {Name: "Nordics", Of: []string{"no", "se", "dk", "fi", "is"}},
		"oslo":    {Name: "Oslo", Type: place.TypeLocality, Within: []string{"no-03"}},
	})
	if err != nil {
		panic(err)
	}
	return g
}()

var atlasSpace = axis.NewSpace(map[string]axis.Spec{
	"market": {Kind: axis.KindGeo},
	"age":    {Kind: axis.KindRange, Unit: "years"},
}, atlasPlaces)

func TestScope_CoversIn(t *testing.T) {
	tests := []struct {
		name  string
		scope map[string]string
		point map[string]string
		want  bool
	}{
		{"geo made-of place answers at a member", map[string]string{"market": "nordics"}, map[string]string{"market": "no"}, true},
		{"geo country answers at itself", map[string]string{"market": "no"}, map[string]string{"market": "no"}, true},
		{"geo made-of place is silent outside it", map[string]string{"market": "nordics"}, map[string]string{"market": "de"}, false},
		{"geo country is silent at another", map[string]string{"market": "no"}, map[string]string{"market": "de"}, false},
		{"geo country is silent at its region", map[string]string{"market": "no"}, map[string]string{"market": "nordics"}, false},
		{"geo country answers at a declared city within it", map[string]string{"market": "no"}, map[string]string{"market": "oslo"}, true},
		{"geo made-of place answers at a declared city", map[string]string{"market": "nordics"}, map[string]string{"market": "oslo"}, true},
		{"geo subdivision answers at a declared city within it", map[string]string{"market": "no-03"}, map[string]string{"market": "oslo"}, true},
		{"geo declared city is silent at its country", map[string]string{"market": "oslo"}, map[string]string{"market": "no"}, false},
		{"geo subdivision is silent at a sibling", map[string]string{"market": "no-46"}, map[string]string{"market": "oslo"}, false},
		{"range answers inside", map[string]string{"age": "13-17"}, map[string]string{"age": "14-16"}, true},
		{"range is silent outside", map[string]string{"age": "13-17"}, map[string]string{"age": "18-"}, false},
		{"untyped axis is equality", map[string]string{"brand": "a"}, map[string]string{"brand": "a"}, true},
		{"untyped axis differs", map[string]string{"brand": "a"}, map[string]string{"brand": "b"}, false},
		{"axis the point lacks", map[string]string{"market": "nordics"}, map[string]string{}, false},
		{"scope without axes covers everything", nil, map[string]string{"market": "de"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := contextop.Scope{Coordinates: tt.scope}
			assert.Equal(t, tt.want, s.CoversIn(atlasSpace, tt.point))
		})
	}
	// Covers stays equality on every axis.
	assert.False(t, contextop.Scope{Coordinates: map[string]string{"market": "nordics"}}.Covers(map[string]string{"market": "no"}))
}

func TestScope_InForce(t *testing.T) {
	s := contextop.Scope{ValidFrom: new(day(10)), ValidTo: new(day(20))}
	assert.True(t, s.InForce(time.Time{}), "the zero instant is the as-declared view")
	assert.False(t, s.InForce(day(9)))
	assert.True(t, s.InForce(day(10)), "valid_from is inclusive")
	assert.True(t, s.InForce(day(19)))
	assert.False(t, s.InForce(time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC)), "valid_to is exclusive")
	assert.True(t, contextop.Scope{}.InForce(day(1)))
	assert.Equal(t, "project market=nordics from 2026-03-10 until 2026-03-20",
		contextop.Scope{
			Level:       contextop.LevelProject,
			Coordinates: map[string]string{"market": "nordics"},
			ValidFrom:   new(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)),
			ValidTo:     new(time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)),
		}.Describe())
}

func TestResolve_TypedAxesAndWindows(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)
	add := func(subject contextop.Subject, scope contextop.Scope) {
		t.Helper()
		_, err := ledger.Append(ctx, contextop.Record{
			Project: "prj", Actor: person("ana"), Kind: contextop.KindObserve, Subject: subject, Scope: scope,
		})
		require.NoError(t, err)
	}
	add(termRule("Quick cast", "Quickcast", false), contextop.Scope{Level: contextop.LevelProject, Coordinates: map[string]string{"market": "nordics"}})
	add(termRule("kiddo", "young person", false), contextop.Scope{Level: contextop.LevelProject, Coordinates: map[string]string{"age": "13-17"}})
	add(termRule("Spring sale", "Summer sale", false), contextop.Scope{Level: contextop.LevelProject, ValidTo: new(day(15))})
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)

	terms := func(req contextop.ResolveRequest) []string {
		req.Project, req.Space = "prj", atlasSpace
		var out []string
		for _, r := range contextop.Resolve(records, nil, req).Advisory {
			out = append(out, r.Term)
		}
		return out
	}
	assert.Contains(t, terms(contextop.ResolveRequest{Coordinates: map[string]string{"market": "no"}}), "Quick cast")
	assert.NotContains(t, terms(contextop.ResolveRequest{Coordinates: map[string]string{"market": "de"}}), "Quick cast")
	assert.Contains(t, terms(contextop.ResolveRequest{Coordinates: map[string]string{"age": "14-16"}}), "kiddo")
	assert.NotContains(t, terms(contextop.ResolveRequest{Coordinates: map[string]string{"age": "18-"}}), "kiddo")
	assert.Contains(t, terms(contextop.ResolveRequest{At: day(14)}), "Spring sale")
	assert.NotContains(t, terms(contextop.ResolveRequest{At: day(15)}), "Spring sale", "a rule past its valid_to stops answering")
	assert.Contains(t, terms(contextop.ResolveRequest{}), "Spring sale", "the as-declared view holds every window")
}

func TestLedger_KeepWithWindowFoldsOntoTheRule(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)
	noted, err := ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false),
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: person("ana"), Kind: contextop.KindKeep, Target: noted.ID,
		Scope: contextop.Scope{Level: contextop.LevelProject, ValidTo: new(day(20))},
	})
	require.NoError(t, err)
	got, err := ledger.Get(ctx, noted.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, got.Status)
	require.NotNil(t, got.Scope.ValidTo)
	assert.True(t, got.Scope.ValidTo.Equal(day(20)))
}

func TestLedger_RecordsAsOf(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)
	noted, err := ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve,
		Subject: termRule("utilise", "use", false), At: day(1),
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("ana"), Kind: contextop.KindKeep, Target: noted.ID, At: day(3)})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("ana"), Kind: contextop.KindDrop, Target: noted.ID, At: day(5)})
	require.NoError(t, err)

	statusAt := func(at time.Time) contextop.Status {
		t.Helper()
		rs, err := ledger.RecordsAsOf(ctx, at, contextop.Filter{Subjects: true})
		require.NoError(t, err)
		if len(rs) == 0 {
			return ""
		}
		return rs[0].Status
	}
	assert.Equal(t, contextop.Status(""), statusAt(day(1).Add(-time.Hour)), "nothing was recorded yet")
	assert.Equal(t, contextop.StatusSuggested, statusAt(day(2)))
	assert.Equal(t, contextop.StatusEstablished, statusAt(day(3)), "the instant itself is included")
	assert.Equal(t, contextop.StatusEstablished, statusAt(day(4)))
	assert.Equal(t, contextop.StatusDropped, statusAt(day(6)))
}

func TestLedger_Histories(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)
	append_ := func(r contextop.Record) contextop.Record {
		t.Helper()
		r.Project = "prj"
		out, err := ledger.Append(ctx, r)
		require.NoError(t, err)
		return out
	}
	kept := append_(contextop.Record{Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false), At: day(1)})
	append_(contextop.Record{Actor: person("ana"), Kind: contextop.KindKeep, Target: kept.ID, At: day(2)})
	append_(contextop.Record{Actor: person("ana"), Kind: contextop.KindDrop, Target: kept.ID, At: day(4)})

	a := append_(contextop.Record{Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("signon", "sign in", false), At: day(5)})
	b := append_(contextop.Record{Actor: agent("claude", "s2"), Kind: contextop.KindObserve, Subject: termRule("signon", "log in", false), At: day(6)})

	late := append_(contextop.Record{Actor: agent("claude", "s3"), Kind: contextop.KindObserve, Subject: termRule("e-mail", "email", false), At: day(7)})
	append_(contextop.Record{Actor: person("ana"), Kind: contextop.KindReset, Before: late.ID, At: day(8)})

	hist, complete, err := ledger.Histories(ctx, contextop.Filter{Project: "prj"}, 0)
	require.NoError(t, err)
	assert.True(t, complete)

	assert.Equal(t, []contextop.StatusAt{
		{At: day(1), Status: contextop.StatusSuggested},
		{At: day(2), Status: contextop.StatusEstablished},
		{At: day(4), Status: contextop.StatusDropped},
	}, hist[kept.ID])
	assert.Equal(t, []contextop.StatusAt{
		{At: day(5), Status: contextop.StatusSuggested},
		{At: day(6), Status: contextop.StatusContested},
	}, hist[a.ID], "a rival recorded later contests it from that instant")
	assert.Equal(t, []contextop.StatusAt{{At: day(6), Status: contextop.StatusContested}}, hist[b.ID])
	assert.Equal(t, []contextop.StatusAt{
		{At: day(7), Status: contextop.StatusSuggested},
		{At: day(8), Status: contextop.StatusReset},
	}, hist[late.ID])
}

func TestLedger_HistoriesBounded(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)
	noted, err := ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false), At: day(1),
	})
	require.NoError(t, err)
	for i := 2; i <= 6; i++ {
		_, err := ledger.Append(ctx, contextop.Record{
			Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve,
			Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "note"}, At: day(i),
		})
		require.NoError(t, err)
	}
	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("ana"), Kind: contextop.KindKeep, Target: noted.ID, At: day(9)})
	require.NoError(t, err)

	hist, complete, err := ledger.Histories(ctx, contextop.Filter{Project: "prj"}, 3)
	require.NoError(t, err)
	assert.False(t, complete, "seven instants is past a limit of three")
	assert.Equal(t, []contextop.StatusAt{
		{At: day(1), Status: contextop.StatusSuggested},
		{At: day(9), Status: contextop.StatusEstablished},
	}, hist[noted.ID], "past the limit a subject reports its birth and where it stands now")
	assert.Len(t, hist, 6)
}
