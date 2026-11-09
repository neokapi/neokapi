package host

import (
	"testing"

	"github.com/neokapi/neokapi/host/screens"
	"github.com/stretchr/testify/assert"
)

func TestLookNext(t *testing.T) {
	steps := func(s screens.MapStepState) []screens.MapStepDTO {
		s.Checked = true
		return screens.MapSteps(s)
	}
	tests := []struct {
		name string
		m    LookProject
		next string
		line string
	}{
		{
			name: "no content",
			m:    LookProject{Source: &SourceCoverage{}},
			next: "kapi add <pattern>",
		},
		{
			name: "suggestions waiting",
			m:    LookProject{Suggests: 2, Steps: steps(screens.MapStepState{Awaiting: 2})},
			next: "kapi rules",
			line: "Decide with `kapi rules`. See a file with `kapi look <path>`.",
		},
		{
			name: "findings that fail",
			m:    LookProject{Failing: 3, Steps: steps(screens.MapStepState{Failing: 3})},
			next: "kapi fix",
			line: "Fix what fails with `kapi fix`. See a file with `kapi look <path>`.",
		},
		{
			name: "no assistant connected",
			m:    LookProject{Steps: steps(screens.MapStepState{Shared: true})},
			next: "kapi connect",
			line: "Connect your assistant with `kapi connect`. See a file with `kapi look <path>`.",
		},
		{
			name: "rules kept on this machine",
			m:    LookProject{Steps: steps(screens.MapStepState{Connected: true})},
			next: "kapi share",
			line: "Share the rules with `kapi share`. See a file with `kapi look <path>`.",
		},
		{
			name: "steps taken, findings fail",
			m:    LookProject{Failing: 1},
			next: "kapi fix",
		},
		{
			name: "steps taken, nothing fails",
			m:    LookProject{},
			next: "kapi check",
			line: "Check with `kapi check`. See a file with `kapi look <path>`.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m
			m.Next = lookNext(&m)
			assert.Equal(t, tt.next, m.Next)
			if tt.line != "" {
				assert.Equal(t, tt.line, m.NextLine())
			}
		})
	}
}

func TestConnectHostInPlace(t *testing.T) {
	kept := ConnectFile{Kind: ConnectMCP, Action: ConnectKept}
	tests := []struct {
		name  string
		files []ConnectFile
		want  bool
	}{
		{"every file kept", []ConnectFile{kept, {Kind: ConnectSkill, Action: ConnectKept}}, true},
		{"a hook waiting for a person", []ConnectFile{kept, {Kind: ConnectHook, Action: ConnectPending}}, true},
		{"a file to create", []ConnectFile{kept, {Kind: ConnectSkill, Action: ConnectCreated}}, false},
		{"a file to update", []ConnectFile{{Kind: ConnectMCP, Action: ConnectUpdated}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ConnectHost{Files: tt.files}.InPlace())
		})
	}
}
