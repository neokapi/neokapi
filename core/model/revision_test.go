package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

func linkRuns(href string) []model.Run {
	return []model.Run{
		model.TextR("Read "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="` + href + `">`, Attrs: map[string]string{"href": href}}),
		model.TextR("docs"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
	}
}

func TestRunsRevision_Shape(t *testing.T) {
	rev := model.RunsRevision(model.Variant("en"), linkRuns("x"))
	assert.Regexp(t, `^r:[0-9a-f]{16}$`, rev)
	assert.Equal(t, rev, model.RunsRevision(model.Variant("en"), linkRuns("x")), "a function of its input")
}

// The revision moves with every change a reader could see in the content, and
// with nothing else.
func TestRunsRevision_CoversCodesAndKeyOnly(t *testing.T) {
	base := model.RunsRevision(model.Variant("en"), linkRuns("x"))
	tests := []struct {
		name  string
		rev   string
		moves bool
	}{
		{"an href change moves it, which the content hash misses", model.RunsRevision(model.Variant("en"), linkRuns("y")), true},
		{"removing the link moves it", model.RunsRevision(model.Variant("en"), []model.Run{model.TextR("Read docs")}), true},
		{"the same runs under another edition", model.RunsRevision(model.Variant("fr"), linkRuns("x")), true},
		{"a locale spelled another way", model.RunsRevision(model.EditionKey{Locale: "EN"}, linkRuns("x")), false},
		{"a channel edition", model.RunsRevision(model.EditionKey{Locale: "en", Channel: "short"}, linkRuns("x")), true},
		{"a run marked do-not-translate", model.RunsRevision(model.Variant("en"), []model.Run{model.TextR("Read "), {Text: &model.TextRun{Text: "docs", NoTranslate: true}}}), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.moves, tc.rev != base)
		})
	}
	assert.Equal(t, model.ComputeContentHash(model.RunsText(linkRuns("x"))), model.ComputeContentHash(model.RunsText(linkRuns("y"))),
		"the content hash cannot tell the two apart")
}

func TestEditionRevision_IgnoresStatusAndOtherEditions(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	b.SetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{model.TextR("Bonjour")}, Status: model.Status(model.TargetStatusDraft)})
	src := model.EditionRevision(b, model.EditionKey{})
	fr := model.EditionRevision(b, model.Variant("fr"))

	assert.Equal(t, src, model.EditionRevision(b, model.Variant("en")), "every key that reaches an edition gives one revision")
	assert.Equal(t, model.AbsentRevision, model.EditionRevision(b, model.Variant("de")))

	b.SetEditionStatus(model.Variant("fr"), model.Status(model.TargetStatusEstablished))
	b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusEstablished))
	assert.Equal(t, fr, model.EditionRevision(b, model.Variant("fr")), "a decision does not move a revision")
	assert.Equal(t, src, model.EditionRevision(b, model.EditionKey{}))

	b.SetSourceText("Hello there")
	assert.Equal(t, fr, model.EditionRevision(b, model.Variant("fr")), "a source edit leaves a translation's revision alone")
	assert.NotEqual(t, src, model.EditionRevision(b, model.EditionKey{}))
}

// A translation's revision is the one EditionRevision gives it, and the source
// language names a translation only when the block holds one in it.
func TestTargetRevision(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	b.SetTargetRuns("fr-FR", []model.Run{model.TextR("Bonjour")})
	assert.Equal(t, model.EditionRevision(b, model.Variant("fr-FR")), model.TargetRevision(b, "fr-FR"))
	assert.Equal(t, model.TargetRevision(b, "fr-FR"), model.TargetRevision(b, "fr_FR"), "every spelling of a locale names one translation")
	assert.Equal(t, model.AbsentRevision, model.TargetRevision(b, "de"))
	assert.Equal(t, model.AbsentRevision, model.TargetRevision(b, "en"), "the source language names no translation here")

	b.SetTargetRuns("en", []model.Run{model.TextR("Hi")})
	assert.Equal(t, model.RunsRevision(model.Variant("en"), []model.Run{model.TextR("Hi")}), model.TargetRevision(b, "en"),
		"a same-language translation is the one it names")
}

func TestCanonicalRunsJSON(t *testing.T) {
	assert.Equal(t, "[]", string(model.CanonicalRunsJSON(nil)))
	assert.Equal(t, "[]", string(model.CanonicalRunsJSON([]model.Run{})))
	assert.Equal(t, `[{"text":"a < b & c"}]`, string(model.CanonicalRunsJSON([]model.Run{model.TextR("a < b & c")})), "no HTML escaping")
	assert.Equal(t, `[null]`, string(model.CanonicalRunsJSON([]model.Run{{}})), "an invalid run is written as null")
	plural := model.PluralR(model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOther: {model.TextR("b")}, model.PluralOne: {model.TextR("a")},
	}})
	assert.Equal(t, `[{"plural":{"pivot":"n","forms":{"one":[{"text":"a"}],"other":[{"text":"b"}]}}}]`, string(model.CanonicalRunsJSON([]model.Run{plural})))
}
