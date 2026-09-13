package main

import (
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/check"
)

// A Negative is a reviewed target altered so that it no longer uses a required
// rendering. Every mode must fail it, and a mode that passes one has passed
// content that breaks the rule.
type Negative struct {
	// Kind is "deleted" (the words that used a rendering are gone) or
	// "clipped" (each is replaced by a word that opens like the rendering and
	// ends differently, the shape of "Kaiplan" for "kaiplass").
	Kind   string
	Target string
}

// wordPattern is a word as the negatives see it: letters and digits, joined
// by inner hyphens, so "inline-kode" is one word.
var wordPattern = regexp.MustCompile(`[\p{L}\p{N}]+(?:-[\p{L}\p{N}]+)*`)

// negativesFor builds the negatives of one target that uses rendering.
func negativesFor(target, rendering string) []Negative {
	return negativesForAll(target, []string{rendering})
}

// negativesForAll builds the negatives of a target that uses a rule through
// any of surfaces (its renderings and their forms): the target with every word
// that contains a surface deleted, and the target with every such word replaced
// by a clip of the first surface the target uses. A negative keeps no word that
// holds any surface, so it is a real negative. It returns none when no word of
// the target contains a surface.
func negativesForAll(target string, surfaces []string) []Negative {
	var declared, needles []string
	for _, s := range surfaces {
		if s = strings.TrimSpace(s); s != "" {
			declared = append(declared, s)
			needles = append(needles, strings.ToLower(s))
		}
	}
	if len(needles) == 0 {
		return nil
	}

	// first is the surface the target uses first, in its declared casing, so
	// a clip of "Kaiplasser" stays capitalised.
	first := ""
	uses := func(word string) bool {
		lower := strings.ToLower(word)
		for i, n := range needles {
			if strings.Contains(lower, n) {
				if first == "" {
					first = declared[i]
				}
				return true
			}
		}
		return false
	}
	replace := func(with func() string) (string, int) {
		hits := 0
		out := wordPattern.ReplaceAllStringFunc(target, func(w string) string {
			if !uses(w) {
				return w
			}
			hits++
			return with()
		})
		return out, hits
	}

	deleted, hits := replace(func() string { return "" })
	if hits == 0 {
		return nil
	}
	clip := check.ClipWord(first)
	clipped, _ := replace(func() string { return clip })
	return []Negative{
		{Kind: "deleted", Target: deleted},
		{Kind: "clipped", Target: clipped},
	}
}
