package contextop

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/axis"
)

// Two rules disagree only where they can both answer: at a common point and a
// common instant.
func TestMeet_TypedAxesAndWindows(t *testing.T) {
	space := axis.Space{
		"market": {Kind: axis.KindGeo, Groups: map[string][]string{"nordics": {"no", "se"}}},
		"age":    {Kind: axis.KindRange},
	}
	day := func(d int) *time.Time { at := time.Date(2026, 6, d, 0, 0, 0, 0, time.UTC); return &at }
	at := func(coords map[string]string, from, to *time.Time) Record {
		return Record{Project: "p", Scope: Scope{Coordinates: coords, ValidFrom: from, ValidTo: to}, space: space}
	}

	tests := []struct {
		name string
		a, b Record
		want bool
	}{
		{"a region and a country inside it", at(map[string]string{"market": "nordics"}, nil, nil), at(map[string]string{"market": "no"}, nil, nil), true},
		{"a region and a country outside it", at(map[string]string{"market": "nordics"}, nil, nil), at(map[string]string{"market": "de"}, nil, nil), false},
		{"overlapping age ranges", at(map[string]string{"age": "13-17"}, nil, nil), at(map[string]string{"age": "16-20"}, nil, nil), true},
		{"disjoint age ranges", at(map[string]string{"age": "13-17"}, nil, nil), at(map[string]string{"age": "18-"}, nil, nil), false},
		{"one window ends the day the other begins", at(nil, nil, day(10)), at(nil, day(10), nil), false},
		{"windows that overlap", at(nil, day(1), day(10)), at(nil, day(5), day(20)), true},
		{"an open window and a bounded one", at(nil, nil, nil), at(nil, day(5), day(6)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, meet(tt.a, tt.b))
			assert.Equal(t, tt.want, meet(tt.b, tt.a), "meeting is symmetric")
		})
	}

	// Without a space, the same two points are compared exactly.
	plain := at(map[string]string{"market": "nordics"}, nil, nil)
	plain.space = nil
	other := at(map[string]string{"market": "no"}, nil, nil)
	other.space = nil
	assert.False(t, meet(plain, other))
}

// A correction seen in Norway is evidence for a suggestion scoped to the
// Nordics, and not for one scoped to Germany.
func TestSeenWithin_TypedAxes(t *testing.T) {
	space := axis.Space{"market": {Kind: axis.KindGeo, Groups: map[string][]string{"nordics": {"no", "se"}}}}
	suggestion := func(market string) Record {
		return Record{Project: "p", Scope: Scope{Coordinates: map[string]string{"market": market}}, space: space}
	}
	correction := Record{Project: "p", Scope: Scope{Coordinates: map[string]string{"market": "no"}}}

	assert.True(t, seenWithin(suggestion("nordics"), correction))
	assert.True(t, seenWithin(suggestion("no"), correction))
	assert.False(t, seenWithin(suggestion("de"), correction))
}
