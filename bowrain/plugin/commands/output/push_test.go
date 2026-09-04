package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/venue"
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
				BlocksPushed:   12,
				BlocksUploaded: 12,
				WordCount:      240,
				FilesScanned:   3,
				Converge:       "on-push",
				ProjectURL:     "https://bowrain.example.com/acme/p/proj123/s/main",
				ReviewURL:      "https://bowrain.example.com/acme/tasks",
			},
			contains: []string{
				"Pushed 12 blocks (12 uploaded), 240 words (scanned 3 files)",
				"Convergence: on-push. The server now translates, checks, and queues review for this push",
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
				"Convergence: manual. Run 'kapi up' (or start a run from the web) to converge",
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

func TestPushOutput_FormatText_VoiceLine(t *testing.T) {
	tests := []struct {
		name     string
		out      PushOutput
		contains []string
		absent   []string
	}{
		{
			name: "carried with the push",
			out: PushOutput{
				BlocksPushed: 3,
				VoiceProfile: "Acme Voice",
				VoiceAction:  "carried",
			},
			contains: []string{`Voice profile: "Acme Voice" carried to the workspace brand hub`},
		},
		{
			name: "carried on a push that moved no content",
			out: PushOutput{
				UpToDate:     true,
				VoiceProfile: "Acme Voice",
				VoiceAction:  "carried",
			},
			contains: []string{`Voice profile: "Acme Voice" carried to the workspace brand hub`},
		},
		{
			name: "skipped carries the reason",
			out: PushOutput{
				BlocksPushed: 1,
				VoiceProfile: "Acme Voice",
				VoiceAction:  "skipped",
				VoiceReason:  "--no-brand",
			},
			contains: []string{`Voice profile: "Acme Voice" not pushed (--no-brand)`},
		},
		{
			name: "dry run announces the would-push",
			out: PushOutput{
				DryRun:       true,
				BlocksPushed: 3,
				VoiceProfile: "Acme Voice",
				VoiceAction:  "would-push",
			},
			contains: []string{`Would push voice profile "Acme Voice" to the workspace brand hub`},
			absent:   []string{"Voice profile:"},
		},
		{
			name:     "no brand push, no line",
			out:      PushOutput{BlocksPushed: 2},
			absent:   []string{"Voice profile", "brand hub"},
			contains: []string{"Pushed 2 blocks"},
		},
		{
			// A recipe edit that stops declaring a collection must say what
			// became of it. Silence here reads as "removed", which is the one
			// thing that did not happen.
			name: "an undeclared collection is reported as kept",
			out: PushOutput{
				BlocksPushed:          1,
				UndeclaredCollections: []string{"marketing", "legacy"},
			},
			contains: []string{
				"no longer declares (kept, with their content): marketing, legacy",
			},
		},
		{
			name:   "nothing undeclared, no line",
			out:    PushOutput{BlocksPushed: 1},
			absent: []string{"no longer declares"},
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

// The push footer says what the platform did not accept. A refused approval
// still stored its content, so a silent report reads as "approved" to the one
// person in a position to notice otherwise.
func TestPushOutput_FormatText_RefusedVerdicts(t *testing.T) {
	tests := []struct {
		name     string
		out      PushOutput
		contains []string
		absent   []string
	}{
		{
			name: "one line per language and reason, in the plain register",
			out: PushOutput{
				BlocksPushed: 47,
				VerdictsRefused: []venue.DecisionRefusal{
					{Locale: "fr-FR", Kind: venue.VerdictApproval, Reason: venue.RefusedNoReviewPermission, Count: 2},
					{Locale: "de-DE", Kind: venue.VerdictSignOff, Reason: venue.RefusedSeparationOfDuties, Count: 1},
					{Locale: "nb-NO", Kind: venue.VerdictDemotion, Reason: venue.RefusedSignOffWithdrawal, Count: 2},
				},
				VerdictsRetired: 5,
			},
			contains: []string{
				"2 approvals not accepted for fr-FR: no review permission",
				"1 sign-off not accepted for de-DE: separation of duties",
				"2 demotions not accepted for nb-NO: withdrawing a sign-off needs review permission",
				"5 local record(s) now match the platform; they will not be sent again",
			},
		},
		{
			name: "a push the platform accepted whole says nothing about it",
			out:  PushOutput{BlocksPushed: 47},
			absent: []string{
				"not accepted",
				"now match the platform",
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
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("output should not contain %q\ngot:\n%s", unwanted, got)
				}
			}
		})
	}
}
