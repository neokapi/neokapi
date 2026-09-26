package model

import "testing"

func TestSourceHeldMarker(t *testing.T) {
	b := &Block{}
	if b.SourceHeld() {
		t.Fatal("a fresh block is not held")
	}

	b.SetSourceHeld(true)
	if !b.SourceHeld() {
		t.Fatal("SetSourceHeld(true) marks the block held")
	}
	if b.Properties[PropSourceHeld] != "1" {
		t.Fatalf("hold marker property = %q, want 1", b.Properties[PropSourceHeld])
	}

	b.SetSourceHeld(false)
	if b.SourceHeld() {
		t.Fatal("SetSourceHeld(false) clears the hold")
	}
	if _, ok := b.Properties[PropSourceHeld]; ok {
		t.Fatal("clearing removes the property rather than leaving a stale value")
	}
}

func TestSetSourceHeldFalseNoAlloc(t *testing.T) {
	b := &Block{} // nil Properties
	b.SetSourceHeld(false)
	if b.Properties != nil {
		t.Fatal("clearing an unset marker must not allocate the properties map")
	}
}

func TestResolveTranslateAfterDefault(t *testing.T) {
	// Unset resolves to the default (checked); a typo also falls back to the
	// default and is flagged.
	if g, ok := ResolveTranslateAfter(""); g != DefaultTranslateAfter || !ok {
		t.Fatalf("empty level = (%q,%v), want (%q,true)", g, ok, DefaultTranslateAfter)
	}
	if g, ok := ResolveTranslateAfter("bogus"); g != DefaultTranslateAfter || ok {
		t.Fatalf("unknown level = (%q,%v), want (%q,false)", g, ok, DefaultTranslateAfter)
	}
	if g, _ := ResolveTranslateAfter("none"); g != TranslateAfterNone {
		t.Fatalf("none level = %q, want none", g)
	}
}
