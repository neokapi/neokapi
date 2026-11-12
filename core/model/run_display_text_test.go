package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunsDisplayText(t *testing.T) {
	tests := []struct {
		name string
		runs []Run
		want string
	}{
		{
			name: "a code finder's placeholder reads as its native form",
			runs: []Run{TextR("Failed to cordon node "), {Ph: &PlaceholderRun{ID: "c1", Type: "code", Data: "{{name}}"}}},
			want: "Failed to cordon node {{name}}",
		},
		{
			name: "a display text wins over the native form",
			runs: []Run{TextR("Hi "), {Ph: &PlaceholderRun{ID: "1", Data: "%1$s", Disp: "{user}"}}},
			want: "Hi {user}",
		},
		{
			name: "a placeholder with only an equiv reads as it in braces",
			runs: []Run{TextR("Hi "), {Ph: &PlaceholderRun{ID: "1", Equiv: "user"}}},
			want: "Hi {user}",
		},
		{
			name: "paired codes keep the text between them",
			runs: []Run{
				TextR("Read "),
				{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
				TextR("this"),
				{PcClose: &PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
			},
			want: "Read this",
		},
		{
			name: "a plural reads as its other form",
			runs: []Run{{Plural: &PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{
				PluralOne:   {TextR("one file")},
				PluralOther: {{Ph: &PlaceholderRun{ID: "1", Data: "{n}"}}, TextR(" files")},
			}}}},
			want: "{n} files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RunsDisplayText(tt.runs))
		})
	}
}
