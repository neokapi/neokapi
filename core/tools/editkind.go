package tools

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// EditKind says how a source has changed against the source an answer was
// approved for. It is what a match score was always a proxy for, and a bad one.
//
// A percentage measures how many characters moved. What decides whether an
// approved translation still stands is whether the WORDS moved, and the two
// come apart exactly where it matters most:
//
//	"Click the button below when you're ready …"  →  the same with "not"   95%
//	"Get started"                                 →  "Get started."        91%
//
// The first inverts an instruction and the second adds a full stop.
//
// Ranked by score, the inversion looks like the better match, and a fill floor
// anywhere between them fills the dangerous one and refuses the harmless one.
// Length is why: a period is 8% of a button label and under 2% of a sentence,
// so the same edit scores differently depending on how much text surrounds it.
//
// A kind does not have that problem. Adding "not" is substantive in a two-word
// string and in a paragraph; a full stop is cosmetic in both.
type EditKind string

const (
	// EditNone: the sources are identical.
	EditNone EditKind = "none"
	// EditCosmetic: the same words in the same order, differing only in
	// punctuation, case, spacing or the shape of a quote or dash. An approved
	// translation still stands; at most it needs the same cosmetic touch.
	EditCosmetic EditKind = "cosmetic"
	// EditSubstantive: a word was added, removed, or changed. An approved
	// translation may no longer be a translation of this, and nothing about the
	// size of the change makes that safer.
	EditSubstantive EditKind = "substantive"
)

// SafeToFill reports whether an answer approved for the prior source can be
// written as this source's target.
//
// Only when the words have not moved. This is the whole point of classifying
// rather than scoring: it does not soften with length.
func (k EditKind) SafeToFill() bool { return k == EditNone || k == EditCosmetic }

// ClassifyEdit compares the source an answer was approved for against the
// source in hand.
func ClassifyEdit(prior, current string) EditKind {
	if prior == current {
		return EditNone
	}
	if wordsEqual(prior, current) {
		return EditCosmetic
	}
	return EditSubstantive
}

// wordsEqual reports whether two texts carry the same words in the same order.
//
// A word is a maximal run of letters and digits, with hyphens and apostrophes
// dropped INSIDE it so "set-up" and "setup" are one word either way and
// "don't" and "don’t" are the same word. Everything between words is a
// separator and is not compared: spaces, punctuation, quote marks of any shape.
//
// The word boundary is what keeps this honest. Stripping punctuation without it
// would make "not able" and "notable" the same text, which is a meaning change
// wearing a cosmetic disguise, the exact failure this function exists to catch.
func wordsEqual(a, b string) bool {
	wa, wb := words(a), words(b)
	if len(wa) != len(wb) {
		return false
	}
	for i := range wa {
		if wa[i] != wb[i] {
			return false
		}
	}
	return true
}

// words splits text into comparable words: NFC-normalised, case-folded, with
// intra-word hyphens and apostrophes removed.
func words(s string) []string {
	var out []string
	for _, t := range tokenize(s) {
		if t.word {
			out = append(out, t.key)
		}
	}
	return out
}

// token is one piece of a source: the text as written, and the form the
// comparison uses.
//
// Keeping both is what lets the verdict and the diff beside it come from one
// splitter. key is the comparable form: the normalised word, or sepKey for any
// run of separators, so punctuation compares equal to punctuation and a full
// stop that replaced a comma aligns instead of reading as a word going missing.
type token struct {
	raw  string
	key  string
	word bool
}

// sepKey is the key every separator run shares.
const sepKey = "\x00sep"

// tokenize splits text into alternating word and separator tokens.
//
// An intra-word rune continues whichever kind of token is open rather than
// switching: inside a word it joins ("set-up" is one word, keyed like "setup"),
// and outside one it is punctuation like any other.
func tokenize(s string) []token {
	s = norm.NFC.String(s)
	var out []token
	var raw, key strings.Builder
	inWord := false
	flush := func() {
		if raw.Len() == 0 {
			return
		}
		t := token{raw: raw.String(), word: inWord}
		if inWord {
			t.key = key.String()
		} else {
			t.key = sepKey
		}
		out = append(out, t)
		raw.Reset()
		key.Reset()
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				flush()
				inWord = true
			}
			raw.WriteRune(r)
			key.WriteRune(unicode.ToLower(r))
		case isIntraWord(r):
			// Written down, never compared: a hyphen appearing or disappearing
			// inside a word is a house-style change, not a different word.
			raw.WriteRune(r)
		default:
			if inWord {
				flush()
				inWord = false
			}
			raw.WriteRune(r)
		}
	}
	flush()
	return out
}

// isIntraWord reports whether a rune joins rather than separates: hyphens of
// every width, and apostrophes of either shape.
func isIntraWord(r rune) bool {
	switch r {
	case '-', '‐', '‑', '‒', '–', '—', '−',
		'\'', '’', 'ʼ':
		return true
	}
	return false
}
