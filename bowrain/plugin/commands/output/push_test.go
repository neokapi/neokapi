package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPushOutput_FormatText_LoopFooter(t *testing.T) {
	tests := []struct {
		name     string
		out      PushOutput
		contains []string
		absent   []string
	}{
		{
			name: "on-push footer with web destinations",
			out: PushOutput{
				BlocksPushed: 12,
				WordCount:    240,
				FilesScanned: 3,
				Converge:     "on-push",
				ProjectURL:   "https://bowrain.example.com/acme/p/proj123/s/main",
				ReviewURL:    "https://bowrain.example.com/acme/tasks",
			},
			contains: []string{
				"Pushed 12 blocks, 240 words (scanned 3 files)",
				"Convergence: on-push — the server now translates, checks, and queues review for this push",
				"Project: https://bowrain.example.com/acme/p/proj123/s/main",
				"Review:  https://bowrain.example.com/acme/tasks",
			},
		},
		{
			name: "manual policy points at kapi up",
			out: PushOutput{
				BlocksPushed: 1,
				Converge:     "manual",
				ProjectURL:   "https://bowrain.example.com/acme/p/proj123/s/main",
			},
			contains: []string{
				"Convergence: manual — run 'kapi up' (or start a run from the web) to converge",
				"Project: https://bowrain.example.com/acme/p/proj123/s/main",
			},
			absent: []string{"Review:"},
		},
		{
			name: "dry run stays footer-free",
			out: PushOutput{
				BlocksPushed: 12,
				DryRun:       true,
				Converge:     "on-push",
				ProjectURL:   "https://bowrain.example.com/acme/p/proj123/s/main",
			},
			contains: []string{"Would push 12 blocks"},
			absent:   []string{"Convergence:", "Project:"},
		},
		{
			name: "no-op push stays footer-free",
			out: PushOutput{
				UpToDate:   true,
				Converge:   "on-push",
				ProjectURL: "https://bowrain.example.com/acme/p/proj123/s/main",
			},
			contains: []string{"Already up to date."},
			absent:   []string{"Convergence:", "Project:"},
		},
		{
			name:     "no server coordinates, no footer",
			out:      PushOutput{BlocksPushed: 2, WordCount: 4, FilesScanned: 1},
			contains: []string{"Pushed 2 blocks"},
			absent:   []string{"Convergence:", "Project:", "Review:"},
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
					t.Errorf("output missing %q\n---\n%s", want, got)
				}
			}
			for _, not := range tt.absent {
				if strings.Contains(got, not) {
					t.Errorf("output unexpectedly contains %q\n---\n%s", not, got)
				}
			}
		})
	}
}

func TestPushOutput_FormatText_BrandLine(t *testing.T) {
	tests := []struct {
		name     string
		out      PushOutput
		contains []string
		absent   []string
	}{
		{
			name: "created",
			out: PushOutput{
				BlocksPushed: 3,
				BrandProfile: "Acme Voice",
				BrandAction:  "created",
				BrandVersion: 1,
			},
			contains: []string{`Brand profile: "Acme Voice" created in the workspace brand hub (v1)`},
		},
		{
			name: "updated keeps history",
			out: PushOutput{
				BlocksPushed: 3,
				BrandProfile: "Acme Voice",
				BrandAction:  "updated",
				BrandVersion: 4,
			},
			contains: []string{`Brand profile: "Acme Voice" updated → v4 (previous version kept in history)`},
		},
		{
			name: "unchanged",
			out: PushOutput{
				UpToDate:     true,
				BrandProfile: "Acme Voice",
				BrandAction:  "unchanged",
				BrandVersion: 2,
			},
			contains: []string{`Brand profile: "Acme Voice" unchanged`},
		},
		{
			name: "skipped carries the reason",
			out: PushOutput{
				BlocksPushed: 1,
				BrandProfile: "Acme Voice",
				BrandAction:  "skipped",
				BrandReason:  "requires the manage-brand permission",
			},
			contains: []string{`Brand profile: "Acme Voice" not pushed (requires the manage-brand permission)`},
		},
		{
			name: "dry run announces the would-push",
			out: PushOutput{
				DryRun:       true,
				BlocksPushed: 3,
				BrandProfile: "Acme Voice",
				BrandAction:  "would-push",
			},
			contains: []string{`Would push brand profile "Acme Voice" to the workspace brand hub`},
			absent:   []string{"Brand profile:"},
		},
		{
			name:     "no brand push, no line",
			out:      PushOutput{BlocksPushed: 2},
			absent:   []string{"Brand profile", "brand hub"},
			contains: []string{"Pushed 2 blocks"},
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
					t.Errorf("output missing %q\n---\n%s", want, got)
				}
			}
			for _, not := range tt.absent {
				if strings.Contains(got, not) {
					t.Errorf("output unexpectedly contains %q\n---\n%s", not, got)
				}
			}
		})
	}
}
