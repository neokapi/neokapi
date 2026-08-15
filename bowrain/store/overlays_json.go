package store

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// Block.Overlays are the positional, run-anchored stand-off layers
// (segmentation, term, entity, term-candidate, qa, alignment) that ride
// alongside a block's source runs. They persist in the blocks row's `overlays`
// column, next to `source_json`, so they survive a store round-trip — without
// it, the entity/term overlays the entity→concept promote path depends on are
// dropped on the next GetBlocks.
//
// The JSON codec is the canonical one in core/venue, shared with the
// REST sync-pull projection (host/venue/client) so the overlay wire shape is
// defined once rather than mirrored per consumer. These wrappers keep the
// store's historical MarshalOverlays/UnmarshalOverlays call sites and the
// byte-stable "[]" column default unchanged.

// MarshalOverlays encodes a block's stand-off overlays for the `overlays`
// column. Nil/empty overlays encode to the byte-stable "[]" default so an unset
// round-trip is a no-op.
func MarshalOverlays(overlays []model.Overlay) ([]byte, error) {
	return venue.MarshalOverlays(overlays)
}

// UnmarshalOverlays reverses MarshalOverlays, rehydrating each span's typed
// Value through the model payload registry. An empty / "[]" / "null" column
// yields nil overlays.
func UnmarshalOverlays(data []byte) ([]model.Overlay, error) {
	return venue.UnmarshalOverlays(data)
}
