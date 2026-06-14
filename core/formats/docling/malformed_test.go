package docling_test

import (
	"testing"

	doclingfmt "github.com/neokapi/neokapi/core/formats/docling"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// TestMalformedInputs feeds the reader broken/degenerate input and asserts it
// never panics, surfaces parse failures as errors, and degrades gracefully on
// structurally valid but incomplete documents (dangling refs, missing arrays).
func TestMalformedInputs(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantErr  bool // expect a PartResult.Error
		maxBlock int  // upper bound on emitted blocks (when no error)
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "not_json", input: "this is not json", wantErr: true},
		{name: "truncated", input: `{"schema_name":"DoclingDocument","texts":[`, wantErr: true},
		{name: "empty_object", input: `{}`, wantErr: false, maxBlock: 0},
		{
			name:     "dangling_ref",
			input:    `{"schema_name":"DoclingDocument","body":{"children":[{"$ref":"#/texts/9"}]},"texts":[]}`,
			wantErr:  false,
			maxBlock: 0,
		},
		{
			name:     "null_bbox",
			input:    `{"schema_name":"DoclingDocument","body":{"children":[{"$ref":"#/texts/0"}]},"texts":[{"self_ref":"#/texts/0","label":"paragraph","text":"ok","prov":[{"page_no":1,"bbox":null}]}]}`,
			wantErr:  false,
			maxBlock: 1,
		},
		{
			name:     "cyclic_group",
			input:    `{"schema_name":"DoclingDocument","body":{"children":[{"$ref":"#/groups/0"}]},"groups":[{"self_ref":"#/groups/0","name":"list","children":[{"$ref":"#/groups/0"},{"$ref":"#/texts/0"}]}],"texts":[{"self_ref":"#/texts/0","label":"list_item","text":"item"}]}`,
			wantErr:  false,
			maxBlock: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			r := doclingfmt.NewReader()
			if err := r.Open(ctx, testutil.RawDocFromString(tc.input, model.LocaleEnglish)); err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer r.Close()

			var sawErr bool
			var blocks int
			for res := range r.Read(ctx) {
				if res.Error != nil {
					sawErr = true
					continue
				}
				if res.Part != nil && res.Part.Type == model.PartBlock {
					blocks++
				}
			}
			if tc.wantErr && !sawErr {
				t.Errorf("expected an error, got none")
			}
			if !tc.wantErr && sawErr {
				t.Errorf("unexpected error")
			}
			if !tc.wantErr && blocks > tc.maxBlock {
				t.Errorf("emitted %d blocks, want <= %d", blocks, tc.maxBlock)
			}
		})
	}
}
