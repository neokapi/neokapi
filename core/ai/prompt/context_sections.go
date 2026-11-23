package prompt

import (
	"fmt"
	"strings"
)

// Context supplies reference material that helps interpret a block without
// adding translation tasks. For example, the key settings.save identifies Save
// as an action. Prompt instructions distinguish this reference material from the
// segments the model must translate and return.
type Context struct {
	// Key is the block's key or path, such as app.settings.title.
	// It helps disambiguate short text and is already part of block identity.
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

	// Prior contains this block's previous source and approved target as
	// reference for revising the translation after a source edit.
	//
	// Populate it only when the approval's governing context still matches the
	// current context; memory.Version.GovernedBy performs that check. Otherwise
	// superseded wording could influence a result stamped with a current fingerprint.
	Prior *PriorVersion

	// Rejections are earlier drafts of this block a reviewer rejected, oldest
	// first, with the reasons in the words a person reads and the reviewer's
	// note. A host supplies them; they change what the model is asked, so
	// they are part of Digest.
	Rejections []Rejection
}

// Rejection is one earlier draft of a block a reviewer rejected. The JSON
// field names are part of the batch payload sent to the model.
type Rejection struct {
	// Text is the rejected draft, empty when its wording is no longer held.
	Text string `json:"rejected,omitempty"`
	// Reasons are the reviewer's reasons, as phrases.
	Reasons []string `json:"why,omitempty"`
	// Note is what the reviewer wrote.
	Note string `json:"note,omitempty"`
}

// PriorVersion pairs a block's previous source with its approved target.
// The JSON field names are part of the batch payload sent to the model.
type PriorVersion struct {
	// Source is what the block said when Target was approved. Without it the
	// target is an anchor with no explanation; with it the pair is a diff the
	// model can reason about.
	Source string `json:"was"`
	// Target is the answer approved for that source.
	Target string `json:"approved"`
}

// empty reports whether the prior version lacks either side of the approved
// pair. Both are required to interpret the previous translation.
func (p *PriorVersion) empty() bool {
	return p == nil || strings.TrimSpace(p.Source) == "" || strings.TrimSpace(p.Target) == ""
}

// Empty reports whether there is nothing to say about this block.
func (c Context) Empty() bool {
	return strings.TrimSpace(c.Key) == "" && len(c.Before) == 0 && len(c.After) == 0 &&
		c.Prior.empty() && len(c.Rejections) == 0
}

// Digest fingerprints the *neighbourhood*, the part of the context that is not a
// function of the block itself.
//
// Key is deliberately excluded: it travels with the block, so a cache keyed by
// the block already accounts for it. The neighbours do not, and a block whose
// neighbours changed must be re-translated even though its own text did not.
func (c Context) Digest() string {
	if len(c.Before) == 0 && len(c.After) == 0 && c.Prior.empty() && len(c.Rejections) == 0 {
		return ""
	}
	var b strings.Builder
	// The rejections belong in the digest for the reason the prior version
	// does: a draft made with one reviewer's note must not be served after the
	// note changed.
	for _, r := range c.Rejections {
		b.WriteString("rejected\x00")
		b.WriteString(r.Text)
		b.WriteByte('\x00')
		b.WriteString(strings.Join(r.Reasons, ","))
		b.WriteByte('\x00')
		b.WriteString(r.Note)
		b.WriteByte('\x00')
	}
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
			Kind:   KindContext,
			Origin: "document (the block's key)",
			// The key carries the same guard the neighbourhood does. A bare
			// "This text appears at:" left a key that reads as a subject
			// competing with the text: gemini-3.5-flash rendered the key for
			// `onboarding.title` = "Capture every idea" in all four target
			// languages ("Onboarding", "オンボーディング", "Paso a paso de
			// bienvenida", "Bienvenue !"). It got the other nine units of
			// the same file right. Naming it a location rather than content is what
			// separates the two.
			Heading: "Where the text sits in the document, for context only. It is a " +
				"location, not content: do not translate it and do not return it:",
			Text: key,
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

	if len(c.Rejections) > 0 {
		out = append(out, Section{
			Kind:   KindContext,
			Origin: fmt.Sprintf("review (%s of this block)", plural(len(c.Rejections), "rejected draft")),
			Heading: "A reviewer rejected earlier drafts of this text. Write a new translation that " +
				"answers the reasons and the note. Do not return any of these drafts:",
			Text: RenderRejections(c.Rejections),
		})
	}

	return out
}

// RenderRejections renders rejected drafts as the prompt states them, oldest
// first, one block of lines each.
func RenderRejections(rs []Rejection) string {
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			b.WriteByte('\n')
		}
		if t := collapse(r.Text); t != "" {
			fmt.Fprintf(&b, "rejected draft: %s\n", t)
		} else {
			b.WriteString("rejected draft: (its wording is no longer held)\n")
		}
		if len(r.Reasons) > 0 {
			fmt.Fprintf(&b, "why: %s\n", strings.Join(r.Reasons, "; "))
		}
		if n := collapse(r.Note); n != "" {
			fmt.Fprintf(&b, "reviewer's note: %s\n", n)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// collapse flattens a neighbour onto one line. A neighbour is a hint, not a
// document: newlines in it would blur the boundary between one hint and the next.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
