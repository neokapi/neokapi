package check

import "strings"

// clipEndings are tried in order after the kept opening of a word.
var clipEndings = []string{"an", "or", "ix"}

// ClipWord is a word that opens with most of word and ends differently: all but
// its last three characters, at least three, followed by the first ending that
// does not spell word again. "kaiplass" clips to "kaiplan". The result never
// contains word, so a checker that accepts it is matching a rendering by its
// opening characters. A blank word clips to "".
func ClipWord(word string) string {
	r := []rune(strings.TrimSpace(word))
	if len(r) == 0 {
		return ""
	}
	keep := max(3, len(r)-3)
	if keep >= len(r) {
		keep = len(r) - 1
	}
	opening := string(r[:keep])
	needle := strings.ToLower(string(r))
	for _, ending := range clipEndings {
		if w := opening + ending; !strings.Contains(strings.ToLower(w), needle) {
			return w
		}
	}
	return opening + "q"
}
