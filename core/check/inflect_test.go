package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInflectionsCoverTheVerbsProfilesForbid.
//
// The words a voice profile actually forbids are mostly verbs, and prose uses
// them inflected. A rule about `utilize` that passes "the platform utilizes
// your data" is the failure this exists to stop (#2226).
func TestInflectionsCoverTheVerbsProfilesForbid(t *testing.T) {
	for _, tc := range []struct {
		term string
		want []string
	}{
		{"utilize", []string{"utilize", "utilizes", "utilized", "utilizing"}},
		{"leverage", []string{"leverage", "leverages", "leveraged", "leveraging"}},
		{"streamline", []string{"streamline", "streamlines", "streamlined", "streamlining"}},
		{"empower", []string{"empower", "empowers", "empowered", "empowering"}},
	} {
		got := Inflections(tc.term)
		for _, w := range tc.want {
			assert.Contains(t, got, w, "%s should inflect to %s", tc.term, w)
		}
	}
}

// TestInflectionsDoNotInventUnrelatedWords.
//
// A bare `-d` suffix would turn a two-letter term into a different word: `ad`
// plus `d` is "add", and a rule about advertising would start flagging
// arithmetic. An e-final term reaches its past tense through the e-drop.
func TestInflectionsDoNotInventUnrelatedWords(t *testing.T) {
	assert.NotContains(t, Inflections("advert"), "advertd")
	// The e-drop is how an e-final term reaches its past tense.
	assert.Contains(t, Inflections("streamline"), "streamlined")
}

// TestInflectionsLeaveNonWordTermsAlone: "C++" and "{count}" take their own edge
// rule in the matcher, and appending an "s" to them would find nothing.
func TestInflectionsLeaveNonWordTermsAlone(t *testing.T) {
	assert.Equal(t, []string{"C++"}, Inflections("C++"))
	assert.Equal(t, []string{"{count}"}, Inflections("{count}"))
	assert.Empty(t, Inflections("  "))
}

// TestFindTermInflectedMatchesTheWholeInflection.
//
// The hit has to cover the inflected word, not the stem inside it. Reporting
// bytes 0..7 of "utilizes" would underline "utilize" and leave the "s" outside
// the finding, and an editor offering a replacement would produce "uses".
func TestFindTermInflectedMatchesTheWholeInflection(t *testing.T) {
	text := "The platform utilizes your data."
	hits := FindTermInflected(text, "utilize")
	assert.Len(t, hits, 1)
	assert.Equal(t, "utilizes", text[hits[0][0]:hits[0][1]])
}

// TestFindTermInflectedReportsEachOccurrenceOnce: the forms overlap by
// construction, so a naive union would report "utilizes" twice.
func TestFindTermInflectedReportsEachOccurrenceOnce(t *testing.T) {
	text := "utilize, utilizes, utilized, utilizing"
	hits := FindTermInflected(text, "utilize")
	assert.Len(t, hits, 4)
	var got []string
	for _, h := range hits {
		got = append(got, text[h[0]:h[1]])
	}
	assert.Equal(t, []string{"utilize", "utilizes", "utilized", "utilizing"}, got)
}

// TestFindTermInflectedKeepsTheWordBoundary: inflection widens what counts as
// the end of the word, it does not remove the rule.
func TestFindTermInflectedKeepsTheWordBoundary(t *testing.T) {
	assert.Empty(t, FindTermInflected("a leverager bought in", "leverage"),
		"the boundary still holds: leveragers is not a form this generates")
	assert.Len(t, FindTermInflected("they leverage it", "leverage"), 1)
	assert.Len(t, FindTermInflected("they leveraged it", "leverage"), 1)
}

// TestShortTermsAreNotInflected.
//
// A short stem plus a suffix is usually a different word. `Go` is the case that
// proves it: core/profile's whole-word test asserts a rule about `Go` does not
// match inside "going", and generating `Go`+`ing` broke exactly that.
func TestShortTermsAreNotInflected(t *testing.T) {
	assert.Equal(t, []string{"Go"}, Inflections("Go"))
	assert.Empty(t, FindTermInflected("we are going home", "Go"))
	assert.Len(t, FindTermInflected("written in Go today", "Go"), 1)

	// `use` is under the floor too. Losing "uses" is the price of not matching
	// "going", and it is the right side of that trade: the words a profile
	// forbids are long ones.
	assert.Equal(t, []string{"use"}, Inflections("use"))
}

// TestFindTermIsStillExact: the un-inflected matcher is what `exact: true`
// reaches, and it must keep its old behaviour.
func TestFindTermIsStillExact(t *testing.T) {
	assert.Len(t, FindTerm("we utilize it", "utilize"), 1)
	assert.Empty(t, FindTerm("it utilizes data", "utilize"))
}
