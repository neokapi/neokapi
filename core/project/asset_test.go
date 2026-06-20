package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func TestIsBinaryAssetFormat(t *testing.T) {
	for _, f := range []string{"image", "audio", "video"} {
		if !IsBinaryAssetFormat(f) {
			t.Errorf("%q should be a binary asset format", f)
		}
	}
	for _, f := range []string{"json", "xliff", "markdown", "srt", "vtt", ""} {
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

// TestResolveAssetVariants_AudioVideo proves that whole-file audio and video
// assets resolve per-locale replacement variants the same way images do:
// pairing the source with each locale's target path, detecting an
// already-localized variant on disk, and flagging the missing one.
func TestResolveAssetVariants_AudioVideo(t *testing.T) {
	cases := []struct {
		name   string
		ext    string
		source string
	}{
		{"audio", ".mp3", "media/en/intro.mp3"},
		{"video", ".mp4", "media/en/intro.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			// The fr replacement is already supplied; de is not yet localized.
			if err := os.MkdirAll(filepath.Join(root, "media/fr"), 0o755); err != nil {
				t.Fatal(err)
			}
			frPath := filepath.Join(root, "media/fr/intro"+tc.ext)
			if err := os.WriteFile(frPath, []byte("FR"), 0o644); err != nil {
				t.Fatal(err)
			}

			item := ContentItem{
				Path:   "media/en/*" + tc.ext,
				Base:   "media/en/",
				Target: "media/{lang}/{name}" + tc.ext,
			}
			variants := ResolveAssetVariants(root, item, tc.source,
				[]model.LocaleID{"fr", "de"})
			if len(variants) != 2 {
				t.Fatalf("got %d variants, want 2", len(variants))
			}
			fr, de := variants[0], variants[1]
			if fr.Path != frPath {
				t.Errorf("fr path = %q, want %q", fr.Path, frPath)
			}
			if !fr.Exists {
				t.Error("fr variant should be detected (skip-already-localized)")
			}
			if want := filepath.Join(root, "media/de/intro"+tc.ext); de.Path != want {
				t.Errorf("de path = %q, want %q", de.Path, want)
			}
			if de.Exists {
				t.Error("de variant should be missing")
			}
		})
	}
}
