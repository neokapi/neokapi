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

// NewSpanClassifyTool creates a new inline-code classification tool.
// The tool name (span-classify) is kept for backwards compatibility
// with existing flow definitions; internally it reclassifies the Type
// field on inline-code runs (Ph / PcOpen / PcClose).
func NewSpanClassifyTool(cfg *SpanClassifyConfig) *SpanClassifyTool {
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()

	t := &SpanClassifyTool{
		vocab: vocab,
	}
	t.ToolName = "span-classify"
	t.ToolDescription = "Reclassifies code:markup inline-code runs into semantic vocabulary types"
	// Transform: span-classify rewrites source and target inline-code runs.
	t.Transform = t.handleBlock
	return t
}

func (t *SpanClassifyTool) handleBlock(v tool.SourceView) error {
	// Classify inline codes in the source content.
	v.SetSourceRuns(t.classifyRuns(v.SourceRuns()))

	// Classify inline codes in every target locale's content.
	for _, loc := range v.TargetLocales() {
		v.SetTargetRuns(loc, t.classifyRuns(v.TargetRuns(loc)))
	}

	return nil
}

// classifyRuns walks a run sequence and reclassifies any "code:markup"
// placeholder / paired-code runs into proper vocabulary types,
// following the same strategies as the legacy Span-based classifier:
// first the Okapi sub-type map, then HTML-element parsing from the
// raw Data field.
func (t *SpanClassifyTool) classifyRuns(runs []model.Run) []model.Run {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch {
		case r.Ph != nil:
			out[i] = model.Run{Ph: t.classifyPh(r.Ph)}
		case r.PcOpen != nil:
			out[i] = model.Run{PcOpen: t.classifyPcOpen(r.PcOpen)}
		case r.PcClose != nil:
			out[i] = model.Run{PcClose: t.classifyPcClose(r.PcClose)}
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, v := range r.Plural.Forms {
				forms[k] = t.classifyRuns(v)
			}
			out[i] = model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}}
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, v := range r.Select.Cases {
				cases[k] = t.classifyRuns(v)
			}
			out[i] = model.Run{Select: &model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}}
		default:
			out[i] = r
		}
	}
	return out
}

func (t *SpanClassifyTool) classifyPh(ph *model.PlaceholderRun) *model.PlaceholderRun {
	semType := t.resolveSemanticType(ph.Type, ph.SubType, ph.Data)
	if semType == "" {
		return ph
	}
	info := t.vocab.Lookup(semType)
	copy := *ph
	copy.Type = semType
	if info != nil {
		copy.Constraints = &model.RunConstraints{
			Deletable:   info.Constraints.Deletable,
			Cloneable:   info.Constraints.Cloneable,
			Reorderable: info.Constraints.Reorderable,
		}
		copy.Disp = info.Display.Placeholder
		if copy.Equiv == "" {
			copy.Equiv = info.Equiv
		}
	}
	return &copy
}

func (t *SpanClassifyTool) classifyPcOpen(pc *model.PcOpenRun) *model.PcOpenRun {
	semType := t.resolveSemanticType(pc.Type, pc.SubType, pc.Data)
	if semType == "" {
		return pc
	}
	info := t.vocab.Lookup(semType)
	copy := *pc
	copy.Type = semType
	if info != nil {
		copy.Constraints = &model.RunConstraints{
			Deletable:   info.Constraints.Deletable,
			Cloneable:   info.Constraints.Cloneable,
			Reorderable: info.Constraints.Reorderable,
		}
		copy.Disp = info.Display.Open
		if copy.Equiv == "" {
			copy.Equiv = info.Equiv
		}
	}
	return &copy
}

func (t *SpanClassifyTool) classifyPcClose(pc *model.PcCloseRun) *model.PcCloseRun {
	semType := t.resolveSemanticType(pc.Type, pc.SubType, pc.Data)
	if semType == "" {
		return pc
	}
	info := t.vocab.Lookup(semType)
	copy := *pc
	copy.Type = semType
	if info != nil && copy.Equiv == "" {
		copy.Equiv = info.Equiv
	}
	return &copy
}

// resolveSemanticType returns the vocabulary-aware type for a
// run whose legacy type was "code:markup", using the same strategies
// as the old Span-based classifier: Okapi sub-type map, then
// HTML-element name parsed out of the raw Data field. Returns the
// empty string when no classification is possible.
func (t *SpanClassifyTool) resolveSemanticType(typ, subType, data string) string {
	if typ != "code:markup" {
		return ""
	}
	if subType != "" {
		if semType, ok := okapiSubTypeMap[subType]; ok {
			return semType
		}
	}
	if tagName := extractTagName(data); tagName != "" {
		if semType, ok := htmlToSemanticType[strings.ToLower(tagName)]; ok {
			return semType
		}
	}
	return ""
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
