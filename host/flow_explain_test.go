package host

import (
	"bytes"
	"testing"
)

func TestExplainBindings(t *testing.T) {
	tests := []struct {
		name   string
		flow   string
		inputs []string
		output string
		want   string
	}{
		{"file to file", "translate", []string{"a.json"}, "b.json", "flow translate: file(a.json) → file(b.json)\n"},
		{"file to default file", "translate", []string{"a.json"}, "", "flow translate: file(a.json) → file\n"},
		{"klz in place is process-only", "translate", []string{"work.klz"}, "", "flow translate: store(work.klz) → store\n"},
		{"no input sources from store", "qa", nil, "", "flow qa: store → store\n"},
		{"store to interchange", "extract", []string{"store:"}, "xliff:hand.xliff", "flow extract: store → interchange(hand.xliff)\n"},
		{"file to none for analysis", "qa", []string{"a.json"}, "none", "flow qa: file(a.json) → none\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := explainBindings(&buf, tt.flow, tt.inputs, tt.output); err != nil {
				t.Fatalf("explainBindings: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("explainBindings = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExplainProjectFlowRun(t *testing.T) {
	tests := []struct {
		name    string
		flow    string
		inputs  []string
		output  string
		locales []string
		want    string
	}{
		{
			"multi input all locales", "tm-recycle",
			[]string{"a/demo.yaml", "b/demo.yaml"}, "", []string{"nb", "de"},
			"flow tm-recycle: file(a/demo.yaml) → file\n" +
				"flow tm-recycle: file(b/demo.yaml) → file\n" +
				"locales: nb, de\n",
		},
		{
			"single input explicit output", "tm-recycle",
			[]string{"a.json"}, "out.json", []string{"nb"},
			"flow tm-recycle: file(a.json) → file(out.json)\nlocales: nb\n",
		},
		{
			"source-only pass omits empty locale", "lint",
			[]string{"a.json"}, "", []string{""},
			"flow lint: file(a.json) → file\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := explainProjectFlowRun(&buf, tt.flow, tt.inputs, tt.output, tt.locales); err != nil {
				t.Fatalf("explainProjectFlowRun: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("explainProjectFlowRun = %q, want %q", got, tt.want)
			}
		})
	}
}
