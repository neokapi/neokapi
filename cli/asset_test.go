package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreserveAssetVariant(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "en.png")
	if err := os.WriteFile(src, []byte("EN"), 0o644); err != nil {
		t.Fatal(err)
	}
	variant := filepath.Join(dir, "fr.png")
	if err := os.WriteFile(variant, []byte("FR"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "de.png")

	// Existing, distinct image variant → authoritative, preserve it.
	if !preserveAssetVariant("image", src, variant) {
		t.Error("existing image variant should be preserved")
	}
	// Missing variant → run the flow to produce a fallback.
	if preserveAssetVariant("image", src, missing) {
		t.Error("missing variant should not be preserved")
	}
	// Source == target → not a distinct variant.
	if preserveAssetVariant("image", src, src) {
		t.Error("source-as-target should not be preserved")
	}
	// Text formats are always reprocessed.
	if preserveAssetVariant("json", src, variant) {
		t.Error("text format should never be preserved")
	}
}
