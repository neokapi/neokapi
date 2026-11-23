package blockstore

import "testing"

// TestStoreKeyIsStable pins the bytes of StoreKey. The SQLite block store
// files blocks and overlays under it, so a change to its seed strands every
// stored block.
func TestStoreKeyIsStable(t *testing.T) {
	for _, tc := range []struct{ rel, id, text, want string }{
		{"docs/a.md", "tu1", "Alpha", "9cd2ae6e84c16eb69a1401237696f16acd90d6afe8a170b8abf3a47859ed1655"},
		{"docs/a.md", "", "Alpha", "b8b852e77d8a4e273772868ad8c8c090e8f50c518bd71d71e865f3c863c83f8a"},
	} {
		if got := StoreKey(tc.rel, tc.id, tc.text); got != tc.want {
			t.Errorf("StoreKey(%q, %q, %q) = %q, want %q", tc.rel, tc.id, tc.text, got, tc.want)
		}
	}
}
