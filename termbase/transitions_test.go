package termbase_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
)

func TestKnownTermStatus(t *testing.T) {
	for _, s := range []model.TermStatus{
		model.TermProposed, model.TermApproved, model.TermPreferred,
		model.TermAdmitted, model.TermDeprecated, model.TermForbidden,
	} {
		assert.True(t, termbase.KnownTermStatus(s), string(s))
	}
	assert.False(t, termbase.KnownTermStatus(""))
	assert.False(t, termbase.KnownTermStatus("bogus"))
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    model.TermStatus
		to      model.TermStatus
		wantErr bool
	}{
		{"proposed to approved", model.TermProposed, model.TermApproved, false},
		{"approved to preferred", model.TermApproved, model.TermPreferred, false},
		{"preferred to deprecated", model.TermPreferred, model.TermDeprecated, false},
		{"approved to forbidden", model.TermApproved, model.TermForbidden, false},
		// History is the guard, not a trap: forbidden → preferred is valid,
		// it is just governed (see TestIsGovernedTransition).
		{"forbidden to preferred", model.TermForbidden, model.TermPreferred, false},
		{"no-op same to same", model.TermApproved, model.TermApproved, false},
		{"no-op forbidden to forbidden", model.TermForbidden, model.TermForbidden, false},
		{"unknown from", "bogus", model.TermApproved, true},
		{"unknown to", model.TermApproved, "bogus", true},
		{"empty from", "", model.TermApproved, true},
		{"empty to", model.TermApproved, "", true},
		{"both unknown", "x", "y", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := termbase.ValidateTransition(tt.from, tt.to)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsGovernedTransition(t *testing.T) {
	tests := []struct {
		name string
		from model.TermStatus
		to   model.TermStatus
		want bool
	}{
		// Any transition TO forbidden or preferred is governed.
		{"approved to forbidden", model.TermApproved, model.TermForbidden, true},
		{"deprecated to forbidden", model.TermDeprecated, model.TermForbidden, true},
		{"proposed to preferred", model.TermProposed, model.TermPreferred, true},
		{"approved to preferred", model.TermApproved, model.TermPreferred, true},
		// Any transition FROM forbidden is governed.
		{"forbidden to deprecated", model.TermForbidden, model.TermDeprecated, true},
		{"forbidden to approved", model.TermForbidden, model.TermApproved, true},
		{"forbidden to preferred", model.TermForbidden, model.TermPreferred, true},
		// Ordinary curation is not governed.
		{"proposed to approved", model.TermProposed, model.TermApproved, false},
		{"approved to admitted", model.TermApproved, model.TermAdmitted, false},
		{"approved to deprecated", model.TermApproved, model.TermDeprecated, false},
		{"preferred to deprecated", model.TermPreferred, model.TermDeprecated, false},
		{"preferred to approved", model.TermPreferred, model.TermApproved, false},
		// No-op transitions are never governed.
		{"forbidden to forbidden", model.TermForbidden, model.TermForbidden, false},
		{"preferred to preferred", model.TermPreferred, model.TermPreferred, false},
		{"approved to approved", model.TermApproved, model.TermApproved, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, termbase.IsGovernedTransition(tt.from, tt.to))
		})
	}
}
