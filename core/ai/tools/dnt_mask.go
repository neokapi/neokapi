package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// dntSentinelRe matches a DNT sentinel case-insensitively, so restoration
// survives a model that altered the token's case (e.g. lowercased it) even
// though the payload itself is protected. The capture group is the term index.
var dntSentinelRe = regexp.MustCompile(`(?i)\[\[DNT_(\d+)\]\]`)

// This file enforces do-not-translate (DNT) terms in the AI translate path
// (strategy 2026-07-dogfood doc 07 / roadmap epic 019, item 4). The glossary is
// advisory — it asks the model to prefer a rendering; DNT is enforced — a
// protected span is MASKED with a sentinel before the model sees it and RESTORED
// to the exact original after, so the term survives verbatim regardless of what
// the model does with it. This is the mask/lock-then-restore mechanism the epic
// prefers over post-hoc "reject/repair", which cannot safely reconstruct a
// dropped span.

// dntSentinel formats the placeholder that stands in for the i-th protected
// span while the text is translated. The shape (uppercase ASCII, no spaces,
// double-bracketed) is chosen to survive a generation intact: models
// overwhelmingly pass an opaque token like this through untouched, and dntRestore
// matches it case-insensitively so even a model that only changes the token's
// case still restores the exact term. A model that structurally MANGLES the
// token (splits it, inserts spaces) is the residual risk: the sentinel then
// neither restores nor is removed — the same observable outcome as the model
// dropping a DNT term, which the ungated dnt-check tool flags. Masking cannot
// invent text, so it never makes that case worse.
func dntSentinel(i int) string {
	return fmt.Sprintf("[[DNT_%d]]", i)
}

// sanitizeDNT trims, de-duplicates, and drops empty terms, then sorts the
// survivors longest-first so a term that is a substring of another is masked
// after the longer one (e.g. "Kapi Desktop" before "Kapi"). The order is
// deterministic, so the same inputs always mask identically.
func sanitizeDNT(terms []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j]) // longest first
		}
		return out[i] < out[j]
	})
	return out
}

// dntMask replaces every occurrence of each protected term in text with a
// per-term sentinel and returns the masked text plus the index→term restoration
// map. A term absent from the text contributes no sentinel. Terms are applied
// longest-first (sanitizeDNT ordering) so overlapping terms mask without
// corrupting each other.
func dntMask(text string, terms []string) (string, map[int]string) {
	if len(terms) == 0 || text == "" {
		return text, nil
	}
	restore := map[int]string{}
	for i, term := range terms {
		if term == "" || !strings.Contains(text, term) {
			continue
		}
		text = strings.ReplaceAll(text, term, dntSentinel(i))
		restore[i] = term
	}
	if len(restore) == 0 {
		return text, nil
	}
	return text, restore
}

// dntRestore substitutes each sentinel in the translated text back to its exact
// original term. Because the original was replaced BEFORE translation, the only
// thing to do on the way out is swap the sentinels back — a sentinel the model
// dropped simply does not appear (the same "DNT term missing from target" case
// the dnt-check tool already flags; restoration never invents text). Matching is
// case-insensitive on the sentinel token so a model that altered the token's
// case still restores the exact original term.
func dntRestore(text string, restore map[int]string) string {
	if len(restore) == 0 || text == "" {
		return text
	}
	return dntSentinelRe.ReplaceAllStringFunc(text, func(match string) string {
		m := dntSentinelRe.FindStringSubmatch(match)
		if len(m) != 2 {
			return match
		}
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			return match
		}
		if term, ok := restore[idx]; ok {
			return term
		}
		return match // unknown index — leave as-is rather than invent text
	})
}
