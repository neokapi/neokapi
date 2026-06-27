package projection

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// recordingSink records inline events as a flat string trace for assertions.
type recordingSink struct{ trace []string }

func (s *recordingSink) Text(t string) { s.trace = append(s.trace, "text:"+t) }
func (s *recordingSink) Open(typ string, attrs map[string]string) {
	if h := attrs[model.AttrHref]; h != "" {
		s.trace = append(s.trace, fmt.Sprintf("open:%s(%s)", typ, h))
		return
	}
	s.trace = append(s.trace, "open:"+typ)
}
func (s *recordingSink) Close(typ string) { s.trace = append(s.trace, "close:"+typ) }
func (s *recordingSink) Placeholder(typ, equiv string, attrs map[string]string) {
	if src := attrs[model.AttrSrc]; src != "" {
		s.trace = append(s.trace, fmt.Sprintf("ph:%s(%s)", typ, src))
		return
	}
	s.trace = append(s.trace, fmt.Sprintf("ph:%s=%s", typ, equiv))
}

func TestWalkInline_BoldAndLink(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Some "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold"}},
		{Text: &model.TextRun{Text: "bold"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold"}},
		{Text: &model.TextRun{Text: " and a "}},
		{PcOpen: &model.PcOpenRun{ID: "2", Type: "link:hyperlink", Attrs: map[string]string{model.AttrHref: "https://x.com"}}},
		{Text: &model.TextRun{Text: "link"}},
		{PcClose: &model.PcCloseRun{ID: "2", Type: "link:hyperlink"}},
		{Text: &model.TextRun{Text: "."}},
	}
	s := &recordingSink{}
	WalkInline(runs, s)
	assert.Equal(t, []string{
		"text:Some ",
		"open:fmt:bold", "text:bold", "close:fmt:bold",
		"text: and a ",
		"open:link:hyperlink(https://x.com)", "text:link", "close:link:hyperlink",
		"text:.",
	}, s.trace)
}

func TestWalkInline_Image(t *testing.T) {
	runs := []model.Run{
		{Ph: &model.PlaceholderRun{ID: "i", Type: "media:image", Equiv: "logo",
			Attrs: map[string]string{model.AttrSrc: "/logo.png", model.AttrAlt: "Logo"}}},
	}
	s := &recordingSink{}
	WalkInline(runs, s)
	assert.Equal(t, []string{"ph:media:image(/logo.png)"}, s.trace)
}

func TestWalkInline_PluralOtherBranch(t *testing.T) {
	runs := []model.Run{
		{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
			model.PluralOne:   {{Text: &model.TextRun{Text: "one item"}}},
			model.PluralOther: {{Text: &model.TextRun{Text: "many items"}}},
		}}},
	}
	s := &recordingSink{}
	WalkInline(runs, s)
	assert.Equal(t, []string{"text:many items"}, s.trace)
}

func TestWalkInline_NestedFormatting(t *testing.T) {
	runs := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold"}},
		{PcOpen: &model.PcOpenRun{ID: "2", Type: "fmt:italic"}},
		{Text: &model.TextRun{Text: "x"}},
		{PcClose: &model.PcCloseRun{ID: "2", Type: "fmt:italic"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold"}},
	}
	s := &recordingSink{}
	WalkInline(runs, s)
	assert.Equal(t, []string{"open:fmt:bold", "open:fmt:italic", "text:x", "close:fmt:italic", "close:fmt:bold"}, s.trace)
}
