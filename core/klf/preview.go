package klf

import (
	"fmt"
	"sort"
	"strings"
)

// VocabularyEntry describes how a Run type renders across HTML,
// plain-text display, and CAT-tool chips. Mirrors VocabularyEntry in
// packages/format/src/vocabulary.ts.
type VocabularyEntry struct {
	Key         string
	Category    string
	HTML        HTMLRendering
	Display     TextRendering
	Chip        ChipRendering
	Constraints RunConstraints
}

// HTMLRendering is the template set applied when rendering a run as
// HTML. `{field}` placeholders expand against the run's context.
type HTMLRendering struct {
	Open        string
	Close       string
	Placeholder string
}

// TextRendering is the plain-text equivalent for terminals and for
// MT APIs that don't accept HTML.
type TextRendering struct {
	Open        string
	Close       string
	Placeholder string
}

// ChipRendering is the CAT-tool chip label + color.
type ChipRendering struct {
	Label string
	Color ChipColor
}

// ChipColor is the CSS color triple for a chip.
type ChipColor struct {
	Bg     string
	Border string
	Text   string
}

// JSXVocabulary mirrors the hand-curated default JSX vocabulary in
// packages/format/src/vocabulary.ts — the three span types the Go
// side needs to render Level-1 previews without loading the JSON
// vocabulary file. Kept in sync by a round-trip test against the
// embedded rich-jsx.json.
var JSXVocabulary = []VocabularyEntry{
	{
		Key:      "jsx:element",
		Category: "structure",
		HTML: HTMLRendering{
			Open:  `<{subType} data-neokapi-span="{id}">`,
			Close: `</{subType}>`,
		},
		Display: TextRendering{
			Open:  "<{subType}>",
			Close: "</{subType}>",
		},
		Chip: ChipRendering{
			Label: "{subType}",
			Color: ChipColor{Bg: "#e2e8f0", Border: "#94a3b8", Text: "#1e293b"},
		},
		Constraints: RunConstraints{Deletable: false, Cloneable: false, Reorderable: true},
	},
	{
		Key:      "jsx:var",
		Category: "variable",
		HTML: HTMLRendering{
			Placeholder: `<span class="neokapi-var" data-var="{equiv}" data-type="{subType}">{equiv}</span>`,
		},
		Display: TextRendering{
			Placeholder: "{{equiv}}",
		},
		Chip: ChipRendering{
			Label: "{equiv}",
			Color: ChipColor{Bg: "#dbeafe", Border: "#3b82f6", Text: "#1e40af"},
		},
		Constraints: RunConstraints{Deletable: false, Cloneable: true, Reorderable: true},
	},
	{
		Key:      "jsx:node",
		Category: "node",
		HTML: HTMLRendering{
			Placeholder: `<span class="neokapi-node" data-node="{id}" title="{data}">{equiv}</span>`,
		},
		Display: TextRendering{
			Placeholder: "«{equiv}»",
		},
		Chip: ChipRendering{
			Label: "⟨⟩",
			Color: ChipColor{Bg: "#fef3c7", Border: "#f59e0b", Text: "#92400e"},
		},
		Constraints: RunConstraints{Deletable: true, Cloneable: false, Reorderable: true},
	},
}

// VocabularyLookup resolves a run type string to its VocabularyEntry
// or returns nil if unknown. Matches the TS VocabularyLookup
// function type.
type VocabularyLookup func(typeKey string) *VocabularyEntry

// NewVocabularyLookup builds a lookup function from a flat slice.
func NewVocabularyLookup(entries []VocabularyEntry) VocabularyLookup {
	m := make(map[string]*VocabularyEntry, len(entries))
	for i := range entries {
		m[entries[i].Key] = &entries[i]
	}
	return func(key string) *VocabularyEntry {
		return m[key]
	}
}

// DefaultJSXVocabulary is a convenience lookup built from
// JSXVocabulary for tests, tools, and the Go-side preview reference
// implementation.
func DefaultJSXVocabulary() VocabularyLookup {
	return NewVocabularyLookup(JSXVocabulary)
}

// RenderBlockHTML wraps renderRuns in a <kat-block> marker — the
// same interactive wrapper neokapi's existing preview builders emit
// for every format. Matches renderBlockHtml in
// packages/format/src/preview.ts.
func RenderBlockHTML(b *Block, vocab VocabularyLookup) string {
	if vocab == nil {
		vocab = DefaultJSXVocabulary()
	}
	inner := RenderRuns(b.Source, vocab)
	return fmt.Sprintf(`<kat-block id=%q data-type=%q>%s</kat-block>`, b.ID, string(b.Type), inner)
}

// RenderRuns walks a run sequence in order and renders each run to
// its HTML form, recursing into plural / select forms. Mirrors
// renderRuns in packages/format/src/preview.ts.
func RenderRuns(runs []Run, vocab VocabularyLookup) string {
	var out strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			out.WriteString(escapeHTML(r.Text.Text))
		case r.Ph != nil:
			out.WriteString(renderEntry(vocab, r.Ph.Type, "placeholder", renderCtx{
				ID: r.Ph.ID, SubType: r.Ph.SubType, Data: r.Ph.Data, Equiv: r.Ph.Equiv,
			}))
		case r.PcOpen != nil:
			out.WriteString(renderEntry(vocab, r.PcOpen.Type, "open", renderCtx{
				ID: r.PcOpen.ID, SubType: r.PcOpen.SubType, Data: r.PcOpen.Data, Equiv: r.PcOpen.Equiv,
			}))
		case r.PcClose != nil:
			out.WriteString(renderEntry(vocab, r.PcClose.Type, "close", renderCtx{
				ID: r.PcClose.ID, SubType: r.PcClose.SubType, Data: r.PcClose.Data, Equiv: r.PcClose.Equiv,
			}))
		case r.Sub != nil:
			out.WriteString(fmt.Sprintf(`<span class="neokapi-sub" data-ref=%q>%s</span>`,
				escapeHTML(r.Sub.Ref), escapeHTML(r.Sub.Equiv)))
		case r.Plural != nil:
			out.WriteString(renderPlural(r.Plural, vocab))
		case r.Select != nil:
			out.WriteString(renderSelect(r.Select, vocab))
		}
	}
	return out.String()
}

type renderCtx struct {
	ID, SubType, Data, Equiv string
}

func renderEntry(vocab VocabularyLookup, typ, kind string, ctx renderCtx) string {
	entry := vocab(typ)
	if entry == nil {
		return fmt.Sprintf(`<span class="neokapi-unknown">%s</span>`, escapeHTML(ctx.Data))
	}
	var tmpl string
	switch kind {
	case "open":
		tmpl = entry.HTML.Open
	case "close":
		tmpl = entry.HTML.Close
	case "placeholder":
		tmpl = entry.HTML.Placeholder
	}
	return expandTemplate(tmpl, map[string]string{
		"id":      ctx.ID,
		"subType": ctx.SubType,
		"data":    escapeHTML(ctx.Data),
		"equiv":   escapeHTML(ctx.Equiv),
	})
}

// renderPlural emits forms in a deterministic order to keep output
// byte-stable for fixture tests. The TS reference implementation
// uses Object.entries order, which for our fixtures always matches
// ICU's canonical order (zero, one, two, few, many, other).
func renderPlural(p *PluralRun, vocab VocabularyLookup) string {
	var inner strings.Builder
	for _, form := range orderedPluralForms(p.Forms) {
		label := "plural:" + string(form)
		body := RenderRuns(p.Forms[form], vocab)
		fmt.Fprintf(&inner,
			`<div class="neokapi-plural-form" data-form=%q><span class="neokapi-plural-form-label">%s</span>%s</div>`,
			string(form), escapeHTML(label), body)
	}
	return fmt.Sprintf(`<span class="neokapi-plural" data-pivot=%q>%s</span>`,
		escapeHTML(p.Pivot), inner.String())
}

func renderSelect(s *SelectRun, vocab VocabularyLookup) string {
	var inner strings.Builder
	for _, key := range orderedSelectKeys(s.Cases) {
		label := "select:" + key
		body := RenderRuns(s.Cases[key], vocab)
		fmt.Fprintf(&inner,
			`<div class="neokapi-select-case" data-value=%q><span class="neokapi-select-case-label">%s</span>%s</div>`,
			key, escapeHTML(label), body)
	}
	return fmt.Sprintf(`<span class="neokapi-select" data-pivot=%q>%s</span>`,
		escapeHTML(s.Pivot), inner.String())
}

// pluralOrder is ICU's canonical plural form order; used to walk a
// PluralRun.Forms map deterministically.
var pluralOrder = []PluralForm{PluralZero, PluralOne, PluralTwo, PluralFew, PluralMany, PluralOther}

func orderedPluralForms(m map[PluralForm][]Run) []PluralForm {
	out := make([]PluralForm, 0, len(m))
	seen := make(map[PluralForm]bool, len(m))
	for _, f := range pluralOrder {
		if _, ok := m[f]; ok {
			out = append(out, f)
			seen[f] = true
		}
	}
	// Any non-standard keys get appended in sorted order after the
	// canonical ones so output stays stable across runs.
	extras := make([]string, 0)
	for k := range m {
		if !seen[k] {
			extras = append(extras, string(k))
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		out = append(out, PluralForm(k))
	}
	return out
}

func orderedSelectKeys(m map[string][]Run) []string {
	// 'other' sorts last to match ICU convention, then alpha for the rest.
	hasOther := false
	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "other" {
			hasOther = true
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if hasOther {
		keys = append(keys, "other")
	}
	return keys
}

// expandTemplate replaces `{field}` placeholders in a vocabulary
// template with values from a context map. Matches expandTemplate
// in packages/format/src/vocabulary.ts.
func expandTemplate(tmpl string, ctx map[string]string) string {
	var out strings.Builder
	out.Grow(len(tmpl))
	for i := 0; i < len(tmpl); {
		if tmpl[i] != '{' {
			out.WriteByte(tmpl[i])
			i++
			continue
		}
		// Find matching '}'. Keys are `\w+` (ASCII-ish), so we only
		// need to scan to the next brace.
		end := -1
		for j := i + 1; j < len(tmpl); j++ {
			if tmpl[j] == '}' {
				end = j
				break
			}
		}
		if end < 0 {
			out.WriteByte(tmpl[i])
			i++
			continue
		}
		key := tmpl[i+1 : end]
		if !isWordKey(key) {
			// Not a placeholder, keep as-is.
			out.WriteByte(tmpl[i])
			i++
			continue
		}
		out.WriteString(ctx[key])
		i = end + 1
	}
	return out.String()
}

func isWordKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '_' {
			return false
		}
	}
	return true
}

func escapeHTML(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
