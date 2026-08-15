// Package clip shortens strings for diagnostics without splitting a rune.
package clip

// Runes shortens s to at most n runes, appending a horizontal ellipsis when it
// cuts, and returns s unchanged when it already fits.
//
// It counts runes rather than bytes, so the result is always valid UTF-8. A
// byte slice at an arbitrary offset lands mid-rune on any non-ASCII input and
// emits a replacement character into whatever message carries the result —
// which is why the cut is expressed in runes even though the callers are
// budgeting roughly by length.
//
// n below zero clips to nothing. Runes counts code points, not grapheme
// clusters, so a cut can still land inside a combining sequence or an emoji
// ZWJ join; the output stays well-formed UTF-8 either way.
//
// Callers sizing a string to a terminal column want cell-aware truncation
// instead — a wide rune occupies two cells, which no rune count captures.
func Runes(s string, n int) string {
	if n < 0 {
		n = 0
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
