//go:build parity

package parity

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// CanonicalPart is the part-comparison value used by CompareEvents. It
// projects model.Part onto only the fields whose semantics must match
// between native and bridge implementations: type, identity, and
// rendered translatable text. Implementation-specific fields (skeleton
// IDs, source URIs, internal counters) are deliberately omitted.
type CanonicalPart struct {
	Type         model.PartType
	BlockID      string `json:",omitempty"`
	Translatable bool   `json:",omitempty"`
	Source       string `json:",omitempty"`
	Targets      string `json:",omitempty"`
	GroupID      string `json:",omitempty"`
	GroupType    string `json:",omitempty"`
	LayerID      string `json:",omitempty"`
	LayerName    string `json:",omitempty"`
	DataID       string `json:",omitempty"`
	MediaMime    string `json:",omitempty"`
	MediaSize    int    `json:",omitempty"`
}

// Canonicalize maps a part stream to its canonical comparison form.
func Canonicalize(parts []*model.Part) []CanonicalPart {
	out := make([]CanonicalPart, 0, len(parts))
	for _, p := range parts {
		c := CanonicalPart{Type: p.Type}
		switch p.Type {
		case model.PartBlock:
			if b, ok := p.Resource.(*model.Block); ok {
				c.BlockID = b.ID
				c.Translatable = b.Translatable
				c.Source = renderBlockSource(b)
				c.Targets = renderBlockTargets(b)
			}
		case model.PartGroupStart:
			if g, ok := p.Resource.(*model.GroupStart); ok {
				c.GroupID = g.ID
				c.GroupType = g.Type
			}
		case model.PartGroupEnd:
			if g, ok := p.Resource.(*model.GroupEnd); ok {
				c.GroupID = g.ID
			}
		case model.PartLayerStart:
			if l, ok := p.Resource.(*model.Layer); ok {
				c.LayerID = l.ID
				c.LayerName = l.Name
			}
		case model.PartLayerEnd:
			if l, ok := p.Resource.(*model.Layer); ok {
				c.LayerID = l.ID
			}
		case model.PartData:
			if d, ok := p.Resource.(*model.Data); ok {
				c.DataID = d.ID
			}
		case model.PartMedia:
			if md, ok := p.Resource.(*model.Media); ok {
				c.MediaMime = md.MimeType
				c.MediaSize = len(md.Data)
			}
		}
		out = append(out, c)
	}
	return out
}

// renderBlockSource concatenates segment text including span data, so
// inline-code differences surface in the diff. Whitespace is collapsed
// to make text comparison stable across implementations that emit
// different whitespace tokens.
func renderBlockSource(b *model.Block) string {
	var buf strings.Builder
	for i, seg := range b.Source {
		if i > 0 {
			buf.WriteByte(' ')
		}
		if seg != nil {
			buf.WriteString(model.RenderRunsWithData(seg.Runs))
		}
	}
	return collapseWhitespace(buf.String())
}

// renderBlockTargets concatenates target locales' segment text in
// locale-sorted order so the field is order-independent.
func renderBlockTargets(b *model.Block) string {
	if len(b.Targets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(b.Targets))
	for locale, segs := range b.Targets {
		var buf strings.Builder
		buf.WriteString(string(locale))
		buf.WriteByte('=')
		for i, seg := range segs {
			if i > 0 {
				buf.WriteByte(' ')
			}
			if seg != nil {
				buf.WriteString(model.RenderRunsWithData(seg.Runs))
			}
		}
		parts = append(parts, collapseWhitespace(buf.String()))
	}
	// Locale-keyed map iteration is non-deterministic — sort for
	// reproducible diffs.
	sortStrings(parts)
	return strings.Join(parts, "|")
}

// collapseWhitespace flattens runs of whitespace to single spaces and
// trims leading/trailing space. Different implementations sometimes
// preserve different leading whitespace tokens (e.g. JSON pretty
// printing); the parity bar is "same translatable text", not "same
// whitespace serialization".
func collapseWhitespace(s string) string {
	var buf strings.Builder
	prevSpace := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				buf.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		buf.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(buf.String())
}

func sortStrings(s []string) {
	// Tiny insertion sort — slices are short (locale count).
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
