package project

import "testing"

// BlockStoreHash must be globally unique per (source file, in-file id): the same
// file-local id in two different files must NOT collide (the root of the
// "Website 0 blocks" bug), while the same (file, id) stays stable so target
// overlays survive a re-extract.
func TestBlockStoreHash(t *testing.T) {
	a := BlockStoreHash("input/docs/a.md", "tu1", "API Reference")
	b := BlockStoreHash("input/docs/b.md", "tu1", "API Reference")
	if a == b {
		t.Fatalf("same id in different files must not collide: %q == %q", a, b)
	}

	// Stable across calls (re-extract idempotence).
	if a != BlockStoreHash("input/docs/a.md", "tu1", "API Reference") {
		t.Fatal("hash must be stable for the same (file, id)")
	}

	// Distinct ids in the same file differ.
	if a == BlockStoreHash("input/docs/a.md", "tu2", "API Reference") {
		t.Fatal("distinct ids in the same file must not collide")
	}

	// No id (daemon-backed readers): fall back to source text, still namespaced
	// by file so identical text in different files stays distinct.
	n1 := BlockStoreHash("a.md", "", "Hello")
	n2 := BlockStoreHash("b.md", "", "Hello")
	if n1 == "" || n1 == n2 {
		t.Fatalf("empty-id fallback must be non-empty and file-namespaced: %q / %q", n1, n2)
	}
}
