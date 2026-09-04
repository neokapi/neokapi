package review

import (
	"strings"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
)

// NeighbourhoodOf collects the translatable blocks either side of blocks[idx],
// in document order: the window's worth before it, nearest last, and the
// window's worth after it, nearest first. A window of zero or less means
// DefaultWindow. The unit's own key is the reader's name for it, the same key
// a translate prompt carries.
func NeighbourhoodOf(blocks []*model.Block, idx, window int, loc model.LocaleID) Neighbourhood {
	if window <= 0 {
		window = DefaultWindow
	}
	out := Neighbourhood{Key: PromptKey(blocks[idx]), Window: window}
	for i := idx - 1; i >= 0 && len(out.Before) < window; i-- {
		if n, ok := NeighbourOf(blocks[i], loc); ok {
			out.Before = append([]Neighbour{n}, out.Before...)
		}
	}
	for i := idx + 1; i < len(blocks) && len(out.After) < window; i++ {
		if n, ok := NeighbourOf(blocks[i], loc); ok {
			out.After = append(out.After, n)
		}
	}
	return out
}

// NeighbourOf projects one block into the neighbourhood, keyed by its stable
// unit key, or reports that it carries nothing a reader would see: a block
// that is not translatable, or one with no source runs.
func NeighbourOf(b *model.Block, loc model.LocaleID) (Neighbour, bool) {
	if b == nil || !b.Translatable {
		return Neighbour{}, false
	}
	src := b.SourceRuns()
	if len(src) == 0 {
		return Neighbour{}, false
	}
	n := Neighbour{Key: convergence.BlockKey(b), Source: src}
	if t := b.Target(loc); t != nil {
		n.Target = t.Runs
		n.Status = string(t.Status)
	}
	return n, true
}

// PromptKey is the key a translate prompt sends for a block: the reader's name
// for it, which is what makes a bare "Save" mean something, and the stable
// unit key where the reader named nothing.
func PromptKey(b *model.Block) string {
	if b == nil {
		return ""
	}
	if name := strings.TrimSpace(b.Name); name != "" {
		return name
	}
	return convergence.BlockKey(b)
}
