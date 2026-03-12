package tools

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// SpanClassifyConfig holds configuration for the span classify tool.
type SpanClassifyConfig struct{}

// SpanClassifyTool reclassifies "code:markup" spans into proper semantic
// vocabulary types by inspecting Span.Data and Span.SubType. This tool is
// designed to run after bridge readers (Okapi, XLIFF) that produce generic
// code:markup spans.
type SpanClassifyTool struct {
	tool.BaseTool
	vocab *model.VocabularyRegistry
}

// htmlToSemanticType maps HTML element names to vocabulary semantic types.
// Used for classifying spans based on their Data field.
var htmlToSemanticType = map[string]string{
	"b": "fmt:bold", "strong": "fmt:bold",
	"i": "fmt:italic", "em": "fmt:italic",
	"u": "fmt:underline",
	"s": "fmt:strikethrough", "del": "fmt:strikethrough", "strike": "fmt:strikethrough",
	"a":    "link:hyperlink",
	"code": "fmt:code", "kbd": "fmt:code", "samp": "fmt:code", "tt": "fmt:code",
	"sub": "fmt:subscript", "sup": "fmt:superscript",
	"mark": "fmt:highlight",
	"br":   "struct:break", "hr": "struct:break",
	"img": "media:image",
}

// okapiSubTypeMap maps known Okapi Code type strings to vocabulary types.
var okapiSubTypeMap = map[string]string{
	"okapi:bold":          "fmt:bold",
	"okapi:italic":        "fmt:italic",
	"okapi:underline":     "fmt:underline",
	"okapi:strikethrough": "fmt:strikethrough",
	"okapi:link":          "link:hyperlink",
	"okapi:code":          "fmt:code",
	"okapi:subscript":     "fmt:subscript",
	"okapi:superscript":   "fmt:superscript",
	"okapi:break":         "struct:break",
	"okapi:image":         "media:image",
}

// NewSpanClassifyTool creates a new span classification tool.
func NewSpanClassifyTool(cfg *SpanClassifyConfig) *SpanClassifyTool {
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()

	t := &SpanClassifyTool{
		vocab: vocab,
	}
	t.ToolName = "span-classify"
	t.ToolDescription = "Reclassifies code:markup spans into semantic vocabulary types"
	t.HandleBlockFn = t.handleBlock
	return t
}

func (t *SpanClassifyTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}

	// Classify spans in source fragments.
	for _, seg := range block.Source {
		if seg.Content != nil {
			t.classifyFragmentSpans(seg.Content)
		}
	}

	// Classify spans in all target fragments.
	for _, segs := range block.Targets {
		for _, seg := range segs {
			if seg.Content != nil {
				t.classifyFragmentSpans(seg.Content)
			}
		}
	}

	return part, nil
}

func (t *SpanClassifyTool) classifyFragmentSpans(frag *model.Fragment) {
	for _, s := range frag.Spans {
		if s.Type != "code:markup" {
			continue
		}

		// Strategy 1: Check SubType against known Okapi type strings.
		if s.SubType != "" {
			if semType, ok := okapiSubTypeMap[s.SubType]; ok {
				t.applyClassification(s, semType)
				continue
			}
		}

		// Strategy 2: Parse Data to extract HTML element name.
		tagName := extractTagName(s.Data)
		if tagName != "" {
			if semType, ok := htmlToSemanticType[strings.ToLower(tagName)]; ok {
				t.applyClassification(s, semType)
				continue
			}
		}

		// Strategy 3: Leave as code:markup (no classification possible).
	}
}

func (t *SpanClassifyTool) applyClassification(s *model.Span, semType string) {
	s.Type = semType

	// Populate display metadata from vocabulary.
	info := t.vocab.Lookup(semType)
	if info == nil {
		return
	}

	s.Deletable = info.Constraints.Deletable
	s.Cloneable = info.Constraints.Cloneable
	s.CanReorder = info.Constraints.Reorderable

	switch s.SpanType {
	case model.SpanOpening:
		s.DisplayText = info.Display.Open
		s.EquivText = info.Equiv
	case model.SpanClosing:
		s.DisplayText = info.Display.Close
		s.EquivText = info.Equiv
	case model.SpanPlaceholder:
		s.DisplayText = info.Display.Placeholder
		s.EquivText = info.Equiv
	}
}

// extractTagName extracts the HTML element name from raw markup data.
// Handles "<b>", "</b>", "<br/>", '<a href="url">', etc.
func extractTagName(data string) string {
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, "<") {
		return ""
	}
	// Remove leading < and optional /
	data = strings.TrimPrefix(data, "<")
	data = strings.TrimPrefix(data, "/")

	// Extract tag name (sequence of alphanumeric chars).
	var name strings.Builder
	for _, r := range data {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			name.WriteRune(r)
		} else {
			break
		}
	}
	return name.String()
}
