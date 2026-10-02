package model

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The direct encoder writes exactly what Run.MarshalJSON writes, for every
// kind, every optional field, and every character encoding/json escapes.
func TestAppendRunJSON(t *testing.T) {
	tricky := []string{"", "plain", "a<b>&c", "quote\" back\\slash", "\b\f\n\r\t", "\x00\x01\x1f\x7f",
		"  ", "bad\xffutf\xfe", "\xed\xa0\x80", "Blåbær 🍓", "\"\\/"}
	var runs []Run
	for _, s := range tricky {
		runs = append(runs,
			TextR(s),
			Run{Text: &TextRun{Text: s, NoTranslate: true}},
			PhR(PlaceholderRun{ID: s, Type: "code:variable", Data: s, Equiv: s}),
			PhR(PlaceholderRun{ID: "1", Type: s, SubType: s, Data: "{n}", Equiv: "n", Disp: s, Attrs: map[string]string{s: s, "b": "2", "a": "1"},
				Constraints: &RunConstraints{Deletable: true, Reorderable: true}}),
			PcOpenR(PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="` + s + `">`, Attrs: map[string]string{"href": s}}),
			PcOpenR(PcOpenRun{ID: "2", Attrs: map[string]string{}}),
			PcCloseR(PcCloseRun{ID: s, Type: s, SubType: s, Data: s, Equiv: s}),
			PcCloseR(PcCloseRun{ID: "1"}),
			SubR(SubRun{ID: s, Ref: s, Equiv: s}),
		)
	}
	runs = append(runs,
		PluralR(PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{PluralOther: {TextR("x")}, PluralOne: {PhR(PlaceholderRun{ID: "n"}), TextR(" y")}, PluralFew: nil, PluralMany: {}}}),
		PluralR(PluralRun{Pivot: "n"}),
		SelectR(SelectRun{Pivot: "g", Cases: map[string][]Run{"other": {TextR("they")}, "female": {SelectR(SelectRun{Pivot: "h", Cases: map[string][]Run{}})}}}),
		SelectR(SelectRun{Pivot: "g"}),
	)
	for _, r := range runs {
		want, err := r.MarshalJSON()
		require.NoError(t, err)
		got, ok := appendRunJSON(nil, r)
		require.True(t, ok)
		assert.Equal(t, string(want), string(got))
	}

	for _, bad := range []Run{{}, {Text: &TextRun{}, Ph: &PlaceholderRun{}}, PluralR(PluralRun{Forms: map[PluralForm][]Run{PluralOther: {{}}}})} {
		_, err := bad.MarshalJSON()
		require.Error(t, err)
		_, ok := appendRunJSON(nil, bad)
		assert.False(t, ok, "a run Run.MarshalJSON refuses is refused here too")
	}
}

// Random strings over the characters escaping treats specially agree too.
func TestAppendRunJSON_RandomStrings(t *testing.T) {
	alphabet := []string{"a", "Z", " ", "\"", "\\", "/", "<", ">", "&", "\x00", "\x08", "\x0c", "\n", "\r", "\t", "\x1b", "\x7f",
		"é", " ", " ", "�", "🍓", "\xff", "\xc3", "\xe2\x80", "\xed\xb0\x80"}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		var b strings.Builder
		for range rng.IntN(12) {
			b.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		r := PcOpenR(PcOpenRun{ID: b.String(), Type: "t", Data: b.String(), Attrs: map[string]string{b.String(): b.String()}})
		want, err := r.MarshalJSON()
		require.NoError(t, err)
		got, _ := appendRunJSON(nil, r)
		require.Equal(t, string(want), string(got), "%q", b.String())
	}
}
