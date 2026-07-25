package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// The corpus is built to stress the things batching is documented to break, not
// to be representative prose.
//
// Two properties matter, and the literature is emphatic about both:
//
//   - Batch degradation is worst when each item's input is LONG. A corpus of
//     nothing but "Save" would flatter every N and measure nothing.
//   - The failure is structural — dropped, merged and renumbered segments, and
//     mangled markup — not stylistic. So the corpus carries placeholders and
//     inline tags whose survival is objectively checkable without a reference
//     translation or a human.
//
// It also carries near-duplicate keys with the same source text ("Save" under two
// different keys), because that is exactly where positional mapping used to
// corrupt silently and where a key earns its tokens.

// Case is one corpus entry.
type Case struct {
	Key    string
	Source string
	// Kind labels the stressor, so a result can be read by category rather than
	// as one undifferentiated percentage.
	Kind string
}

// TestCorpus is the fixed input for every N in a sweep.
type TestCorpus struct {
	Cases []Case
}

// Words counts the source words the corpus puts through the model. This is the
// denominator of the cost metric: content budgets are denominated in words, not
// tokens, and a token count is a fact about a vendor's tokenizer rather than about
// the work.
func (c TestCorpus) Words() int {
	n := 0
	for _, tc := range c.Cases {
		n += len(strings.Fields(tc.Source))
	}
	return n
}

// Digest identifies the exact corpus a run was measured on. A curve measured on
// a different corpus is a different experiment, however similar it looks, so the
// history records this and the dashboard refuses to plot mismatched runs on one
// line.
func (c TestCorpus) Digest() string {
	h := sha256.New()
	for _, tc := range c.Cases {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00", tc.Kind, tc.Key, tc.Source)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func (c TestCorpus) Describe() string {
	byKind := map[string]int{}
	for _, tc := range c.Cases {
		byKind[tc.Kind]++
	}
	var parts []string
	for _, k := range []string{"ui", "ui-ambiguous", "placeholder", "inline-tag", "prose"} {
		if n := byKind[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	return fmt.Sprintf("%d blocks (%s)", len(c.Cases), strings.Join(parts, ", "))
}

// Blocks renders the corpus as translatable blocks, with the key in Name so the
// key context has something to send.
func (c TestCorpus) Blocks() []*model.Block {
	out := make([]*model.Block, len(c.Cases))
	for i, tc := range c.Cases {
		b := model.NewBlock(fmt.Sprintf("tu%d", i+1), tc.Source)
		b.Name = tc.Key
		b.Translatable = true
		out[i] = b
	}
	return out
}

// Corpus returns the fixed evaluation corpus.
func Corpus() TestCorpus {
	c := TestCorpus{}

	// Short UI strings. Cheap to translate, and the case where a dropped segment
	// is easiest for a model to get away with.
	ui := []struct{ key, text string }{
		{"file.open", "Open"},
		{"file.close", "Close"},
		{"file.recent", "Recent files"},
		{"edit.undo", "Undo"},
		{"edit.redo", "Redo"},
		{"view.zoom_in", "Zoom in"},
		{"view.fullscreen", "Full screen"},
		{"help.about", "About"},
		{"account.sign_out", "Sign out"},
		{"search.placeholder", "Search"},
	}
	for _, u := range ui {
		c.Cases = append(c.Cases, Case{Key: u.key, Source: u.text, Kind: "ui"})
	}

	// The same word under different keys — a verb in one place, a noun in another.
	// Without the key these are indistinguishable, and in German they are
	// different words.
	c.Cases = append(c.Cases,
		Case{Key: "dialog.button.save", Source: "Save", Kind: "ui-ambiguous"},
		Case{Key: "settings.section.save", Source: "Save", Kind: "ui-ambiguous"},
		Case{Key: "toolbar.button.open", Source: "Open", Kind: "ui-ambiguous"},
		Case{Key: "status.state.open", Source: "Open", Kind: "ui-ambiguous"},
	)

	// Placeholders. A translation that loses one cannot be written back into the
	// source file — it is not a worse translation, it is not a translation.
	placeholders := []struct{ key, text string }{
		{"cart.items", "You have {0} items in your cart"},
		{"greeting.user", "Welcome back, {{name}}"},
		{"upload.progress", "Uploaded %d of %d files"},
		{"quota.exceeded", "You have used {used} of {total} GB"},
		{"invoice.due", "Invoice {invoice_id} is due on {date}"},
		{"sync.error", "Could not sync %s: %s"},
	}
	for _, p := range placeholders {
		c.Cases = append(c.Cases, Case{Key: p.key, Source: p.text, Kind: "placeholder"})
	}

	// Inline markup, rendered the way kapi renders it. Reordering a tag is as
	// fatal as dropping it.
	tags := []struct{ key, text string }{
		{"terms.accept", `I accept the <ph id="1"/>terms of service<ph id="2"/>`},
		{"banner.upgrade", `Your trial ends soon. <ph id="1"/>Upgrade now<ph id="2"/> to keep your data.`},
		{"footer.contact", `Questions? <ph id="1"/>Contact support<ph id="2"/> any time.`},
		{"warning.delete", `This will <ph id="1"/>permanently<ph id="2"/> delete your account.`},
	}
	for _, tg := range tags {
		c.Cases = append(c.Cases, Case{Key: tg.key, Source: tg.text, Kind: "inline-tag"})
	}

	// Prose. Long items are where the batching literature says degradation bites
	// hardest, and a corpus of UI strings alone would hide it.
	prose := []struct{ key, text string }{
		{"docs.intro", "The engine reads a document, hands each translatable unit to the tools you configured, and writes the result back in the original format. Nothing else in the file is touched."},
		{"docs.caching", "A translation is cached against the content of the block and the configuration that produced it. Change the model, the glossary, or the prompt, and the affected strings are translated again rather than served from a cache a different configuration produced."},
		{"docs.review", "Review does not replace the checks. The checks are deterministic and cheap and run on every block; review is a judgement call, made by a model, on the blocks the checks could not settle."},
		{"docs.plugins", "A plugin is a separate binary that the engine launches as a subprocess and speaks to over gRPC. It can contribute formats, tools, and commands, and it cannot corrupt the engine's memory if it crashes."},
		{"docs.terminology", "Terminology is enforced at generation time, not merely checked afterwards. A term in the terms store is rendered into the prompt, so the model is asked for the right word rather than corrected for using the wrong one."},
		{"docs.gates", "A ship gate is a question about a locale, not about a file: is every block that must be translated translated, and does every block that must be reviewed carry a review? A build that cannot answer yes does not ship that locale."},
	}
	for _, p := range prose {
		c.Cases = append(c.Cases, Case{Key: p.key, Source: p.text, Kind: "prose"})
	}

	return c
}

var (
	// The placeholder shapes the framework's own constraint names.
	placeholderRe = regexp.MustCompile(`\{\{?[a-zA-Z0-9_]+\}?\}|%[sdvf]`)
	inlineTagRe   = regexp.MustCompile(`<ph id="\d+"/>`)
)

// placeholdersIntact reports whether every placeholder in the source survives in
// the translation. Order may legitimately change between languages; presence and
// count may not.
func placeholdersIntact(source, target string) bool {
	return sameMultiset(placeholderRe.FindAllString(source, -1), placeholderRe.FindAllString(target, -1))
}

// tagsIntact reports whether every inline tag survives. A dropped or duplicated
// tag cannot be mapped back onto the source runs.
func tagsIntact(source, target string) bool {
	return sameMultiset(inlineTagRe.FindAllString(source, -1), inlineTagRe.FindAllString(target, -1))
}

func sameMultiset(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}
	counts := map[string]int{}
	for _, w := range want {
		counts[w]++
	}
	for _, g := range got {
		counts[g]--
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}
