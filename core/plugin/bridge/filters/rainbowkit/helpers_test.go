//go:build integration

package rainbowkit

import (
	"strings"

	"github.com/gokapi/gokapi/core/model"
)

const rainbowkitFilterClass = "net.sf.okapi.filters.xini.rainbowkit.XINIRainbowkitFilter"
const mimeType = "text/xml"

func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

func spanCount(b *model.Block) int {
	n := 0
	for _, seg := range b.Source {
		if seg.Content != nil {
			n += len(seg.Content.Spans)
		}
	}
	return n
}
