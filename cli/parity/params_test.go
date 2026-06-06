//go:build parity

package parity

import (
	"reflect"
	"testing"
)

func TestStringifyParams(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want map[string]string
	}{
		{
			name: "nil → nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty → nil",
			in:   map[string]any{},
			want: nil,
		},
		{
			name: "scalars",
			in: map[string]any{
				"useJavaEscapes":      true,
				"escapeExtendedChars": false,
				"keyCondition":        "^msg",
				"maxItems":            42,
				"ratio":               1.5,
			},
			want: map[string]string{
				"useJavaEscapes":      "true",
				"escapeExtendedChars": "false",
				"keyCondition":        "^msg",
				"maxItems":            "42",
				"ratio":               "1.5",
			},
		},
		{
			name: "list → JSON",
			in: map[string]any{
				"codeFinderRules": []string{`\d+`, `%[a-z]+`},
			},
			want: map[string]string{
				"codeFinderRules": `["\\d+","%[a-z]+"]`,
			},
		},
		{
			name: "nil value omitted",
			in: map[string]any{
				"keyCondition":  nil,
				"useCodeFinder": true,
			},
			want: map[string]string{
				"useCodeFinder": "true",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StringifyParams(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("StringifyParams(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
