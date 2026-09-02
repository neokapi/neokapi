package ref

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		name   string
		local  Ref
		remote Ref
		want   map[Component]Status
	}{
		{
			name:   "two zero refs know nothing about the identities and agree on the position",
			local:  Ref{},
			remote: Ref{},
			want: map[Component]Status{
				ComponentContent:   StatusCurrent,
				ComponentContext:   StatusUnknown,
				ComponentTerms:     StatusUnknown,
				ComponentDecisions: StatusUnknown,
			},
		},
		{
			name:   "identical refs are current throughout",
			local:  Ref{Content: 7, Context: "c", Terms: "t", Decisions: "d"},
			remote: Ref{Content: 7, Context: "c", Terms: "t", Decisions: "d"},
			want: map[Component]Status{
				ComponentContent:   StatusCurrent,
				ComponentContext:   StatusCurrent,
				ComponentTerms:     StatusCurrent,
				ComponentDecisions: StatusCurrent,
			},
		},
		{
			name:   "a trailing position is behind and disturbs no identity",
			local:  Ref{Content: 3, Context: "c", Terms: "t", Decisions: "d"},
			remote: Ref{Content: 9, Context: "c", Terms: "t", Decisions: "d"},
			want: map[Component]Status{
				ComponentContent:   StatusBehind,
				ComponentContext:   StatusCurrent,
				ComponentTerms:     StatusCurrent,
				ComponentDecisions: StatusCurrent,
			},
		},
		{
			name:   "a leading position is ahead",
			local:  Ref{Content: 9},
			remote: Ref{Content: 3},
			want: map[Component]Status{
				ComponentContent:   StatusAhead,
				ComponentContext:   StatusUnknown,
				ComponentTerms:     StatusUnknown,
				ComponentDecisions: StatusUnknown,
			},
		},
		{
			name:   "one identity moves and the others stay put",
			local:  Ref{Content: 4, Context: "c", Terms: "t1", Decisions: "d"},
			remote: Ref{Content: 4, Context: "c", Terms: "t2", Decisions: "d"},
			want: map[Component]Status{
				ComponentContent:   StatusCurrent,
				ComponentContext:   StatusCurrent,
				ComponentTerms:     StatusMoved,
				ComponentDecisions: StatusCurrent,
			},
		},
		{
			name:   "an identity only one side claims is unknown, not moved",
			local:  Ref{Context: "c"},
			remote: Ref{},
			want: map[Component]Status{
				ComponentContent:   StatusCurrent,
				ComponentContext:   StatusUnknown,
				ComponentTerms:     StatusUnknown,
				ComponentDecisions: StatusUnknown,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.local, tt.remote)
			for _, c := range got.All() {
				assert.Equal(t, tt.want[c.Component], c.Status, "component %s", c.Component)
			}
		})
	}
}

func TestDivergenceCurrentAndMoved(t *testing.T) {
	t.Run("unknown identities do not make a divergence", func(t *testing.T) {
		d := Compare(Ref{Content: 5}, Ref{Content: 5})
		assert.True(t, d.Current())
		assert.Empty(t, d.Moved())
	})

	t.Run("a moved position is not moved governance", func(t *testing.T) {
		d := Compare(Ref{Content: 1, Context: "c"}, Ref{Content: 42, Context: "c"})
		assert.False(t, d.Current())
		assert.Empty(t, d.Moved(), "content traffic must never report as moved governance")
	})

	t.Run("moved names every differing identity in report order", func(t *testing.T) {
		d := Compare(
			Ref{Context: "c1", Terms: "t1", Decisions: "d1"},
			Ref{Context: "c2", Terms: "t1", Decisions: "d2"},
		)
		assert.False(t, d.Current())
		assert.Equal(t, []Component{ComponentContext, ComponentDecisions}, d.Moved())
	})
}

func TestAdvanceIsMonotonic(t *testing.T) {
	tests := []struct {
		name   string
		start  int64
		to     int64
		expect int64
	}{
		{name: "forward moves", start: 5, to: 9, expect: 9},
		{name: "backward is refused", start: 9, to: 5, expect: 9},
		{name: "an absent position in an answer is not a rewind", start: 9, to: 0, expect: 9},
		{name: "equal is a no-op", start: 9, to: 9, expect: 9},
		{name: "first position is taken", start: 0, to: 3, expect: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ref{Content: tt.start, Context: "c"}.Advance(tt.to)
			assert.Equal(t, tt.expect, got.Content)
			assert.Equal(t, "c", got.Context, "advancing the position must not touch an identity")
		})
	}
}

func TestAssert(t *testing.T) {
	tests := []struct {
		name        string
		expected    string
		actual      string
		wantConflct bool
	}{
		{name: "no assertion always passes", expected: "", actual: "anything"},
		{name: "matching assertion passes", expected: "h", actual: "h"},
		{name: "moved identity conflicts", expected: "h1", actual: "h2", wantConflct: true},
		{name: "an identity that vanished conflicts", expected: "h1", actual: "", wantConflct: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Assert(ComponentTerms, tt.expected, tt.actual)
			if !tt.wantConflct {
				require.NoError(t, err)
				return
			}
			var conflict *Conflict
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, ComponentTerms, conflict.Component)
			assert.Contains(t, err.Error(), "Pull first")
		})
	}
}

func TestFoldIsOrderIndependentAndSensitive(t *testing.T) {
	a := Fold(map[string]string{"one": "1", "two": "2"})
	b := Fold(map[string]string{"two": "2", "one": "1"})
	assert.Equal(t, a, b, "a fold must not depend on map iteration order")

	assert.NotEqual(t, a, Fold(map[string]string{"one": "1", "two": "3"}), "a changed member must change the fold")
	assert.NotEqual(t, a, Fold(map[string]string{"one": "1"}), "a dropped member must change the fold")
	assert.NotEqual(t, a, Fold(map[string]string{"one": "1", "two": "2", "three": "3"}), "an added member must change the fold")

	// The empty fold is a real value: a side holding nothing and a side holding
	// nothing agree, rather than both reporting "unknown" forever.
	assert.NotEmpty(t, Fold(nil))
	assert.Equal(t, Fold(nil), Fold(map[string]string{}))
}

func TestIdentityAccessors(t *testing.T) {
	r := Ref{Content: 3, Context: "c", Terms: "t", Decisions: "d"}
	assert.Equal(t, "c", r.Identity(ComponentContext))
	assert.Equal(t, "t", r.Identity(ComponentTerms))
	assert.Equal(t, "d", r.Identity(ComponentDecisions))
	assert.Empty(t, r.Identity(ComponentContent), "the position has no identity")

	assert.Equal(t, "c2", r.WithIdentity(ComponentContext, "c2").Context)
	assert.Equal(t, r, r.WithIdentity(ComponentContent, "ignored"))

	assert.True(t, Ref{}.IsZero())
	assert.False(t, r.IsZero())
}
