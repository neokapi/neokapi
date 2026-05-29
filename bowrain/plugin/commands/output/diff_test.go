package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestDiffOutput_FormatText(t *testing.T) {
	tests := []struct {
		name     string
		out      DiffOutput
		contains []string
		exact    string
	}{
		{
			name:  "no changes, connected",
			out:   DiffOutput{Connected: true},
			exact: "No changes. Local and remote are in sync.\n",
		},
		{
			name:  "no changes, offline",
			out:   DiffOutput{Connected: false},
			exact: "No local changes since the last sync.\n  (no server configured — comparing against the local sync cache)\n",
		},
		{
			name: "no local changes but remote pending",
			out: DiffOutput{
				Connected:   true,
				PendingPull: 3,
			},
			exact: "Remote: 3 change(s) available to pull\n",
		},
		{
			name: "files changed, summary only",
			out: DiffOutput{
				Connected: true,
				Files: []DiffFileEntry{
					{Path: "src/en.json", Format: "json", Added: 2, Changed: 1},
					{Path: "src/de.json", Format: "json", Removed: 1},
				},
				Added:   2,
				Changed: 1,
				Removed: 1,
			},
			contains: []string{
				"src/en.json", "+2 ~1",
				"src/de.json", "-1",
				"2 file(s) changed: +2 ~1 -1",
				"Use --verbose to see changed block ids/keys.",
			},
		},
		{
			name: "files changed, verbose lists blocks",
			out: DiffOutput{
				Connected: true,
				Verbose:   true,
				Files: []DiffFileEntry{
					{
						Path:    "src/en.json",
						Format:  "json",
						Added:   1,
						Changed: 1,
						Blocks: []DiffBlockEntry{
							{BlockID: "b1", Name: "greeting", Preview: "Hello", Change: "added"},
							{BlockID: "b2", Name: "farewell", Preview: "Bye", Change: "changed"},
						},
					},
				},
				Added:   1,
				Changed: 1,
			},
			contains: []string{
				"+ greeting — Hello",
				"~ farewell — Bye",
				"1 file(s) changed: +1 ~1",
			},
		},
		{
			name: "verbose with pending pull and no --verbose hint",
			out: DiffOutput{
				Connected:   true,
				Verbose:     true,
				PendingPull: -1,
				Files: []DiffFileEntry{
					{Path: "a.json", Added: 1, Blocks: []DiffBlockEntry{{BlockID: "x", Change: "added"}}},
				},
				Added: 1,
			},
			contains: []string{
				"+ x",
				"Remote: changes available to pull",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tt.out.FormatText(&buf); err != nil {
				t.Fatalf("FormatText: %v", err)
			}
			got := buf.String()
			if tt.exact != "" {
				if got != tt.exact {
					t.Errorf("FormatText() = %q, want %q", got, tt.exact)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatText() = %q, missing %q", got, want)
				}
			}
			// The --verbose hint must only appear when not already verbose.
			if tt.out.Verbose && strings.Contains(got, "Use --verbose") {
				t.Errorf("verbose output should not print the --verbose hint: %q", got)
			}
		})
	}
}

func TestDiffStat(t *testing.T) {
	tests := []struct {
		added, changed, removed int
		want                    string
	}{
		{0, 0, 0, "no changes"},
		{1, 0, 0, "+1"},
		{0, 2, 0, "~2"},
		{0, 0, 3, "-3"},
		{1, 2, 3, "+1 ~2 -3"},
	}
	for _, tt := range tests {
		if got := diffStat(tt.added, tt.changed, tt.removed); got != tt.want {
			t.Errorf("diffStat(%d,%d,%d) = %q, want %q", tt.added, tt.changed, tt.removed, got, tt.want)
		}
	}
}

func TestChangeSigil(t *testing.T) {
	cases := map[string]string{"added": "+", "changed": "~", "removed": "-", "other": "?"}
	for in, want := range cases {
		if got := changeSigil(in); got != want {
			t.Errorf("changeSigil(%q) = %q, want %q", in, got, want)
		}
	}
}
