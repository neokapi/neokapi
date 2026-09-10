package sectionedit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	htmlformat "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/model"
)

// bindNativeRanges joins adapter source boundaries to the ordinary reader's
// blocks using advisory source spans. Text is an agreement check, never a
// fuzzy selector. Missing or straddling locations fail closed.
func bindNativeRanges(ctx context.Context, name string, source []byte, sections []Section) error {
	if len(sections) == 0 {
		return nil
	}
	blocks, err := readNativeBlocks(ctx, name, source)
	if err != nil {
		return err
	}
	for i := range sections {
		section := &sections[i]
		headingIndex := -1
		for j, block := range blocks {
			span, ok := block.SourceSpan()
			if !ok || span.Part != section.sourcePart || block.HeadingLevel() != section.Level {
				continue
			}
			if span.Start < section.headingStart || span.End > section.bodyStart {
				continue
			}
			if headingIndex >= 0 {
				return fmt.Errorf("section %q has ambiguous reader heading spans", section.Title)
			}
			headingIndex = j
		}
		if headingIndex < 0 {
			return fmt.Errorf("section %q has no recognized reader heading span", section.Title)
		}
		heading := blocks[headingIndex]
		if normalizedTitle(heading.SourceText()) != normalizedTitle(section.Title) {
			return fmt.Errorf("section %q disagrees with the reader heading", section.Title)
		}
		section.ID = heading.ID
		section.Range = model.BlockRange{Heading: model.RefForBlock(heading), Body: []model.BlockRef{}}
		for _, block := range blocks[headingIndex+1:] {
			span, ok := block.SourceSpan()
			if !ok {
				// Other package parts are separate content ranges. Their block
				// metadata is already recognized by the native OpenXML reader.
				if name == "docx" && block.Properties["partPath"] != section.sourcePart {
					continue
				}
				return fmt.Errorf("section %q: reader block %q has no source span", section.Title, block.ID)
			}
			if span.Part != section.sourcePart {
				continue
			}
			if span.Start >= section.bodyEnd {
				ref := model.RefForBlock(block)
				section.Range.EndBefore = &ref
				break
			}
			if span.Start < section.bodyStart || span.End > section.bodyEnd {
				return fmt.Errorf("section %q straddles reader block %q", section.Title, block.ID)
			}
			section.Range.Body = append(section.Range.Body, model.RefForBlock(block))
		}
	}
	return nil
}

func normalizedTitle(title string) string {
	return strings.Join(strings.Fields(title), " ")
}

func readNativeBlocks(ctx context.Context, name string, source []byte) ([]*model.Block, error) {
	var reader format.DataFormatReader
	switch name {
	case "markdown":
		reader = markdown.NewReader()
	case "html":
		htmlReader := htmlformat.NewReader()
		store := format.NewMemorySkeletonStore()
		defer store.Close()
		htmlReader.SetSkeletonStore(store)
		reader = htmlReader
	case "docx":
		reader = openxml.NewReader()
	default:
		return nil, fmt.Errorf("unsupported reader %q", name)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer reader.Close()
	doc := &model.RawDocument{
		URI: "section-source." + name, Encoding: "UTF-8", SourceLocale: model.LocaleEnglish,
		Reader: io.NopCloser(bytes.NewReader(source)), ReaderAt: bytes.NewReader(source), Size: int64(len(source)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, err
	}
	blocks := []*model.Block{}
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, result.Error
		}
		if result.Part == nil {
			continue
		}
		if block, ok := result.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}
