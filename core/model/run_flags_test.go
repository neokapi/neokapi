package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Text read back from placeholder or edit text keeps the do-not-translate mark
// the reference gave the text it matches.
func TestParseRunsText_KeepsNoTranslate(t *testing.T) {
	ref := kapiCheckSpan()
	shown := RunsPlaceholderText(ref)
	assert.Equal(t, `Run <x id="1"/>kapi check<x id="/1"/> in CI.`, shown)

	tests := []struct {
		name string
		text string
		want string
	}{
		{"unchanged", shown, "Run <1>[kapi check]</1> in CI."},
		{"wording outside the code span changed", `Execute <x id="1"/>kapi check<x id="/1"/> in CI.`, "Execute <1>[kapi check]</1> in CI."},
		{"the protected text edited", `Run <x id="1"/>kapi verify<x id="/1"/> in CI.`, "Run <1>[kapi verify]</1> in CI."},
		{"a translation keeps the command", `Exécutez <x id="1"/>kapi check<x id="/1"/> dans la CI.`, "Exécutez <1>[kapi check]</1> dans la CI."},
		{"text the reference never marked stays plain", `Run it <x id="1"/>kapi check<x id="/1"/> in CI.`, "Run it <1>[kapi check]</1> in CI."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, flagged(ParseRunsPlaceholderText(tc.text, ref)), "placeholder text")
			assert.Equal(t, tc.want, flagged(ParseRunsEditText(tc.text, ref)), "edit text")
		})
	}
}

func TestParseRunsText_LeavesUnmarkedReferencesAlone(t *testing.T) {
	ref := boldSpan()
	got := ParseRunsPlaceholderText(`Hi <x id="1"/>there<x id="/1"/>`, ref)
	for _, r := range got {
		if r.Text != nil {
			assert.False(t, r.Text.NoTranslate)
		}
	}
	assert.Equal(t, "Hi <1>there</1>", sig(got))
}
