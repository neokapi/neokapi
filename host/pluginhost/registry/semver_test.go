package registry

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.0", "1.4.0", 0},
		{"v1.4.0", "1.4.0", 0}, // leading v is optional
		{"1.4.1", "1.4.0", 1},
		{"1.4.0", "1.5.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.4", "1.4.0", 0},         // missing patch padded with .0
		{"1", "1.0.0", 0},           // missing minor+patch
		{"1.4.0-rc1", "1.4.0", 0},   // pre-release ignored for ordering
		{"1.4.0+build", "1.4.0", 0}, // build metadata ignored
		{"1.10.0", "1.9.0", 1},      // numeric (not lexical) compare
	}
	for _, c := range cases {
		if got := CompareSemver(c.a, c.b); got != c.want {
			t.Errorf("CompareSemver(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestMatchConstraint(t *testing.T) {
	cases := []struct {
		constraint, v string
		want          bool
	}{
		{"", "1.4.0", true},
		{"*", "9.9.9", true},
		{"1.4.0", "1.4.0", true},
		{"1.4.0", "1.4.1", false},
		{"^1.4.0", "1.4.0", true},
		{"^1.4.0", "1.9.9", true}, // same major, >=
		{"^1.4.0", "2.0.0", false},
		{"^1.4.0", "1.3.0", false}, // below base
		{"~1.4.0", "1.4.9", true},  // same major+minor, >=
		{"~1.4.0", "1.5.0", false}, // minor differs
		{"~1.4.0", "1.4.0", true},
		{">=1.4.0", "1.4.0", true},
		{">=1.4.0", "1.3.9", false},
		{">1.4.0", "1.4.0", false},
		{">1.4.0", "1.4.1", true},
		{"<=1.4.0", "1.4.0", true},
		{"<=1.4.0", "1.4.1", false},
		{"<1.4.0", "1.3.9", true},
		{"<1.4.0", "1.4.0", false},
		{"v1.4.0", "1.4.0", true}, // v-prefixed constraint
	}
	for _, c := range cases {
		if got := MatchConstraint(c.constraint, c.v); got != c.want {
			t.Errorf("MatchConstraint(%q,%q)=%v want %v", c.constraint, c.v, got, c.want)
		}
	}
}
