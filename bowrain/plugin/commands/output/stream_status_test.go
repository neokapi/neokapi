package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamStatusOutput_FormatText(t *testing.T) {
	tests := []struct {
		name     string
		out      StreamStatusOutput
		contains []string
		absent   []string
	}{
		{
			name: "main base stream, active",
			out: StreamStatusOutput{
				Stream: "main",
				Active: true,
				Exists: false,
			},
			contains: []string{"Stream: main (active)", "State:  main (base stream)"},
			absent:   []string{"Ahead of", "not created"},
		},
		{
			name: "named stream not created on server",
			out: StreamStatusOutput{
				Stream: "feature-x",
				Active: true,
				Exists: false,
			},
			contains: []string{
				"Stream: feature-x (active)",
				"State:  not created on the server yet",
				"Create it with: kapi stream create feature-x",
			},
		},
		{
			name: "stream ahead of parent with breakdown",
			out: StreamStatusOutput{
				Stream:           "feature-x",
				Parent:           "main",
				Visibility:       "private",
				Active:           true,
				Exists:           true,
				Ahead:            5,
				AddedVsParent:    3,
				ModifiedVsParent: 1,
				RemovedVsParent:  1,
				Behind:           2,
				PendingPush:      4,
			},
			contains: []string{
				"Parent: main",
				"State:  active",
				"Visibility: private",
				"Ahead of main: 5 block(s) (3 added, 1 modified, 1 removed)",
				"Behind: 2 remote change(s) to pull",
				"Pending push: 4 block(s) changed locally",
			},
		},
		{
			name: "stream up to date with parent",
			out: StreamStatusOutput{
				Stream: "feature-y",
				Parent: "main",
				Exists: true,
				Ahead:  0,
			},
			contains: []string{"Ahead of main: up to date"},
		},
		{
			name: "archived stream, behind unknown",
			out: StreamStatusOutput{
				Stream:   "old",
				Parent:   "main",
				Exists:   true,
				Archived: true,
				Ahead:    -1,
				Behind:   -1,
			},
			contains: []string{
				"State:  archived",
				"Ahead of main: changes present",
				"Behind: remote changes available",
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
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatText() = %q\nmissing %q", got, want)
				}
			}
			for _, no := range tt.absent {
				if strings.Contains(got, no) {
					t.Errorf("FormatText() = %q\nshould not contain %q", got, no)
				}
			}
		})
	}
}
