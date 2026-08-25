package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinateFilterMatches(t *testing.T) {
	tests := []struct {
		name  string
		f     CoordinateFilter
		point map[string]string
		want  bool
	}{
		{
			name:  "unconstrained filter reaches every point",
			f:     nil,
			point: map[string]string{"brand": "acme", "channel": "support"},
			want:  true,
		},
		{
			name:  "unconstrained filter reaches the default point",
			f:     nil,
			point: nil,
			want:  true,
		},
		{
			name:  "named axis satisfied",
			f:     CoordinateFilter{"brand": "acme"},
			point: map[string]string{"brand": "acme", "channel": "support"},
			want:  true,
		},
		{
			name:  "unnamed axes are free",
			f:     CoordinateFilter{"brand": "acme"},
			point: map[string]string{"brand": "acme", "product": "docs", "channel": "email"},
			want:  true,
		},
		{
			name:  "named axis contradicted",
			f:     CoordinateFilter{"brand": "acme"},
			point: map[string]string{"brand": "other"},
			want:  false,
		},
		{
			name:  "every named axis must hold",
			f:     CoordinateFilter{"brand": "acme", "channel": "support"},
			point: map[string]string{"brand": "acme", "channel": "web"},
			want:  false,
		},
		{
			// The default point is a real place with its own custodian, not a
			// wildcard: content that has not said which brand it belongs to is
			// not acme content.
			name:  "point missing a named axis does not match",
			f:     CoordinateFilter{"brand": "acme"},
			point: map[string]string{"channel": "support"},
			want:  false,
		},
		{
			name:  "default point does not match a constrained filter",
			f:     CoordinateFilter{"brand": "acme"},
			point: nil,
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.f.Matches(tt.point))
		})
	}
}

func TestCoordinateReachUnions(t *testing.T) {
	// Two narrow memberships add up rather than cancelling out.
	var reach CoordinateReach
	reach = reach.Add(CoordinateFilter{"brand": "acme"})
	reach = reach.Add(CoordinateFilter{"brand": "other", "channel": "support"})

	assert.False(t, reach.Unconstrained())
	assert.True(t, reach.Reaches(map[string]string{"brand": "acme", "channel": "web"}))
	assert.True(t, reach.Reaches(map[string]string{"brand": "other", "channel": "support"}))
	assert.False(t, reach.Reaches(map[string]string{"brand": "other", "channel": "web"}))
	assert.False(t, reach.Reaches(map[string]string{"brand": "third"}))
}

func TestCoordinateReachUnconstrainedAbsorbs(t *testing.T) {
	// A grant naming no region opens the whole space, and carrying narrower
	// filters beside it would be dead weight every later check has to walk.
	reach := CoordinateReach{}.Add(CoordinateFilter{"brand": "acme"}).Add(nil)
	assert.True(t, reach.Unconstrained())
	assert.Len(t, reach, 1)
	assert.True(t, reach.Reaches(map[string]string{"brand": "anything"}))

	// Order does not matter.
	reach = CoordinateReach{}.Add(nil).Add(CoordinateFilter{"brand": "acme"})
	assert.True(t, reach.Unconstrained())
}

func TestCoordinateReachEmptyIsEverywhere(t *testing.T) {
	// The pre-regions default: a membership written before regions existed
	// carries no filter and must keep reaching everything.
	var reach CoordinateReach
	assert.True(t, reach.Unconstrained())
	assert.True(t, reach.Reaches(map[string]string{"brand": "acme"}))
	assert.True(t, reach.Reaches(nil))
}

func TestCoordinateReachAddDeduplicates(t *testing.T) {
	reach := CoordinateReach{}.
		Add(CoordinateFilter{"brand": "acme"}).
		Add(CoordinateFilter{"brand": "acme"})
	assert.Len(t, reach, 1)
}

func TestCoordinateReachIntersect(t *testing.T) {
	tests := []struct {
		name          string
		a, b          CoordinateReach
		wantAnywhere  bool
		reaches       []map[string]string
		doesNotReach  []map[string]string
		wantUnbounded bool
	}{
		{
			name:          "unconstrained by unconstrained stays unconstrained",
			a:             nil,
			b:             nil,
			wantAnywhere:  true,
			wantUnbounded: true,
		},
		{
			name:         "unconstrained narrowed by a region takes the region",
			a:            nil,
			b:            CoordinateReach{CoordinateFilter{"brand": "acme"}},
			wantAnywhere: true,
			reaches:      []map[string]string{{"brand": "acme"}},
			doesNotReach: []map[string]string{{"brand": "other"}},
		},
		{
			name:         "agreeing filters conjoin their axes",
			a:            CoordinateReach{CoordinateFilter{"brand": "acme"}},
			b:            CoordinateReach{CoordinateFilter{"channel": "support"}},
			wantAnywhere: true,
			reaches:      []map[string]string{{"brand": "acme", "channel": "support"}},
			doesNotReach: []map[string]string{
				{"brand": "acme", "channel": "web"},
				{"brand": "other", "channel": "support"},
			},
		},
		{
			name:         "disagreeing filters reach nowhere",
			a:            CoordinateReach{CoordinateFilter{"brand": "acme"}},
			b:            CoordinateReach{CoordinateFilter{"brand": "other"}},
			wantAnywhere: false,
		},
		{
			name: "only the compatible pairs survive",
			a: CoordinateReach{
				CoordinateFilter{"brand": "acme"},
				CoordinateFilter{"brand": "other"},
			},
			b:            CoordinateReach{CoordinateFilter{"brand": "acme", "channel": "support"}},
			wantAnywhere: true,
			reaches:      []map[string]string{{"brand": "acme", "channel": "support"}},
			doesNotReach: []map[string]string{
				{"brand": "other", "channel": "support"},
				{"brand": "acme", "channel": "web"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, anywhere := tt.a.Intersect(tt.b)
			assert.Equal(t, tt.wantAnywhere, anywhere)
			if tt.wantUnbounded {
				assert.True(t, got.Unconstrained())
			}
			for _, p := range tt.reaches {
				assert.True(t, got.Reaches(p), "expected reach at %v", p)
			}
			for _, p := range tt.doesNotReach {
				assert.False(t, got.Reaches(p), "expected no reach at %v", p)
			}
		})
	}
}

func TestCoordinateReachIntersectIsSymmetric(t *testing.T) {
	a := CoordinateReach{CoordinateFilter{"brand": "acme"}}
	b := CoordinateReach{CoordinateFilter{"channel": "support"}}
	ab, okAB := a.Intersect(b)
	ba, okBA := b.Intersect(a)
	assert.Equal(t, okAB, okBA)
	assert.Equal(t, ab.String(), ba.String())
}

func TestParseCoordinateFilter(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    CoordinateFilter
		wantErr bool
	}{
		{name: "empty is unconstrained", in: "", want: nil},
		{name: "whitespace is unconstrained", in: "   ", want: nil},
		{name: "single axis", in: "brand=acme", want: CoordinateFilter{"brand": "acme"}},
		{
			name: "several axes",
			in:   "brand=acme,channel=support",
			want: CoordinateFilter{"brand": "acme", "channel": "support"},
		},
		{
			name: "whitespace is trimmed",
			in:   " brand = acme , channel = support ",
			want: CoordinateFilter{"brand": "acme", "channel": "support"},
		},
		{name: "repeating an axis with one value is fine", in: "brand=acme,brand=acme", want: CoordinateFilter{"brand": "acme"}},
		{name: "no equals", in: "brand", wantErr: true},
		{name: "empty axis", in: "=acme", wantErr: true},
		{name: "empty value", in: "brand=", wantErr: true},
		{name: "contradictory axis", in: "brand=acme,brand=other", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCoordinateFilter(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCoordinateFilterStringIsStable(t *testing.T) {
	f := CoordinateFilter{"product": "docs", "brand": "acme", "channel": "support"}
	// Sorted by axis, so audit lines and test expectations do not depend on map
	// iteration order.
	assert.Equal(t, "brand=acme,channel=support,product=docs", f.String())
	assert.Empty(t, CoordinateFilter(nil).String())
}

func TestCoordinatesRoundTripThroughStorage(t *testing.T) {
	f := CoordinateFilter{"brand": "acme", "channel": "support"}
	assert.Equal(t, f, UnmarshalCoordinates(MarshalCoordinates(f)))

	// The unconstrained filter is stored as "{}" so every row reads back the
	// same way.
	assert.Equal(t, "{}", MarshalCoordinates(nil))
	assert.Nil(t, UnmarshalCoordinates("{}"))
}

func TestUnmarshalCoordinatesFailsOpen(t *testing.T) {
	// An unreadable row must not silently become a narrower grant that quietly
	// stops routing work to someone. Unconstrained is the pre-regions behaviour.
	for _, in := range []string{"", "null", "not json", "[1,2]", `{"":"x"}`, `{"brand":""}`} {
		assert.Nil(t, UnmarshalCoordinates(in), "input %q", in)
	}
}

func TestIsCustodian(t *testing.T) {
	region := CoordinateReach{CoordinateFilter{"brand": "acme"}}

	tests := []struct {
		name  string
		perms Permission
		reach CoordinateReach
		want  bool
	}{
		{
			// Reviewing is volume work; authoring a rule is not. A reviewer
			// bounded to one brand is a contributor with a narrow beat.
			name:  "review over a region is not custody",
			perms: PermViewContent | PermReview,
			reach: region,
			want:  false,
		},
		{name: "voice over a region is custody", perms: PermManageVoice, reach: region, want: true},
		{name: "terms over a region is custody", perms: PermManageTerms, reach: region, want: true},
		{
			// Blanket authority is not custody of a region — the workspace owner
			// is not billed as a custodian of everywhere.
			name:  "coordinate-scoped power over the whole space is not custody",
			perms: PermAll,
			reach: nil,
			want:  false,
		},
		{
			name:  "non-custodial powers over a region are not custody",
			perms: PermViewContent | PermTranslate | PermManageFiles,
			reach: region,
			want:  false,
		},
		{name: "nothing at all", perms: 0, reach: region, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCustodian(tt.perms, tt.reach))
		})
	}
}
