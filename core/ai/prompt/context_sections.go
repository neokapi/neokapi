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
	// it costs a handful of tokens, and it is what every localization vendor
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
}

// Empty reports whether there is nothing to say about this block.
func (c Context) Empty() bool {
	return strings.TrimSpace(c.Key) == "" && len(c.Before) == 0 && len(c.After) == 0
}

// Digest fingerprints the *neighbourhood* — the part of the context that is not a
// function of the block itself.
//
// Key is deliberately excluded: it travels with the block, so a cache keyed by
// the block already accounts for it. The neighbours do not, and a block whose
// neighbours changed must be re-translated even though its own text did not.
func (c Context) Digest() string {
	if len(c.Before) == 0 && len(c.After) == 0 {
		return ""
	}
	var b strings.Builder
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
			Heading: "Nearby text, for context only — do not translate it and do not " +
				"return it:",
			Text: strings.TrimRight(b.String(), "\n"),
		})
	}

	return out
}

// collapse flattens a neighbour onto one line. A neighbour is a hint, not a
// document: newlines in it would blur the boundary between one hint and the next.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
