package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntroduced(t *testing.T) {
	fail := func(rule, msg string) Finding { return Finding{Rule: rule, Message: msg, Fails: true} }
	report := func(rule, msg string) Finding { return Finding{Rule: rule, Message: msg} }
	tests := []struct {
		name string
		in   CheckOutcome
		want []Finding
	}{
		{name: "nothing found", in: CheckOutcome{}, want: nil},
		{name: "a violation already there is not held against the edit",
			in:   CheckOutcome{Before: []Finding{fail("terms.vocabulary", "avoid utilize")}, After: []Finding{fail("terms.vocabulary", "avoid utilize")}},
			want: nil},
		{name: "a new failing finding is introduced",
			in:   CheckOutcome{After: []Finding{fail("terms.vocabulary", "avoid utilize")}},
			want: []Finding{fail("terms.vocabulary", "avoid utilize")}},
		{name: "a second occurrence of an existing violation is introduced",
			in:   CheckOutcome{Before: []Finding{fail("terms.vocabulary", "avoid utilize")}, After: []Finding{fail("terms.vocabulary", "avoid utilize"), fail("terms.vocabulary", "avoid utilize")}},
			want: []Finding{fail("terms.vocabulary", "avoid utilize")}},
		{name: "a reporting finding never refuses",
			in:   CheckOutcome{After: []Finding{report("voice.pattern", "long sentence")}},
			want: nil},
		{name: "a fixed violation is not reported as introduced",
			in:   CheckOutcome{Before: []Finding{fail("terms.vocabulary", "avoid utilize")}},
			want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Introduced(tc.in))
		})
	}
}
