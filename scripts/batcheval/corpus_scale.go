package main

import (
	"fmt"
	"strings"
)

// The 30-block corpus was too small to answer the question it was built for.
//
// A sweep to N=32 over 30 blocks does not test a batch of 32. It tests "the whole
// document in one call" — and N=16 tests "two calls". The curve came out flat at
// 100% because the experiment saturated, not because the models are indifferent to
// batch size. The measured ceiling was the corpus's, and reporting it as the
// models' was the mistake.
//
// So the corpus scales. CorpusN(n) keeps the authored 30 exactly as they were (the
// digest is unchanged, so the published runs stay comparable) and generates beyond
// them, holding the stressor mix roughly constant: a third short UI strings, a
// fifth placeholders, a fifth prose, the rest ambiguous keys and inline markup.
//
// Generation is deterministic — index-driven, no randomness — because a corpus that
// differs between runs cannot be a baseline. Every generated string is distinct:
// duplicate source text would let a model answer one segment and copy it into the
// next, which is exactly the failure the eval is trying to detect, so it must not
// be manufactured by the corpus itself.

// CorpusN returns a corpus of n blocks. n <= 30 returns the authored corpus,
// unchanged and with its original digest.
func CorpusN(n int) TestCorpus {
	base := Corpus()
	if n <= len(base.Cases) {
		return base
	}
	c := TestCorpus{Cases: append([]Case(nil), base.Cases...)}
	for i := len(base.Cases); i < n; i++ {
		c.Cases = append(c.Cases, generate(i))
	}
	return c
}

// generate builds block i, cycling the stressor kinds in the proportions the
// authored corpus uses.
func generate(i int) Case {
	switch i % 10 {
	case 0, 3, 6: // ~30% short UI strings
		return uiCase(i)
	case 1, 8: // ~20% placeholders
		return placeholderCase(i)
	case 2, 9: // ~20% prose (long items — where batching is documented to hurt most)
		return proseCase(i)
	case 4: // ~10% inline markup
		return tagCase(i)
	default: // ~20% ambiguous-without-key
		return ambiguousCase(i)
	}
}

var (
	uiVerbs = []string{"Open", "Close", "Rename", "Duplicate", "Archive", "Restore", "Publish", "Unpublish", "Export", "Import", "Share", "Lock"}
	uiNouns = []string{"project", "invoice", "folder", "report", "workspace", "branch", "release", "vocabulary", "segment", "comment", "draft", "snapshot"}

	sections = []string{"billing", "settings", "editor", "review", "catalog", "members", "search", "audit"}

	// The empty qualifier comes first so the commonest phrasing reads naturally
	// ("billing invoice"), with the rest widening the space enough to keep every
	// generated source string distinct.
	qualifiers = []string{
		"", "archived ", "shared ", "draft ", "pending ", "locked ", "public ", "private ",
		"stale ", "linked ", "merged ", "pinned ", "expired ", "queued ", "flagged ", "hidden ",
	}

	phTemplates = []string{
		"You have %[2]d unread %[1]ss",
		"Uploaded {0} of {1} %[1]ss",
		"Welcome back, {{name}}, you have {count} %[1]ss waiting",
		"Could not sync %%s: the %[1]s was modified by {user}",
		"{used} of {total} %[1]ss used this month",
		"%[1]s {id} was updated on {date} by {{author}}",
	}

	tagTemplates = []string{
		`Your %[1]s is ready. <ph id="1"/>Review it<ph id="2"/> before it ships.`,
		`This will <ph id="1"/>permanently<ph id="2"/> delete the %[1]s.`,
		`Questions about this %[1]s? <ph id="1"/>Contact support<ph id="2"/> any time.`,
		`We could not open the %[1]s. <ph id="1"/>Try again<ph id="2"/> or <ph id="3"/>report a problem<ph id="4"/>.`,
	}

	// Prose is assembled from independent clause banks so every paragraph is
	// distinct in content, not merely in a numeral. Long items matter: the
	// literature is emphatic that batch degradation is worst when each item is long.
	proseOpeners = []string{
		"A %[1]s is written once and read many times, so the cost of getting it wrong is paid by everyone downstream.",
		"The %[1]s is not a document but a record of a decision, and it is only useful while that decision still holds.",
		"Every %[1]s in this system is derived from something else, which means it can always be regenerated and should never be hand-edited.",
		"Reviewing a %[1]s is cheap; discovering a broken one in production is not, which is the whole argument for doing it early.",
		"When a %[1]s changes, everything that quoted it becomes a question rather than a fact.",
	}
	proseMiddles = []string{
		"The engine reads it, hands each translatable unit to the tools you configured, and writes the result back in the original format, touching nothing else in the file.",
		"Caching is keyed on the content and on the configuration that produced it, so changing the model or the terms re-translates exactly the strings that depended on them and no others.",
		"Checks are deterministic and run on everything; review is a judgement call, made by a model, on the few blocks the checks could not settle.",
		"Terminology is enforced at generation time rather than corrected afterwards, because it is cheaper to ask for the right word than to detect the wrong one.",
		"A plugin runs as a separate process and speaks over gRPC, so a crash in it cannot corrupt the engine that launched it.",
	}
	proseClosers = []string{
		"That is the contract, and it is the reason the pipeline can be trusted with files nobody wants to reconstruct by hand.",
		"Anything that cannot answer that question does not ship, which is a blunt rule but an honest one.",
		"The alternative is a system that is usually right, and nobody can tell you when.",
		"It is a small constraint that buys a large amount of confidence.",
		"Everything else in the design follows from it.",
	}
)

func pick[T any](xs []T, i int) T { return xs[i%len(xs)] }

// phrase builds a distinct noun phrase for block i by mixed-radix indexing over
// three banks — "billing invoice", "catalog release", "audit snapshot".
//
// It exists because duplicate source text must not occur. A repeated string would
// let a model translate one segment and copy it into another, manufacturing the
// exact failure the eval is meant to detect. Twelve nouns crossed with eight
// domains crossed with twelve verbs gives over a thousand distinct blocks, well
// past any corpus we sweep.
func phrase(i int) string {
	// Mixed radix, so the phrase is injective in i: noun cycles fastest, then the
	// domain, then a qualifier. 12 x 8 x 16 = 1,536 distinct phrases, well past any
	// corpus we sweep. A phrase that repeated would repeat a source string.
	return pick(qualifiers, i/(len(uiNouns)*len(sections))) +
		pick(sections, i/len(uiNouns)) + " " + pick(uiNouns, i)
}

// key turns a phrase into a dotted key, the shape real projects use.
func key(ph string) string { return strings.ReplaceAll(ph, " ", ".") }

func uiCase(i int) Case {
	verb, ph := pick(uiVerbs, i), phrase(i)
	return Case{
		Key:    fmt.Sprintf("%s.%s", key(ph), verb),
		Source: fmt.Sprintf("%s %s", verb, ph),
		Kind:   "ui",
	}
}

// ambiguousCase reuses a bare verb under a distinct key — the case where the key is
// the only thing separating a noun reading from a verb reading, and where positional
// batch mapping used to corrupt silently. These duplicates are the point.
func ambiguousCase(i int) Case {
	verb := pick(uiVerbs, i)
	return Case{
		Key:    fmt.Sprintf("%s.action.%s.%d", pick(sections, i/2+3), verb, i),
		Source: verb,
		Kind:   "ui-ambiguous",
	}
}

func placeholderCase(i int) Case {
	ph := phrase(i)
	return Case{
		Key:    key(ph) + ".status",
		Source: fmt.Sprintf(pick(phTemplates, i), ph, i%9+2),
		Kind:   "placeholder",
	}
}

func tagCase(i int) Case {
	ph := phrase(i)
	return Case{
		Key:    key(ph) + ".notice",
		Source: fmt.Sprintf(pick(tagTemplates, i), ph),
		Kind:   "inline-tag",
	}
}

func proseCase(i int) Case {
	ph := phrase(i)
	return Case{
		Key: "docs." + key(ph),
		Source: fmt.Sprintf(pick(proseOpeners, i), ph) + " " +
			pick(proseMiddles, i/2+i%3) + " " +
			pick(proseClosers, i/3+i%5),
		Kind: "prose",
	}
}
