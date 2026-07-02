package blockstore

import (
	"context"
	"testing"
)

func TestStoreKey_UniquePerFile(t *testing.T) {
	a := StoreKey("docs/a.md", "tu1", "Alpha")
	b := StoreKey("docs/b.md", "tu1", "Alpha")
	if a == b {
		t.Fatalf("same id in different files must not collide: %q == %q", a, b)
	}
	if a != StoreKey("docs/a.md", "tu1", "Alpha") {
		t.Fatal("StoreKey must be stable for the same (file, id)")
	}
	if a == StoreKey("docs/a.md", "tu2", "Alpha") {
		t.Fatal("distinct ids in the same file must differ")
	}
	// Empty id (daemon-backed readers): fall back to source text, still file-namespaced.
	n1 := StoreKey("a.md", "", "Hello")
	n2 := StoreKey("b.md", "", "Hello")
	if n1 == "" || n1 == n2 {
		t.Fatalf("empty-id fallback must be non-empty and file-namespaced: %q / %q", n1, n2)
	}
}

func TestOverlayKey_ContextFallback(t *testing.T) {
	// No source file in context → raw id (single-document scope, no collision).
	if got := OverlayKey(context.Background(), "tu1", "Alpha"); got != "tu1" {
		t.Fatalf("without source rel, OverlayKey must be the raw id, got %q", got)
	}
	// With a source file → the globally-unique StoreKey (matches the block hash).
	ctx := WithSourceRel(context.Background(), "docs/a.md")
	if got, want := OverlayKey(ctx, "tu1", "Alpha"), StoreKey("docs/a.md", "tu1", "Alpha"); got != want {
		t.Fatalf("with source rel, OverlayKey must equal StoreKey: %q != %q", got, want)
	}
}
