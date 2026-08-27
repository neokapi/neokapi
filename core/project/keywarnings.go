package project

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// An unknown recipe key is preserved, not rejected, and that is deliberate:
// `Defaults` and `Collection` end in an inline `Extras` map so a platform layer
// can carry its own keys through a framework-only round trip.
//
// The cost is that a near miss is silent. `source:` where the field is
// `source_language:` lands in Extras, is faithfully preserved, and the project
// loads as a valid monolingual one. A whole recipe of near misses reports
// nothing except the single thing the validator asks about explicitly, which is
// how a set of hand-written eval fixtures spent a metered sweep handing agents
// projects kapi would not read. See #2223.
//
// Rejecting every unknown key would break the mechanism. What separates a typo
// from an extension is that a typo is nearly a field that exists: `source` is
// one edit from `source_language`'s prefix, `bowrain` is not one edit from
// anything. So a key close to a known sibling is reported, and everything else
// is left alone.

// KeyWarning is one unknown key that looks like a known one.
type KeyWarning struct {
	// Path is where the key sits, e.g. "defaults.source".
	Path string
	// DidYouMean is the known field it resembles.
	DidYouMean string
}

func (w KeyWarning) String() string {
	return fmt.Sprintf("%s is not a known field. Did you mean %s?", w.Path, w.DidYouMean)
}

// KeyWarnings reports unknown keys that resemble a known field of the same
// struct, over the whole recipe.
//
// Sorted by path, so a recipe with several produces the same list every time
// and a caller can print them without ordering surprises.
func (p *KapiProject) KeyWarnings() []string {
	if p == nil {
		return nil
	}
	var out []KeyWarning
	out = append(out, nearMisses("", p.Extras, knownFields(reflect.TypeFor[KapiProject]()))...)
	out = append(out, nearMisses("defaults", p.Defaults.Extras, knownFields(reflect.TypeFor[Defaults]()))...)
	for _, c := range p.Collections {
		path := "collections[" + c.Name + "]"
		if c.Name == "" {
			path = "collections[]"
		}
		out = append(out, nearMisses(path, c.Extras, knownFields(reflect.TypeFor[Collection]()))...)
		for _, item := range c.Content {
			out = append(out, nearMisses(path+".content", item.Extras, knownFields(reflect.TypeFor[ContentItem]()))...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	msgs := make([]string, 0, len(out))
	for _, w := range out {
		msgs = append(msgs, w.String())
	}
	return msgs
}

// knownFields is the yaml key of every field a struct declares, taken by
// reflection rather than listed, so a field added to the schema is known here
// the moment it exists.
func knownFields(t reflect.Type) []string {
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, name)
	}
	return out
}

// nearMissDistance is how far an unknown key may be from a known field and
// still be reported as a probable typo.
//
// Two covers a transposition, a doubled letter, and a wrong one. It does not
// reach from `source` to `source_language`, which is why prefix and
// singular/plural are checked separately below: those are the shapes a person
// actually guesses.
const nearMissDistance = 2

// nearMisses pairs each unknown key with the known field it resembles.
func nearMisses(prefix string, extras map[string]yaml.Node, known []string) []KeyWarning {
	var out []KeyWarning
	for key := range extras {
		best, ok := closestField(key, known)
		if !ok {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		out = append(out, KeyWarning{Path: path, DidYouMean: best})
	}
	return out
}

// closestField returns the known field an unknown key most likely meant.
//
// Three shapes, in the order a person produces them: the key is a known field
// with a small edit distance, the key is the start of a known field
// (`source` for `source_language`), or the key differs only in plurality
// (`targets` for `target_languages` reaches this through the prefix rule on
// `target`). A key matching none of them is an extension and is not reported.
func closestField(key string, known []string) (string, bool) {
	k := strings.ToLower(key)
	best, bestScore := "", -1
	for _, f := range known {
		if f == key {
			return "", false // it IS a known field; nothing to warn about
		}
		score := -1
		switch {
		case strings.HasPrefix(f, k+"_"):
			// `source` for `source_language`. The strongest signal: a person
			// wrote the head of a compound field.
			score = 3
		case strings.HasPrefix(f, strings.TrimSuffix(k, "s")+"_"):
			// `targets` for `target_languages`.
			score = 2
		case editDistance(k, f) <= nearMissDistance:
			score = 1
		}
		if score > bestScore {
			best, bestScore = f, score
		}
	}
	return best, bestScore > 0
}

// editDistance is Levenshtein, bounded by the shorter string's length.
func editDistance(a, b string) int {
	if a == b {
		return 0
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// OnKeyWarnings is called after a recipe loads with any near-miss keys it
// carries. Nil by default, so the framework prints nothing on its own.
//
// A hook rather than a print: core/project is library code and a package that
// writes to stderr on its own initiative is a package a caller cannot embed.
// A hook rather than a LoadOptions field: a recipe is loaded from a dozen
// places for a dozen reasons, and threading a writer through all of them to
// report a typo would put the plumbing everywhere the problem is not.
//
// The host sets it once at startup. Set it before loading anything, from one
// goroutine; it is read, not synchronised.
var OnKeyWarnings func(recipePath string, warnings []string)

// reportKeyWarnings hands a freshly loaded recipe's near-miss keys to the hook.
func reportKeyWarnings(path string, p *KapiProject) {
	if OnKeyWarnings == nil {
		return
	}
	if w := p.KeyWarnings(); len(w) > 0 {
		OnKeyWarnings(path, w)
	}
}
