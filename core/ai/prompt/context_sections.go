package prompt

import (
	"fmt"
	"strings"
)

// Context is what kapi tells the model about a block *besides the block*.
//
// The distinction this type exists to enforce: there are two very different
// things you can put in a translation prompt, and conflating them is how you get
// a worse translation for more money.
//
//   - Segments the model must *emit*. Every one is a task competing for
//     attention and a distractor for every other. Adding them degrades quality —
//     measurably, and the failure mode at size is dropped and renumbered
//     segments, not merely clumsier wording.
//   - Reference material the model must *not translate*. This is close to free
//     upside: it disambiguates without adding work.
//
// kapi used to send neither. The batch was co-workers with no reference at all,
// which is the worst of both: the model saw 100 sibling strings it had to
// translate and nothing at all that would tell it what any of them meant.
//
// A `Save` with no context is a coin flip between a verb and a noun — and in
// German, between *Speichern* and *Speicherung*. `settings.save` settles it.
type Context struct {
	// Key is the block's key or path — `app.settings.title`, not `Save`.
	//
	// The cheapest disambiguation signal there is: it is already in the document,
	// it costs a handful of tokens, and it is what every translation vendor
	// sends. It is also *stable*: a block's key is a function of the block, so
	// sending it cannot make a cached translation wrong.
	Key string

	// Before and After are the neighbouring source blocks, as reference. They are
	// not translated and must not be echoed.
	//
	// Unlike Key, these are a function of the block's *surroundings*, so a block's
	// translation now depends on text that is not the block. Callers must fold
	// Digest() into their cache key or they will serve a translation produced
	// under a neighbourhood that no longer exists.
	Before []string
	After  []string

	// Prior is what this same block said last time, and what was approved for
	// it then. It is reference, never an answer: the model reads it, does not
	// echo it, and is free to depart from it where the source has moved.
	//
	// It replaces what a fuzzy match was reaching for. A similarity score
	// answered "is this the same thing, changed a bit?" by measuring
	// characters; a version chain answers it outright, and can hand over all
	// three terms — what the source said, what was approved for it, what the
	// source says now — so the model can do the delta reasoning post-editing
	// used to force onto a person.
	//
	// A caller must fill this only when the governing context has not moved
	// since that answer was approved. A prior target is the most anchoring
	// thing that can go in a prompt, and one approved under superseded rules
	// pulls the model back toward wording those rules now reject — while the
	// result is stamped with today's fingerprint and looks fresh.
	// memory.Version.GovernedBy is the gate.
	Prior *PriorVersion
}

// PriorVersion is one block's previous source and the target approved for it.
// The JSON tags are load-bearing rather than decorative: a prior version rides
// inside the batch payload, so these field names are text a model reads. Lower
// case because everything else in that payload is.
type PriorVersion struct {
	// Source is what the block said when Target was approved. Without it the
	// target is an anchor with no explanation; with it the pair is a diff the
	// model can reason about.
	Source string `json:"was"`
	// Target is the answer approved for that source.
	Target string `json:"approved"`
}

// empty reports whether a prior version says nothing useful. Either half alone
// is nothing: a source with no target teaches the model wording it must not
// reuse, and a target with no source is the anchor without the explanation.
func (p *PriorVersion) empty() bool {
	return p == nil || strings.TrimSpace(p.Source) == "" || strings.TrimSpace(p.Target) == ""
}

// Empty reports whether there is nothing to say about this block.
func (c Context) Empty() bool {
	return strings.TrimSpace(c.Key) == "" && len(c.Before) == 0 && len(c.After) == 0 &&
		c.Prior.empty()
}

// Digest fingerprints the *neighbourhood* — the part of the context that is not a
// function of the block itself.
//
// Key is deliberately excluded: it travels with the block, so a cache keyed by
// the block already accounts for it. The neighbours do not, and a block whose
// neighbours changed must be re-translated even though its own text did not.
func (c Context) Digest() string {
	if len(c.Before) == 0 && len(c.After) == 0 && c.Prior.empty() {
		return ""
	}
	var b strings.Builder
	// The prior version belongs in the digest for the same reason the
	// neighbours do: it is not a function of the block's own text, so a
	// translation cached with one prior version must not be served after the
	// chain has moved. Unlike a location, it changes what the model saw.
	if !c.Prior.empty() {
		b.WriteString(c.Prior.Source)
		b.WriteByte('\x00')
		b.WriteString(c.Prior.Target)
		b.WriteByte('\x00')
	}
	for _, s := range c.Before {
		b.WriteString(s)
		b.WriteByte('\x00')
	}
	b.WriteByte('|')
	for _, s := range c.After {
		b.WriteString(s)
		b.WriteByte('\x00')
	}
	return hashString(b.String())
}

// sections renders the context as attributed prompt sections, so --explain-prompts
// and the prompt reference can show exactly what was said about a block and where
// it came from.
func (c Context) sections() []Section {
	var out []Section

	if key := strings.TrimSpace(c.Key); key != "" {
		out = append(out, Section{
			Kind:    KindContext,
			Origin:  "document (the block's key)",
			Heading: "This text appears at:",
			Text:    key,
		})
	}

	if len(c.Before) > 0 || len(c.After) > 0 {
		var b strings.Builder
		for _, s := range c.Before {
			fmt.Fprintf(&b, "- %s\n", collapse(s))
		}
		b.WriteString("- ⟵ the text to translate\n")
		for _, s := range c.After {
			fmt.Fprintf(&b, "- %s\n", collapse(s))
		}
		out = append(out, Section{
			Kind:   KindContext,
			Origin: fmt.Sprintf("document (%s nearby)", plural(len(c.Before)+len(c.After), "block")),
			Heading: "Nearby text, for context only. Do not translate it and do not " +
				"return it:",
			Text: strings.TrimRight(b.String(), "\n"),
		})
	}

	if !c.Prior.empty() {
		out = append(out, Section{
			Kind:   KindContext,
			Origin: "content memory (this block's previous approved answer)",
			Heading: "This block has been translated before. Keep the wording where the source " +
				"still says the same thing, and depart from it where the source has changed. " +
				"Do not return this:",
			Text: fmt.Sprintf("previous source: %s\nprevious translation: %s",
				collapse(c.Prior.Source), collapse(c.Prior.Target)),
		})
	}

	return out
}

// collapse flattens a neighbour onto one line. A neighbour is a hint, not a
// document: newlines in it would blur the boundary between one hint and the next.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
