package format_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

func TestTrimRunEdges(t *testing.T) {
	bold := func(text string) []model.Run {
		return []model.Run{model.PcOpenR(model.PcOpenRun{ID: "1"}), model.TextR(text), model.PcCloseR(model.PcCloseRun{ID: "1"})}
	}
	for _, tc := range []struct {
		name       string
		runs, want []model.Run
	}{
		{"nothing at the ends", []model.Run{model.TextR("Hello there")}, []model.Run{model.TextR("Hello there")}},
		{"both ends of one run", []model.Run{model.TextR(" \nHello there\n ")}, []model.Run{model.TextR("Hello there")}},
		{"the space inside stays", []model.Run{model.TextR("Hello  there ")}, []model.Run{model.TextR("Hello  there")}},
		{"a run of white space at each end is left out",
			[]model.Run{model.TextR(" "), model.TextR("Hello"), model.TextR("\n")}, []model.Run{model.TextR("Hello")}},
		{"the trim stops at an inline code",
			append(append([]model.Run{model.TextR(" ")}, bold(" Hello ")...), model.TextR(" ")), bold(" Hello ")},
		{"only white space", []model.Run{model.TextR(" \n")}, []model.Run{}},
		{"no runs", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			given := model.RunsEditText(tc.runs)
			got := format.TrimRunEdges(tc.runs, " \n")
			assert.Equal(t, model.RunsEditText(tc.want), model.RunsEditText(got))
			assert.Len(t, got, len(tc.want))
			assert.Equal(t, given, model.RunsEditText(tc.runs), "the runs given are left as they were")
			assert.Equal(t, model.RunsEditText(got), model.RunsEditText(format.TrimRunEdges(got, " \n")), "a second trim changes nothing")
		})
	}
}
