package change_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// guideRuns is the paragraph of the contract's examples:
//
//	<p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>
func guideRuns() []model.Run {
	return []model.Run{
		model.TextR("Read the "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="https://old.example/guide">`, Attrs: map[string]string{"href": "https://old.example/guide"}}),
		model.TextR("shop guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
		model.TextR(" before you "),
		model.PcOpenR(model.PcOpenRun{ID: "2", Type: "fmt:bold", Data: "<b>"}),
		model.TextR("order"),
		model.PcCloseR(model.PcCloseRun{ID: "2", Type: "fmt:bold", Data: "</b>"}),
		model.TextR("."),
	}
}

// guideBlock is the paragraph in English with a Norwegian translation.
func guideBlock() *model.Block {
	b := model.NewRunsBlock("tu1", guideRuns())
	b.Name = "p"
	b.SourceLocale = "en"
	nb := []model.Run{
		model.TextR("Les "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="https://old.example/guide">`, Attrs: map[string]string{"href": "https://old.example/guide"}}),
		model.TextR("butikkguiden"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
		model.TextR(" før du "),
		model.PcOpenR(model.PcOpenRun{ID: "2", Type: "fmt:bold", Data: "<b>"}),
		model.TextR("bestiller"),
		model.PcCloseR(model.PcCloseRun{ID: "2", Type: "fmt:bold", Data: "</b>"}),
		model.TextR("."),
	}
	b.SetTarget("nb", &model.Target{Runs: nb, Status: model.TargetStatusTranslated})
	return b
}

// pluralRuns is an ICU message: "You have {count, plural, one {# item} other
// {# items}} in your basket."
func pluralRuns() []model.Run {
	n := func() model.Run {
		return model.PhR(model.PlaceholderRun{ID: "n", Type: "code:variable", Data: "#", Equiv: "count"})
	}
	return []model.Run{
		model.TextR("You have "),
		model.PluralR(model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
			model.PluralOne:   {n(), model.TextR(" item")},
			model.PluralOther: {n(), model.TextR(" items")},
		}}),
		model.TextR(" in your basket."),
	}
}

// codeSpanRuns is "Run `kapi check` in CI." as the Markdown reader builds it.
func codeSpanRuns() []model.Run {
	return []model.Run{
		model.TextR("Run "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:code", Data: "`"}),
		{Text: &model.TextRun{Text: "kapi check", NoTranslate: true}},
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:code", Data: "`"}),
		model.TextR(" in CI."),
	}
}

var (
	person = change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Now: fixedClock}
	agent  = change.BlockEnv{Actor: change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"}, Now: fixedClock}
	tool   = change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: "pseudo-translate"}, Guards: change.Report}
)

func fixedClock() time.Time { return time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC) }

func ref(edition string) change.Ref {
	k, err := model.ParseEditionKey(edition)
	if err != nil {
		panic(err)
	}
	return change.Ref{Doc: "docs/guide.html", Block: "p", Edition: k}
}

func setText(edition, ifMatch, s string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: ref(edition), IfMatch: ifMatch, Body: &change.SetContent{Text: new(s)}}
}

func setRuns(edition, ifMatch string, runs []model.Run) change.Op {
	if runs == nil {
		runs = []model.Run{}
	}
	return change.Op{Kind: change.KindSetContent, At: ref(edition), IfMatch: ifMatch, Body: &change.SetContent{Runs: runs}}
}

func replace(edition, ifMatch string, edits ...change.TextEdit) change.Op {
	return change.Op{Kind: change.KindReplaceText, At: ref(edition), IfMatch: ifMatch, Body: &change.ReplaceText{Edits: edits}}
}

func find(s, with string) change.TextEdit {
	return change.TextEdit{Find: new(s), Text: with}
}

func span(start, end int, with string) change.TextEdit {
	return change.TextEdit{Start: &start, End: &end, Text: with}
}

// shape renders runs compactly: text verbatim, [text] for do-not-translate
// text, <id>/</id> for paired codes, {id} for placeholders, and a plural or
// select as {pivot: form=… | form=…}.
func shape(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil && r.Text.NoTranslate:
			fmt.Fprintf(&b, "[%s]", r.Text.Text)
		case r.Text != nil:
			b.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			fmt.Fprintf(&b, "<%s>", r.PcOpen.ID)
		case r.PcClose != nil:
			fmt.Fprintf(&b, "</%s>", r.PcClose.ID)
		case r.Ph != nil:
			fmt.Fprintf(&b, "{%s}", r.Ph.ID)
		case r.Sub != nil:
			fmt.Fprintf(&b, "[sub:%s]", r.Sub.ID)
		case r.Plural != nil:
			fmt.Fprintf(&b, "{%s:", r.Plural.Pivot)
			for _, f := range []model.PluralForm{model.PluralZero, model.PluralOne, model.PluralTwo, model.PluralFew, model.PluralMany, model.PluralOther} {
				if form, ok := r.Plural.Forms[f]; ok {
					fmt.Fprintf(&b, " %s=%s", f, shape(form))
				}
			}
			b.WriteString("}")
		case r.Select != nil:
			fmt.Fprintf(&b, "{%s: select}", r.Select.Pivot)
		}
	}
	return b.String()
}
