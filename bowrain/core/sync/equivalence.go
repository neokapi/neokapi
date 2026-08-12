package sync

import (
	"sort"
	"strings"
	"unicode"
)

// Channel-slug equivalence.
//
// Two projects in one workspace can spell the same channel differently — `web`
// and `website`, `marketing-site` and `marketingsite` — and neither can see the
// other. The workspace can, and the useful thing it can do about it is say so.
//
// What it must not do is resolve. A project resolves its coordinates from its
// own recipe, offline, and the same recipe has to mean the same thing whether or
// not the project has ever connected. So the workspace proposes equivalence and
// leaves both slugs exactly as their recipes wrote them.
//
// Equivalence is only ever proposed WITHIN one profile: a channel is a surface
// of a product, so two products' identically named channels are two channels,
// not one. The profile is also the evidence that makes a loose slug comparison
// defensible — inside one product, `web` and `website` are far more likely to be
// one surface than two.

// Evidence values naming why two slugs look like one channel.
const (
	// EvidenceSameNormalized: the slugs differ only in case or separators.
	EvidenceSameNormalized = "the slugs differ only in case or separators"
	// EvidencePrefix: one slug is a prefix of the other inside one profile.
	EvidencePrefix = "one slug is a prefix of the other within the same product"
)

// ChannelAlias is one proposed equivalence: the slug that arrived, the slug the
// workspace already held, and why they look alike.
type ChannelAlias struct {
	Proposed string
	Existing string
	Evidence string
}

// minPrefixLength is how much of a slug must match before a prefix counts as
// evidence. Two characters would pair every short slug in the workspace with
// every other; three is the shortest slug anyone writes on purpose ("web",
// "doc", "app"), so it is the shortest length at which a prefix says something.
const minPrefixLength = 3

// ProposeChannelAliases compares the channels one project declares against the
// channels the workspace already holds, within one profile, and returns the
// pairs that look like one channel.
//
// A slug the workspace already holds byte for byte is not fragmentation and
// raises nothing. Comparison is against the workspace's OTHER slugs only, so a
// project re-pushing its own vocabulary proposes nothing.
func ProposeChannelAliases(proposed, existing []string) []ChannelAlias {
	if len(proposed) == 0 || len(existing) == 0 {
		return nil
	}
	held := make(map[string]bool, len(existing))
	for _, e := range existing {
		held[e] = true
	}

	seen := map[[2]string]bool{}
	var out []ChannelAlias
	for _, p := range proposed {
		if p == "" || held[p] {
			continue
		}
		for _, e := range existing {
			if e == "" || e == p {
				continue
			}
			evidence, ok := looksLikeOneChannel(p, e)
			if !ok {
				continue
			}
			key := [2]string{p, e}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ChannelAlias{Proposed: p, Existing: e, Evidence: evidence})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Proposed != out[j].Proposed {
			return out[i].Proposed < out[j].Proposed
		}
		return out[i].Existing < out[j].Existing
	})
	return out
}

// looksLikeOneChannel reports whether two slugs are worth proposing as one, and
// the evidence for it.
func looksLikeOneChannel(a, b string) (string, bool) {
	ka, kb := ChannelKey(a), ChannelKey(b)
	if ka == "" || kb == "" {
		return "", false
	}
	if ka == kb {
		return EvidenceSameNormalized, true
	}
	shorter, longer := ka, kb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) >= minPrefixLength && strings.HasPrefix(longer, shorter) {
		return EvidencePrefix, true
	}
	return "", false
}

// ChannelKey is the comparison form of a channel slug: lowercase, with
// everything that is not a letter or a digit removed. It exists only to compare
// — nothing is ever stored or resolved under it, because a normalized slug is
// not a slug anyone wrote.
func ChannelKey(slug string) string {
	var b strings.Builder
	b.Grow(len(slug))
	for _, r := range strings.ToLower(slug) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
