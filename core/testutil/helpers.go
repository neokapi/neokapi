package testutil

import (
	"io"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/require"
)

// RawDocFromString creates a RawDocument from a string.
func RawDocFromString(content string, sourceLocale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          "test://input",
		SourceLocale: sourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(content)),
	}
}

// RawDocFromReader creates a RawDocument from a reader.
func RawDocFromReader(r io.Reader, uri string, sourceLocale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          uri,
		SourceLocale: sourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(r),
	}
}

// CollectParts reads all Parts from a PartResult channel.
func CollectParts(t *testing.T, ch <-chan model.PartResult) []*model.Part {
	t.Helper()
	var parts []*model.Part
	for result := range ch {
		require.NoError(t, result.Error)
		parts = append(parts, result.Part)
	}
	return parts
}

// CollectBlocks reads all Parts and returns only the Block resources.
func CollectBlocks(t *testing.T, ch <-chan model.PartResult) []*model.Block {
	t.Helper()
	parts := CollectParts(t, ch)
	return FilterBlocks(parts)
}

// FilterBlocks returns only Block resources from a list of Parts.
func FilterBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}

// FindFirstBlock returns the first Block from a list of Parts, or nil.
func FindFirstBlock(parts []*model.Part) *model.Block {
	blocks := FilterBlocks(parts)
	if len(blocks) == 0 {
		return nil
	}
	return blocks[0]
}

// BlockTexts returns the source text of each Block.
func BlockTexts(blocks []*model.Block) []string {
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.SourceText()
	}
	return texts
}

// PartsToChannel sends Parts to a channel and closes it.
func PartsToChannel(parts []*model.Part) <-chan *model.Part {
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	return ch
}

// CollectFromChannel collects all Parts from a channel.
func CollectFromChannel(ch <-chan *model.Part) []*model.Part {
	var parts []*model.Part
	for p := range ch {
		parts = append(parts, p)
	}
	return parts
}
