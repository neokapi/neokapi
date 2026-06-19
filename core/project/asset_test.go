package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func TestIsBinaryAssetFormat(t *testing.T) {
	if !IsBinaryAssetFormat("image") {
		t.Error("image should be a binary asset format")
	}
	for _, f := range []string{"json", "xliff", "markdown", ""} {
		if IsBinaryAssetFormat(f) {
			t.Errorf("%q should not be a binary asset format", f)
		}
	}
}

func TestResolveAssetVariants_NoTarget(t *testing.T) {
	// No target template → no variant pairing.
	if v := ResolveAssetVariants(t.TempDir(), ContentItem{Path: "a/*.png"}, "a/x.png", []model.LocaleID{"fr"}); v != nil {
		t.Errorf("empty target should yield nil, got %v", v)
	}
}

func TestResolveAssetVariants(t *testing.T) {
	root := t.TempDir()
	// Source en asset + a localized fr variant on disk; de is missing.
	mkdir := func(p string) { _ = os.MkdirAll(filepath.Join(root, p), 0o755) }
	mkdir("assets/en")
	mkdir("assets/fr")
	if err := os.WriteFile(filepath.Join(root, "assets/fr/logo.png"), []byte("FR"), 0o644); err != nil {
		t.Fatal(err)
	}

	item := ContentItem{Path: "assets/en/*.png", Base: "assets/en/", Target: "assets/{lang}/{name}.png"}
	variants := ResolveAssetVariants(root, item, "assets/en/logo.png",
		[]model.LocaleID{"fr", "de"})

	if len(variants) != 2 {
		t.Fatalf("got %d variants, want 2", len(variants))
	}
	fr, de := variants[0], variants[1]
	if want := filepath.Join(root, "assets/fr/logo.png"); fr.Path != want {
		t.Errorf("fr path = %q, want %q", fr.Path, want)
	}
	if !fr.Exists {
		t.Error("fr variant should exist")
	}
	if want := filepath.Join(root, "assets/de/logo.png"); de.Path != want {
		t.Errorf("de path = %q, want %q", de.Path, want)
	}
	if de.Exists {
		t.Error("de variant should be missing")
	}
}
