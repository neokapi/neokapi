package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeriveShipState covers the boundary cases of the ship-state rule:
// governed = coverage 100% AND no failing checks AND every block approved;
// ai_shippable = coverage 100% AND no failing checks, approval incomplete;
// pending = anything less (empty scope, partial coverage, failing checks).
func TestDeriveShipState(t *testing.T) {
	tests := []struct {
		name                                 string
		translated, total, approved, failing int
		want                                 ShipState
	}{
		{"empty locale (nothing to ship)", 0, 0, 0, 0, ShipStatePending},
		{"untranslated", 0, 10, 0, 0, ShipStatePending},
		{"partial coverage", 9, 10, 0, 0, ShipStatePending},
		{"partial coverage all approved so far", 9, 10, 9, 0, ShipStatePending},
		{"full coverage, no review, checks pass", 10, 10, 0, 0, ShipStateAIShippable},
		{"full coverage, mixed approval", 10, 10, 5, 0, ShipStateAIShippable},
		{"full coverage, one short of full approval", 10, 10, 9, 0, ShipStateAIShippable},
		{"full coverage, fully approved", 10, 10, 10, 0, ShipStateGoverned},
		{"single block approved", 1, 1, 1, 0, ShipStateGoverned},
		{"failing checks demote below the gate", 10, 10, 0, 1, ShipStatePending},
		{"failing checks demote even a fully approved locale", 10, 10, 10, 2, ShipStatePending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DeriveShipState(tt.translated, tt.total, tt.approved, tt.failing))
		})
	}
}
